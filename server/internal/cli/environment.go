package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// OpenURL opens a link with the operating system, such as the desktop app's approval screen.
	OpenURL func(string) error
	// LookPath finds a program on PATH, such as claude or codex.
	LookPath func(string) (string, error)
	// Command runs a program and returns its combined output.
	Command func(ctx context.Context, name string, args ...string) ([]byte, error)
	// ReleaseServerURL is the server a release build was made for, or "" for a development build.
	ReleaseServerURL string
}

// releaseServerURL is set at release build time with -ldflags "-X ...cli.releaseServerURL=URL", so
// `sparrow login` knows its server without a flag. Development builds leave it empty.
var releaseServerURL string

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
		OpenURL:    openURL,
		LookPath:   exec.LookPath,
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		ReleaseServerURL: releaseServerURL,
	}
}

// serverURL picks the server to sign in to: the --url flag, then SPARROW_URL, then the server
// this release was built for.
func (e *Environment) serverURL(flagValue string) (*url.URL, error) {
	for _, candidate := range []struct{ source, value string }{
		{"--url", flagValue}, {"SPARROW_URL", e.Getenv("SPARROW_URL")}, {"this build's server", e.ReleaseServerURL},
	} {
		if strings.TrimSpace(candidate.value) == "" {
			continue
		}
		parsed, err := parseServerURL(candidate.value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", candidate.source, err)
		}
		return parsed, nil
	}
	return nil, usageError("this build does not know its server: pass --url or set SPARROW_URL, such as https://sparrow.example.com")
}

// openURL opens link with the operating system's handler. The link is passed as one argument and
// never through a shell.
func openURL(link string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", link)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		command = exec.Command("xdg-open", link)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
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
