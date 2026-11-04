package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendServesWebManifestType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.webmanifest"), []byte(`{"name":"Sparrow"}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	recorder := httptest.NewRecorder()
	newFrontend(dir).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/manifest+json") {
		t.Fatalf("Content-Type = %q, want application/manifest+json", got)
	}
}
