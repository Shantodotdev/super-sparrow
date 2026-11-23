package httpserver

import (
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// listDiscordGuilds returns the configured integration state and the owner's approval queue.
func (s *server) listDiscordGuilds(w http.ResponseWriter, r *http.Request) error {
	guilds := []store.DiscordGuildApproval{}
	enabled := s.discord != nil
	if enabled {
		var err error
		guilds, err = s.discord.GuildApprovals(r.Context())
		if err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "guilds": guilds})
	return nil
}

func (s *server) approveDiscordGuild(w http.ResponseWriter, r *http.Request) error {
	return s.setDiscordGuildApproval(w, r, store.DiscordGuildApproved)
}

func (s *server) revokeDiscordGuild(w http.ResponseWriter, r *http.Request) error {
	return s.setDiscordGuildApproval(w, r, store.DiscordGuildRevoked)
}

func (s *server) setDiscordGuildApproval(w http.ResponseWriter, r *http.Request, status store.DiscordGuildStatus) error {
	if s.discord == nil {
		return store.Refuse(store.ErrNotFound, "Discord integration is disabled")
	}
	caller := identityFrom(r.Context()).caller()
	guild, err := s.discord.SetGuildApproval(r.Context(), caller, r.PathValue("id"), status)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"guild": guild})
	return nil
}

func (s *server) listDiscordLinks(w http.ResponseWriter, r *http.Request) error {
	links, err := s.store.ListDiscordLinks(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
	return nil
}

func (s *server) unlinkDiscordMember(w http.ResponseWriter, r *http.Request) error {
	caller := identityFrom(r.Context()).caller()
	if err := s.store.UnlinkDiscordMember(r.Context(), caller, r.PathValue("guild"), r.PathValue("user")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
