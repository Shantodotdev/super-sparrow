package httpserver

import (
	"net/http"
	"testing"
)

func TestProfileAPI(t *testing.T) {
	a := newAPITeam(t)
	var catalog struct {
		Avatars []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"avatars"`
	}
	a.call(http.MethodGet, "/api/avatars", a.member, nil, http.StatusOK, &catalog)
	if len(catalog.Avatars) == 0 || catalog.Avatars[0].URL != "/avatars/"+catalog.Avatars[0].ID+".png" {
		t.Fatalf("catalog %+v", catalog)
	}
	var profile struct {
		Actor actorView `json:"actor"`
	}
	a.call(http.MethodGet, "/api/me", a.member, nil, http.StatusOK, &profile)
	id := profile.Actor.ID
	a.call(http.MethodPatch, "/api/me", a.member, map[string]any{"display_name": "New Name", "avatar_id": catalog.Avatars[0].ID}, http.StatusOK, &profile)
	a.call(http.MethodGet, "/api/me", a.member, nil, http.StatusOK, &profile)
	if profile.Actor.ID != id || profile.Actor.DisplayName != "New Name" || profile.Actor.AvatarID != catalog.Avatars[0].ID {
		t.Fatalf("profile %+v", profile)
	}
	var people struct {
		Actors []actorView `json:"actors"`
	}
	a.call(http.MethodGet, "/api/people", a.member, nil, http.StatusOK, &people)
	found := false
	for _, actor := range people.Actors {
		if actor.ID == id && actor.AvatarID == profile.Actor.AvatarID {
			found = true
		}
	}
	if !found {
		t.Fatal("people omitted the persisted avatar")
	}
	wantField(t, a.request(http.MethodPatch, "/api/me", a.member, map[string]any{"avatar_id": "../bad"}), "avatar_id")
	wantField(t, a.request(http.MethodPatch, "/api/me", a.member, map[string]any{"display_name": " "}), "display_name")
	wantStatus(t, a.request(http.MethodPatch, "/api/me", a.member, map[string]any{"actor_id": "someone-else"}), http.StatusBadRequest)
	wantStatus(t, a.request(http.MethodPatch, "/api/me", a.agent, map[string]any{"display_name": "Agent"}), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodGet, "/api/avatars", a.agent, nil), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodPatch, "/api/me", "", map[string]any{"display_name": "Nobody"}), http.StatusUnauthorized)
	a.call(http.MethodPatch, "/api/me", a.member, map[string]any{"avatar_id": nil}, http.StatusOK, &profile)
	if profile.Actor.AvatarID != "" {
		t.Fatalf("avatar not cleared: %+v", profile.Actor)
	}
}
