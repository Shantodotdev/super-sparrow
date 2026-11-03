package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/release"
)

// Environment is everything the client commands take from the outside world, so tests can replace
// the keychain, the file system locations, and the process environment.
type Environment struct {
	Getenv     func(string) string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Version    string
	Keyring    Keyring
	ConfigDir  func() (string, error)
	Executable func() (string, error)
	Hostname   func() (string, error)
	PublicKeys []string
	HTTPClient *http.Client
}

// DefaultEnvironment returns the environment of the running process. version is the build's version.
func DefaultEnvironment(version string) *Environment {
	return &Environment{
		Getenv:    os.Getenv,
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Version:   version,
		Keyring:   systemKeyring{},
		ConfigDir: os.UserConfigDir,
		Executable: func() (string, error) {
			path, err := os.Executable()
			if err != nil {
				return "", err
			}
			return filepath.EvalSymlinks(path)
		},
		Hostname:   os.Hostname,
		PublicKeys: release.PublicKeys,
		HTTPClient: &http.Client{},
	}
}

// parseServerURL checks a server address: https, or http only for a server on this computer.
func parseServerURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("the server address is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%q is not a server address such as https://sparrow.example.com", raw)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("the server address must not hold a user, query, or fragment")
	}

	switch parsed.Scheme {
	case "https":
	case "http":
		switch parsed.Hostname() {
		case "localhost", "127.0.0.1", "::1":
		default:
			return nil, fmt.Errorf("refusing plain http to %s: use https (http is allowed only for localhost)", parsed.Hostname())
		}
	default:
		return nil, errors.New("the server address must start with https://")
	}
	return parsed, nil
}
