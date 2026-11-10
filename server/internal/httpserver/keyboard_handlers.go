package httpserver

import (
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"net/http"
)

// keyboardShortcuts returns only the session owner's shortcut preferences.
func (s *server) keyboardShortcuts(w http.ResponseWriter, r *http.Request) error {
	preferences, err := s.store.GetKeyboardShortcuts(r.Context(), identityFrom(r.Context()).caller())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, preferences)
	return nil
}

// updateKeyboardShortcut accepts one revision-checked personal binding; null restores its default.
func (s *server) updateKeyboardShortcut(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Revision *int    `json:"revision"`
		Action   string  `json:"action"`
		Binding  *string `json:"binding"`
	}
	fields, err := decodeUpdate(w, r, &body)
	if err != nil {
		return err
	}
	if body.Revision == nil || *body.Revision < 0 {
		return &store.ValidationError{Field: "revision", Message: "must be a non-negative integer"}
	}
	if body.Binding == nil && !fields["binding"] {
		return &store.ValidationError{Field: "binding", Message: "is required; use null to restore the default"}
	}
	preferences, err := s.store.UpdateKeyboardShortcut(r.Context(), identityFrom(r.Context()).caller(), *body.Revision, body.Action, body.Binding)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, preferences)
	return nil
}
