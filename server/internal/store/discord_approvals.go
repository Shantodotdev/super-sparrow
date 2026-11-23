package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// DiscordGuildStatus describes a guild's observed approval state.
type DiscordGuildStatus string

const (
	DiscordGuildPending  DiscordGuildStatus = "pending"
	DiscordGuildApproved DiscordGuildStatus = "approved"
	DiscordGuildRevoked  DiscordGuildStatus = "revoked"
)

// DiscordGuildApproval is a guild's current authorization in the team's workspace.
type DiscordGuildApproval struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Status DiscordGuildStatus `json:"status"`
}

// DiscordGuildAccess resolves stored approval, falling back to the configured allowlist only
// for guilds with no stored decision. An unknown guild returns an empty status.
func (s *Store) DiscordGuildAccess(ctx context.Context, guildID string, configured bool) (DiscordGuildStatus, error) {
	return discordGuildAccess(ctx, s.db, guildID, configured)
}

func discordGuildAccess(ctx context.Context, q querier, guildID string, configured bool) (DiscordGuildStatus, error) {
	guild, err := findDiscordGuild(ctx, q, guildID, configured)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	return guild.Status, err
}

func findDiscordGuild(ctx context.Context, q querier, guildID string, configured bool) (DiscordGuildApproval, error) {
	guild := DiscordGuildApproval{ID: guildID}
	err := q.QueryRowContext(ctx, `SELECT name, status FROM discord_guild_approvals WHERE guild_id = ?`, guildID).
		Scan(&guild.Name, &guild.Status)
	if errors.Is(err, sql.ErrNoRows) {
		if configured {
			guild.Status = DiscordGuildApproved
			return guild, nil
		}
		return DiscordGuildApproval{}, notFound("Discord server")
	}
	if err != nil {
		return DiscordGuildApproval{}, fmt.Errorf("load Discord server approval: %w", err)
	}

	return guild, nil
}

func requireDiscordGuild(ctx context.Context, q querier, source DiscordIdentity) error {
	status, err := discordGuildAccess(ctx, q, source.GuildID, source.Configured)
	if err != nil {
		return err
	}
	if status != DiscordGuildApproved {
		return forbidden("this Discord server needs workspace approval")
	}
	return nil
}

// RequestDiscordGuild records one pending request from an authenticated Gateway mention.
func (s *Store) RequestDiscordGuild(ctx context.Context, source DiscordIdentity, name string) error {
	if err := validateDiscordGuildID(source.GuildID); err != nil {
		return err
	}
	name, err := optionalText("name", name, 100)
	if err != nil {
		return err
	}

	return s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		_, err := findDiscordGuild(ctx, w.tx, source.GuildID, source.Configured)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		_, err = w.exec(`INSERT INTO discord_guild_approvals (guild_id, name, status, created_at, updated_at)
			VALUES (?, ?, 'pending', ?, ?)`, source.GuildID, name, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("queue Discord server approval: %w", err)
		}

		payload := map[string]any{"guild_id": source.GuildID}
		if source.UserID != "" {
			payload["discord_user_id"] = source.UserID
		}
		return w.event("", TargetActor, SystemActorID, "discord.guild_requested", payload)
	})
}

// ListDiscordGuildApprovals returns durable guild records, supplemented by configured approvals.
func (s *Store) ListDiscordGuildApprovals(ctx context.Context, configured []string) ([]DiscordGuildApproval, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT guild_id, name, status FROM discord_guild_approvals ORDER BY created_at DESC, rowid DESC`)
	if err != nil {
		return nil, fmt.Errorf("list Discord server approvals: %w", err)
	}
	defer rows.Close()

	guilds := make([]DiscordGuildApproval, 0, len(configured))
	seen := make(map[string]struct{}, len(configured))
	for rows.Next() {
		var guild DiscordGuildApproval
		if err := rows.Scan(&guild.ID, &guild.Name, &guild.Status); err != nil {
			return nil, fmt.Errorf("read Discord server approval: %w", err)
		}
		guilds = append(guilds, guild)
		seen[guild.ID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Discord server approvals: %w", err)
	}

	for _, id := range configured {
		if _, ok := seen[id]; !ok {
			guilds = append(guilds, DiscordGuildApproval{ID: id, Status: DiscordGuildApproved})
		}
	}
	return guilds, nil
}

// SetDiscordGuildApproval lets an active owner approve or revoke a known guild.
func (s *Store) SetDiscordGuildApproval(ctx context.Context, caller Caller, guildID string, configured bool, status DiscordGuildStatus) (DiscordGuildApproval, error) {
	if err := validateDiscordGuildID(guildID); err != nil {
		return DiscordGuildApproval{}, err
	}
	if status != DiscordGuildApproved && status != DiscordGuildRevoked {
		return DiscordGuildApproval{}, invalid("status", "must be approved or revoked")
	}

	var guild DiscordGuildApproval
	err := s.write(ctx, caller, func(w *writeTx) error {
		if !w.caller.IsOwner() {
			return forbidden("only an owner can manage Discord server approvals")
		}
		var err error
		guild, err = findDiscordGuild(ctx, w.tx, guildID, configured)
		if err != nil {
			return err
		}
		if guild.Status == status {
			return nil
		}

		_, err = w.exec(`INSERT INTO discord_guild_approvals (guild_id, name, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT(guild_id) DO UPDATE
			SET status = excluded.status, updated_at = excluded.updated_at`,
			guildID, guild.Name, status, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("update Discord server approval: %w", err)
		}
		guild.Status = status

		kind := "discord.guild_approved"
		if status == DiscordGuildRevoked {
			kind = "discord.guild_revoked"
		}
		return w.event("", TargetActor, SystemActorID, kind, map[string]any{"guild_id": guildID})
	})
	if err != nil {
		return DiscordGuildApproval{}, err
	}
	return guild, nil
}

func validateDiscordGuildID(value string) error {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != value {
		return invalid("guild_id", "must be a Discord server ID")
	}
	return nil
}
