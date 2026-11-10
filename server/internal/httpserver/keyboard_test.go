package httpserver

import (
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"net/http"
	"testing"
)

func TestKeyboardAPI(t *testing.T) {
	a := newAPITeam(t)
	var saved store.KeyboardShortcuts
	a.call(http.MethodGet, "/api/me/shortcuts", a.member, nil, http.StatusOK, &saved)
	if saved.Revision != 0 || saved.Overrides == nil {
		t.Fatalf("initial %+v", saved)
	}
	a.call(http.MethodPatch, "/api/me/shortcuts", a.member, map[string]any{"revision": 0, "action": "quickCapture", "binding": "Ctrl+Alt+K"}, http.StatusOK, &saved)
	a.call(http.MethodGet, "/api/me/shortcuts", a.member, nil, http.StatusOK, &saved)
	if saved.Revision != 1 || saved.Overrides["quickCapture"] != "Ctrl+Alt+K" {
		t.Fatalf("saved %+v", saved)
	}
	var owner store.KeyboardShortcuts
	a.call(http.MethodGet, "/api/me/shortcuts", a.owner, nil, http.StatusOK, &owner)
	if owner.Revision != 0 || len(owner.Overrides) != 0 {
		t.Fatalf("another account changed %+v", owner)
	}
	wantStatus(t, a.request(http.MethodPatch, "/api/me/shortcuts", a.member, map[string]any{"revision": 0, "action": "quickCapture", "binding": nil}), http.StatusConflict)
	for _, body := range []map[string]any{
		{"revision": 1, "action": "quickCapture"},
		{"revision": 1, "action": "quickCapture", "binding": "Q"},
		{"revision": 1, "action": "newIssue", "binding": "J"},
		{"revision": 1, "action": "unknown", "binding": "Ctrl+Y"},
		{"revision": 1, "action": "quickCapture", "binding": "Ctrl+Alt+K", "actor_id": "other"},
	} {
		wantStatus(t, a.request(http.MethodPatch, "/api/me/shortcuts", a.member, body), http.StatusBadRequest)
	}
	wantStatus(t, a.request(http.MethodGet, "/api/me/shortcuts", a.agent, nil), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodPatch, "/api/me/shortcuts", a.agent, map[string]any{"revision": 0, "action": "quickCapture", "binding": nil}), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodGet, "/api/me/shortcuts", "", nil), http.StatusUnauthorized)
	saved = store.KeyboardShortcuts{}
	a.call(http.MethodPatch, "/api/me/shortcuts", a.member, map[string]any{"revision": 1, "action": "quickCapture", "binding": nil}, http.StatusOK, &saved)
	if len(saved.Overrides) != 0 {
		t.Fatalf("reset %+v", saved)
	}
}
