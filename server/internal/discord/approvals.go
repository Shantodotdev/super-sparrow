package discord

import (
	"context"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// GuildApprovals returns the approvals visible to a workspace owner.
func (h *Bot) GuildApprovals(ctx context.Context) ([]store.DiscordGuildApproval, error) {
	return h.store.ListDiscordGuildApprovals(ctx, h.cfg.GuildIDs)
}

// SetGuildApproval applies an authenticated workspace owner's decision.
func (h *Bot) SetGuildApproval(ctx context.Context, caller store.Caller, id string, status store.DiscordGuildStatus) (store.DiscordGuildApproval, error) {
	approval, err := h.store.SetDiscordGuildApproval(ctx, caller, id, h.cfg.AllowsGuild(id), status)
	if err == nil && status == store.DiscordGuildRevoked {
		h.clearGuildMemory(id)
	}
	return approval, err
}

func (h *Bot) guildSource(guildID, userID string) store.DiscordIdentity {
	return store.DiscordIdentity{GuildID: guildID, UserID: userID, Configured: h.cfg.AllowsGuild(guildID)}
}

func (h *Bot) guildAccess(ctx context.Context, source store.DiscordIdentity) (store.DiscordGuildStatus, error) {
	return h.store.DiscordGuildAccess(ctx, source.GuildID, source.Configured)
}

// observeGuild records a newly installed or joined guild as pending approval.
func (h *Bot) observeGuild(ctx context.Context, guildID, name string) {
	source := h.guildSource(guildID, "")
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status, err := h.guildAccess(checkCtx, source)
	if err != nil {
		h.logger.WarnContext(checkCtx, "check observed Discord server approval", "error", err)
		return
	}
	if status != "" {
		return
	}

	normalized := shorten(strings.Join(strings.Fields(name), " "), 100)
	if err := h.store.RequestDiscordGuild(checkCtx, source, normalized); err != nil {
		h.logger.WarnContext(checkCtx, "queue observed Discord server approval", "error", err)
	}
}

// requestGuildAccess queues unknown guilds and sends at most one reminder per guild per window.
func (h *Bot) requestGuildAccess(ctx context.Context, request mention, status store.DiscordGuildStatus) {
	if allowed, _ := h.perPendingGuild.Allow(request.source.GuildID); !allowed {
		return
	}
	if status == "" {
		if err := h.store.RequestDiscordGuild(ctx, request.source, request.guildName); err != nil {
			h.logger.WarnContext(ctx, "queue Discord server approval", "error", err)
			return
		}
	}

	server := "This Discord server (ID " + request.source.GuildID + ")"
	if request.guildName != "" {
		server = "**" + escape(request.guildName) + "**"
	}
	h.deliverMention(ctx, request, plain(server+" needs approval. Ask a workspace admin to open Sparrow **Settings → Discord** and approve this server, then mention me again."))
}
