package store

import (
	"os"
	"strings"
	"testing"
)

func TestUpdateProfilePersistsAndRecordsActor(t *testing.T) {
	tm := newTeam(t)
	original := got(tm.store.GetActor(tm.ctx, tm.owner.ActorID)).must(t)
	updated := got(tm.store.UpdateProfile(tm.ctx, tm.member, ProfileUpdate{DisplayName: Change("  New Name  "), AvatarID: Change("adventurers-cat-pilot")})).must(t)
	loaded := got(tm.store.GetActor(tm.ctx, tm.member.ActorID)).must(t)
	if loaded.DisplayName != "New Name" || loaded.AvatarID != "adventurers-cat-pilot" || updated.ID != tm.member.ActorID {
		t.Fatalf("profile %+v", loaded)
	}
	owner := got(tm.store.GetActor(tm.ctx, tm.owner.ActorID)).must(t)
	if owner.DisplayName != original.DisplayName || owner.AvatarID != original.AvatarID {
		t.Fatal("changed another person's profile")
	}
	events := got(tm.store.ListEvents(tm.ctx, EventFilter{TargetType: TargetActor, TargetID: tm.member.ActorID, Limit: MaximumPageLimit})).must(t)
	found := false
	for _, event := range events {
		if event.Kind == "actor.updated" && event.ActorID == tm.member.ActorID && strings.Contains(string(event.Payload), "avatar_id") {
			found = true
		}
	}
	if !found {
		t.Fatal("profile update did not record the authenticated actor")
	}
	cleared := got(tm.store.UpdateProfile(tm.ctx, tm.member, ProfileUpdate{AvatarID: Change("")})).must(t)
	if cleared.AvatarID != "" || cleared.DisplayName != "New Name" {
		t.Fatalf("cleared %+v", cleared)
	}
	_, err := tm.store.UpdateProfile(tm.ctx, tm.agent, ProfileUpdate{DisplayName: Change("Not human")})
	wantErr(t, err, ErrForbidden)
	for _, id := range []string{"https://example.com/avatar.png", "../avatars/cat", "unknown"} {
		_, err := tm.store.UpdateProfile(tm.ctx, tm.member, ProfileUpdate{AvatarID: Change(id)})
		wantInvalid(t, err, "avatar_id")
	}
	_, err = tm.store.UpdateProfile(tm.ctx, tm.member, ProfileUpdate{DisplayName: Change("  ")})
	wantInvalid(t, err, "display_name")
}

func TestProfileAvatarCatalogMatchesAssets(t *testing.T) {
	entries, err := os.ReadDir("../../../web/public/avatars")
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string]bool{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".png") {
			assets[strings.TrimSuffix(entry.Name(), ".png")] = true
		}
	}
	ids := ProfileAvatarIDs()
	if len(ids) != len(assets) {
		t.Fatalf("catalog has %d choices for %d assets", len(ids), len(assets))
	}
	for _, id := range ids {
		if !assets[id] {
			t.Fatalf("missing avatar asset %s", id)
		}
		delete(assets, id)
	}
	if len(assets) != 0 {
		t.Fatalf("unlisted avatars: %v", assets)
	}
}
