package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// cliFilePattern matches the files of a CLI release: its signed manifest, the signature, and one
// binary per platform. Nothing else in the folder is served.
var cliFilePattern = regexp.MustCompile(`^(manifest\.json|manifest\.json\.sig|sparrow-(linux|darwin|windows)-(amd64|arm64)(\.exe)?)$`)

// serveCLIFile serves one file of the latest signed CLI release. The CLI checks the manifest's
// signature itself, so the server only hands out what the release workflow signed.
func (s *server) serveCLIFile(w http.ResponseWriter, r *http.Request) error {
	name := r.PathValue("file")
	if s.cliDir == "" || !cliFilePattern.MatchString(name) {
		return store.Refuse(store.ErrNotFound, "this server offers no CLI file %q", name)
	}

	path := filepath.Join(s.cliDir, name)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return store.Refuse(store.ErrNotFound, "this server has no signed CLI release yet")
	}

	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
	return nil
}
