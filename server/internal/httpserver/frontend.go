package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

// ShellFile is the page TanStack Start's SPA build writes for the app to start from.
const ShellFile = "_shell.html"

var (
	// scriptPattern finds script elements, with their attributes and their text.
	scriptPattern = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
	// srcPattern finds a src attribute, which makes a script external rather than inline.
	srcPattern = regexp.MustCompile(`(?i)(?:^|\s)src\s*=`)
)

// frontend serves the built single-page app from a folder. A path that names a file gets the
// file; any other path without a file extension gets the shell page, so the app's router can show
// it. Files under /assets/ have content hashes in their names and are cached for a year; the
// shell page is checked on every load so a new build reaches people at once. The shell page is
// served with a Content-Security-Policy that allows only the app's own files and the inline
// scripts the build wrote into it, so markdown that slips past sanitizing cannot run scripts.
type frontend struct {
	files fs.FS
}

func newFrontend(dir string) *frontend {
	return &frontend{files: os.DirFS(dir)}
}

func (f *frontend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "DENY")

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || hiddenPath(name) {
		f.serveShell(w, r)
		return
	}

	info, err := fs.Stat(f.files, name)
	switch {
	case err == nil && !info.IsDir():
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFileFS(w, r, f.files, name)
	case (err == nil || errors.Is(err, fs.ErrNotExist)) && path.Ext(name) == "":
		f.serveShell(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *frontend) serveShell(w http.ResponseWriter, r *http.Request) {
	shell, err := fs.ReadFile(f.files, ShellFile)
	if err != nil {
		http.Error(w, "the frontend is not built", http.StatusInternalServerError)
		return
	}
	info, err := fs.Stat(f.files, ShellFile)
	if err != nil {
		http.Error(w, "the frontend is not built", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy(shell))
	http.ServeContent(w, r, ShellFile, info.ModTime(), bytes.NewReader(shell))
}

// contentSecurityPolicy allows the app's own files and, by their SHA-256 hashes, the inline
// scripts in shell, which TanStack Start writes at build time. Hashing them when the page is
// served keeps the policy right after every build without listing them by hand.
func contentSecurityPolicy(shell []byte) string {
	scripts := []string{"'self'"}
	for _, match := range scriptPattern.FindAllSubmatch(shell, -1) {
		attributes, text := match[1], match[2]
		if srcPattern.Match(attributes) || len(text) == 0 {
			continue
		}
		sum := sha256.Sum256(text)
		scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}

	return fmt.Sprintf("default-src 'self'; script-src %s; style-src 'self'; img-src 'self' data:; font-src 'self'; "+
		"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'",
		strings.Join(scripts, " "))
}

// hiddenPath reports whether any part of name starts with a dot, such as .env, which is never served.
func hiddenPath(name string) bool {
	for part := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}
