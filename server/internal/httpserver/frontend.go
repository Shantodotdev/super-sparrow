package httpserver

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// ShellFile is the page TanStack Start's SPA build writes for the app to start from.
const ShellFile = "_shell.html"

// frontend serves the built single-page app from a folder. A path that names a file gets the
// file; any other path without a file extension gets the shell page, so the app's router can show
// it. Files under /assets/ have content hashes in their names and are cached for a year; the
// shell page is checked on every load so a new build reaches people at once.
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
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, f.files, ShellFile)
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
