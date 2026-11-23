package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/config"
	"github.com/shantodotdev/super-sparrow/server/internal/discord"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func TestDiscordGuildApprovalsRequireWorkspaceOwner(t *testing.T) {
	a := newAPITeam(t)
	path := "/api/discord/guilds"
	type inventory struct {
		Enabled bool                         `json:"enabled"`
		Guilds  []store.DiscordGuildApproval `json:"guilds"`
	}
	var list inventory
	a.call(http.MethodGet, path, a.owner, nil, http.StatusOK, &list)
	if list.Enabled || list.Guilds == nil || len(list.Guilds) != 0 {
		t.Fatalf("disabled integration response: %+v", list)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publicURL, _ := url.Parse(origin)
	bot := discord.New(config.Discord{GuildIDs: []string{"123"}}, a.store, nil, logger, time.Now)
	a.handler = New(Options{Logger: logger, Store: a.store, Auth: a.auth, PublicURL: publicURL, Now: time.Now, Discord: bot})
	if err := a.store.RequestDiscordGuild(t.Context(), store.DiscordIdentity{GuildID: "456", UserID: "789"}, "Pending server"); err != nil {
		t.Fatal(err)
	}

	for _, credential := range []string{a.member, a.agent} {
		a.call(http.MethodGet, path, credential, nil, http.StatusForbidden, nil)
		a.call(http.MethodPost, path+"/456/approve", credential, nil, http.StatusForbidden, nil)
		a.call(http.MethodPost, path+"/123/revoke", credential, nil, http.StatusForbidden, nil)
	}

	a.call(http.MethodGet, path, a.owner, nil, http.StatusOK, &list)
	if !list.Enabled || len(list.Guilds) != 2 || list.Guilds[0].Status != store.DiscordGuildPending ||
		list.Guilds[0].Name != "Pending server" || list.Guilds[1].Status != store.DiscordGuildApproved {
		t.Fatalf("approval inventory: %+v", list)
	}
	a.call(http.MethodPost, path+"/999/approve", a.owner, nil, http.StatusNotFound, nil)

	var decision struct {
		Guild store.DiscordGuildApproval `json:"guild"`
	}
	a.call(http.MethodPost, path+"/456/approve", a.owner, nil, http.StatusOK, &decision)
	if decision.Guild.Status != store.DiscordGuildApproved {
		t.Fatalf("approve pending server: %+v", decision)
	}
	a.call(http.MethodPost, path+"/123/revoke", a.owner, nil, http.StatusOK, &decision)
	if decision.Guild.Status != store.DiscordGuildRevoked {
		t.Fatalf("revoke configured server: %+v", decision)
	}

	events, err := a.store.ListEvents(t.Context(), store.EventFilter{ActorID: a.fixture.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	var approved, revoked bool
	for _, event := range events {
		approved = approved || event.Kind == "discord.guild_approved"
		revoked = revoked || event.Kind == "discord.guild_revoked"
	}
	if !approved || !revoked {
		t.Fatal("approval decisions were not attributed to the owner")
	}
	response := a.request(http.MethodGet, "/api/events", a.member, nil)
	wantStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), "discord.guild_") {
		t.Fatal("approval activity was visible to a member")
	}
}
