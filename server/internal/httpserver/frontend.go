package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

// DesktopAgentToken is the product token the desktop app puts in its User-Agent. The server hands
// the app's pages only to requests that carry it, or that arrive on a local development address.
const DesktopAgentToken = "SuperSparrowDesktop/"

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
// it. Files under /assets/ have content hashes and /avatars/v1/ has explicitly versioned images. Both
// are cached for a year; the
// shell page is checked on every load so a new build reaches people at once. The shell page is
// served with a Content-Security-Policy that allows only the app's own files and the inline
// scripts the build wrote into it, so markdown that slips past sanitizing cannot run scripts.
type frontend struct {
	files fs.FS
	// objectOrigin is the address of the attachments bucket, which the page may load files from
	// and send uploads to.
	objectOrigin string
}

func newFrontend(dir, objectOrigin string) *frontend {
	return &frontend{files: os.DirFS(dir), objectOrigin: objectOrigin}
}

func (f *frontend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !desktopRequest(r) {
		serveDesktopOnly(w)
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
		if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "avatars/v1/") {
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

// desktopRequest reports whether a page request comes from the desktop app, or from a browser on
// a local development address. The marker is not a secret, so this keeps browsers away from the
// pages but is no protection for the API, which every request still has to authenticate for.
func desktopRequest(r *http.Request) bool {
	if strings.Contains(r.UserAgent(), DesktopAgentToken) {
		return true
	}
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = strings.Trim(r.Host, "[]")
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// serveDesktopOnly answers a browser with a short page that points to the desktop app.
func serveDesktopOnly(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprint(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">`+
		`<title>Super Sparrow</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#07080a;color:#aebed4;font:15px system-ui,sans-serif}`+
		`main{max-width:360px;padding:24px;text-align:center}h1{margin:0 0 8px;font-size:20px;color:#eef0f5}p{margin:0}</style>`+
		`<main><h1>Open the desktop app</h1><p>Super Sparrow no longer runs in a browser. Sign in with the Super Sparrow desktop app.</p></main></html>`)
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
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy(shell, f.objectOrigin))
	http.ServeContent(w, r, ShellFile, info.ModTime(), bytes.NewReader(shell))
}

// contentSecurityPolicy allows the app's own files and, by their SHA-256 hashes, the inline
// scripts in shell, which TanStack Start writes at build time. Hashing them when the page is
// served keeps the policy right after every build without listing them by hand.
// The desktop app's command bridge and the attachments bucket are the only other addresses the
// page may reach.
func contentSecurityPolicy(shell []byte, objectOrigin string) string {
	scripts := []string{"'self'"}
	for _, match := range scriptPattern.FindAllSubmatch(shell, -1) {
		attributes, text := match[1], match[2]
		if srcPattern.Match(attributes) || len(text) == 0 {
			continue
		}
		sum := sha256.Sum256(text)
		scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}

	return fmt.Sprintf("default-src 'self'; script-src %[1]s; style-src 'self'; img-src 'self' data: blob: %[2]s; media-src 'self' %[2]s; font-src 'self'; "+
		"connect-src 'self' %[2]s ipc: http://ipc.localhost; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'",
		strings.Join(scripts, " "), objectOrigin)
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
