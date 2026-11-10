package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendIsForTheDesktopApp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ShellFile), []byte("<!doctype html><title>app</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newFrontend(dir, "https://objects.example")
	const browser = "Mozilla/5.0 Firefox/130.0"

	tests := []struct {
		name      string
		host      string
		userAgent string
		want      int
	}{
		{"browser on a public host", "sparrow.example.com", browser, http.StatusForbidden},
		{"browser with no user agent", "sparrow.example.com", "", http.StatusForbidden},
		{"desktop app on a public host", "sparrow.example.com", "SuperSparrowDesktop/0.1.0", http.StatusOK},
		{"browser on localhost", "localhost:3000", browser, http.StatusOK},
		{"browser on 127.0.0.1", "127.0.0.1:8080", browser, http.StatusOK},
		{"browser on ::1", "[::1]:8080", browser, http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = test.host
			request.Header.Set("User-Agent", test.userAgent)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

// page requests a frontend page as the desktop app does.
func (f *fixture) page(path string) *httptest.ResponseRecorder {
	f.t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("User-Agent", DesktopAgentToken+"0.1.0")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)

	return response
}
