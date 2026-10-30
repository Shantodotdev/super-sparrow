package httpserver

import (
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"net/http"
	"strings"
)

// updateProfile changes only the person identified by the authenticated session.
func (s *server) updateProfile(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		DisplayName *string `json:"display_name"`
		AvatarID    *string `json:"avatar_id"`
	}
	nulls, err := decodeUpdate(w, r, &body)
	if err != nil {
		return err
	}
	actor, err := s.store.UpdateProfile(r.Context(), identityFrom(r.Context()).caller(), store.ProfileUpdate{
		DisplayName: patch(body.DisplayName, nulls["display_name"]),
		AvatarID:    patch(body.AvatarID, nulls["avatar_id"]),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"actor": newActorView(actor)})
	return nil
}

// listProfileAvatars lists the built-in avatar assets accepted by profile updates.
func (s *server) listProfileAvatars(w http.ResponseWriter, r *http.Request) error {
	type avatarView struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	avatars := []avatarView{}
	for _, id := range store.ProfileAvatarIDs() {
		avatars = append(avatars, avatarView{ID: id, Name: strings.ReplaceAll(id, "-", " "), URL: "/avatars/" + id + ".png"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatars": avatars})
	return nil
}
