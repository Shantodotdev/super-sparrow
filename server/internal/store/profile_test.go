package store

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestUpdateProfilePersistsAndRecordsActor(t *testing.T) {
	tm := newTeam(t)
	original := got(tm.store.GetActor(tm.ctx, tm.owner.ActorID)).must(t)
	// New people get a random avatar, so pick one the member does not already have.
	before := got(tm.store.GetActor(tm.ctx, tm.member.ActorID)).must(t)
	avatar := ProfileAvatarIDs()[0]
	if before.AvatarID == avatar {
		avatar = ProfileAvatarIDs()[1]
	}
	updated := got(tm.store.UpdateProfile(tm.ctx, tm.member, ProfileUpdate{DisplayName: Change("  New Name  "), AvatarID: Change(avatar)})).must(t)
	loaded := got(tm.store.GetActor(tm.ctx, tm.member.ActorID)).must(t)
	if loaded.DisplayName != "New Name" || loaded.AvatarID != avatar || updated.ID != tm.member.ActorID {
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

// TestProfileAvatarVariants checks the versioned catalog for complete, bounded WebP assets.
func TestProfileAvatarVariants(t *testing.T) {
	for _, id := range ProfileAvatarIDs() {
		for _, size := range []int{48, 96, 192} {
			name := fmt.Sprintf("../../../web/public/avatars/v1/%s-%d.webp", id, size)
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || len(data) > 20000 {
				t.Errorf("invalid or oversized avatar variant %s: %d bytes", name, len(data))
			}
		}
	}
}

func TestNewPeopleGetUniqueDefaultAvatarsUntilExhausted(t *testing.T) {
	tm := newTeam(t)
	owner := got(tm.store.GetActor(tm.ctx, tm.owner.ActorID)).must(t)
	if owner.AvatarID == "" {
		t.Fatal("first owner has no default avatar")
	}

	seen := map[string]int{}
	people := 0
	for _, actor := range got(tm.store.ListActors(tm.ctx)).must(t) {
		if actor.Kind == ActorHuman {
			seen[actor.AvatarID]++
			people++
		}
	}
	for i := people; i < len(profileAvatarIDs); i++ {
		person := got(tm.store.CreateHuman(tm.ctx, tm.owner, NewHuman{DisplayName: "Person", Email: fmt.Sprintf("person%d@example.com", i), Role: RoleMember})).must(t)
		if person.AvatarID == "" || seen[person.AvatarID] > 0 {
			t.Fatalf("person %d got avatar %q, already used %d times", i, person.AvatarID, seen[person.AvatarID])
		}
		seen[person.AvatarID]++
	}

	extra := got(tm.store.CreateHuman(tm.ctx, tm.owner, NewHuman{DisplayName: "Extra", Email: "extra@example.com", Role: RoleMember})).must(t)
	if !slices.Contains(profileAvatarIDs, extra.AvatarID) {
		t.Fatalf("extra person got avatar %q", extra.AvatarID)
	}
}
