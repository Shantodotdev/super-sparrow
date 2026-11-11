package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "sparrow"
	keyringUser    = "login"
)

// errNoEntry is returned by a Keyring when nothing is stored.
var errNoEntry = errors.New("no stored entry")

// errNotSignedIn is returned when there are no stored credentials.
var errNotSignedIn = errors.New("not signed in")

// Keyring stores one secret per service and user, normally in the operating system's keychain.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

// systemKeyring is the operating system's keychain.
type systemKeyring struct{}

func (systemKeyring) Get(service, user string) (string, error) {
	secret, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", errNoEntry
	}
	return secret, err
}

func (systemKeyring) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

func (systemKeyring) Delete(service, user string) error {
	err := keyring.Delete(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return errNoEntry
	}
	return err
}

// credentials is a stored CLI login.
type credentials struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// credentialStore keeps the login in the keychain, or in a private file when the keychain is not
// available, such as on a headless Linux machine.
type credentialStore struct {
	keyring   Keyring
	configDir func() (string, error)
}

// path returns the location of the fallback credentials file.
func (s credentialStore) path() (string, error) {
	dir, err := s.configDir()
	if err != nil {
		return "", fmt.Errorf("find the user config directory: %w", err)
	}
	return filepath.Join(dir, "sparrow", "credentials.json"), nil
}

// save stores creds in the keychain. When the keychain fails it writes the file instead and
// returns a warning for the person; a successful keychain write removes any older file.
func (s credentialStore) save(creds credentials) (warning string, err error) {
	encoded, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("encode credentials: %w", err)
	}

	keyringErr := s.keyring.Set(keyringService, keyringUser, string(encoded))
	if keyringErr == nil {
		if err := s.removeFile(); err != nil {
			return "", err
		}
		return "", nil
	}

	path, err := s.path()
	if err != nil {
		return "", errors.Join(fmt.Errorf("keychain unavailable: %w", keyringErr), err)
	}
	// An older keychain entry would be read before the file, so it must not outlive this save.
	staleNote := ""
	if err := s.keyring.Delete(keyringService, keyringUser); err != nil && !errors.Is(err, errNoEntry) {
		staleNote = fmt.Sprintf("; could not check the keychain for an older login (%v)", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create credentials directory: %w", err)
	}
	if err := writePrivateFile(path, encoded); err != nil {
		return "", err
	}
	return fmt.Sprintf("warning: the keychain is unavailable (%v); the login token is stored in %s with owner-only permissions%s", keyringErr, path, staleNote), nil
}

// writePrivateFile writes data to a file only its owner can read.
func writePrivateFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write credentials file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("protect credentials file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write credentials file: %w", err)
	}
	return file.Close()
}

// keychainReadError reports that the keychain failed for a reason other than holding no entry,
// and that no fallback file made up for it.
type keychainReadError struct{ err error }

func (e *keychainReadError) Error() string {
	return fmt.Sprintf("not signed in, or the keychain could not be read (%v): run `sparrow login`", e.err)
}

func (e *keychainReadError) Unwrap() error { return e.err }

// load returns the stored login: the keychain's, else the file's. errNotSignedIn means there is
// none. A keychain failure is reported as a keychainReadError unless the file holds a login.
func (s credentialStore) load() (credentials, error) {
	secret, keyringErr := s.keyring.Get(keyringService, keyringUser)
	if keyringErr == nil {
		return decodeCredentials([]byte(secret))
	}
	missing := errNotSignedIn
	if !errors.Is(keyringErr, errNoEntry) {
		missing = &keychainReadError{err: keyringErr}
	}

	path, err := s.path()
	if err != nil {
		return credentials{}, missing
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return credentials{}, missing
	}
	if err != nil {
		return credentials{}, fmt.Errorf("read credentials file: %w", err)
	}
	return decodeCredentials(data)
}

// decodeCredentials parses and checks stored credentials.
func decodeCredentials(data []byte) (credentials, error) {
	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil || creds.URL == "" || creds.Token == "" {
		return credentials{}, errors.New("the stored login is damaged; run sparrow login again")
	}
	return creds, nil
}

// delete removes the login from the keychain and the file.
func (s credentialStore) delete() error {
	if err := s.keyring.Delete(keyringService, keyringUser); err != nil && !errors.Is(err, errNoEntry) {
		// A keychain that cannot be reached holds nothing the file fallback could not replace.
		if _, getErr := s.keyring.Get(keyringService, keyringUser); getErr == nil {
			return fmt.Errorf("delete the login from the keychain: %w", err)
		}
	}
	return s.removeFile()
}

// removeFile deletes the fallback credentials file when it exists.
func (s credentialStore) removeFile() error {
	path, err := s.path()
	if err != nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete credentials file: %w", err)
	}
	return nil
}
