package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const discordAgentName = "discord"

const maxDiscordUserName = 100

// DiscordIdentity identifies the account from a verified guild interaction.
type DiscordIdentity struct {
	GuildID string
	UserID  string
	// Configured comes from Sparrow’s Discord configuration, outside the event payload.
	Configured bool
}

// DiscordSetup changes a mapping, a default project, or both, in one transaction.
type DiscordSetup struct {
	InteractionID string
	UserID        string
	// UserName is the linked member's Discord name, shown to owners reviewing links.
	UserName   string
	PersonID   string
	ProjectKey string
}

// DiscordLink is one Discord member's link to a Sparrow teammate, as an owner reviews it.
type DiscordLink struct {
	GuildID    string `json:"guild_id"`
	GuildName  string `json:"guild_name"`
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	PersonID   string `json:"person_id"`
	PersonName string `json:"person_name"`
}

// SaveDiscordSetup applies setup on behalf of the verified Discord user, including users who
// have no Sparrow link yet. A linked user's change runs as their Discord agent and an unlinked
// user's as the system. The event preserves the acting Discord ID independently of the link.
func (s *Store) SaveDiscordSetup(ctx context.Context, source DiscordIdentity, input DiscordSetup) error {
	userName, err := optionalText("discord_user_name", input.UserName, maxDiscordUserName)
	if err != nil {
		return err
	}
	input.UserName = userName

	by := Caller{ActorID: SystemActorID}
	if agentID, err := discordAgentID(ctx, s.db, source); err == nil {
		by = Caller{ActorID: agentID, Client: ClientDiscord, Discord: source}
	} else if !errors.Is(err, ErrForbidden) {
		return err
	}

	return s.write(ctx, by, func(w *writeTx) error {
		if err := requireDiscordGuild(ctx, w.tx, source); err != nil {
			return err
		}

		var applied bool
		if err := w.tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM events
			WHERE kind = 'discord.setup' AND json_extract(payload, '$.interaction_id') = ?)`, input.InteractionID).Scan(&applied); err != nil {
			return fmt.Errorf("check Discord setup receipt: %w", err)
		}
		if applied {
			return nil
		}

		payload := map[string]any{"guild_id": source.GuildID, "discord_user_id": source.UserID, "interaction_id": input.InteractionID}
		if input.PersonID != "" {
			agent, previous, err := linkDiscordPerson(w, source, DiscordMemberName{input.UserID, input.UserName}, input.PersonID)
			if err != nil {
				return err
			}
			payload["linked_discord_user_id"], payload["linked_agent_id"] = input.UserID, agent.ID
			if previous != "" {
				payload["previous_agent_id"] = previous
			}
		}

		if input.ProjectKey != "" {
			project, err := getProject(ctx, w.tx, "key = ?", input.ProjectKey)
			if err != nil {
				return err
			}
			_, err = w.exec(`INSERT INTO discord_settings (guild_id, default_project_id) VALUES (?, ?)
				ON CONFLICT(guild_id) DO UPDATE SET default_project_id = excluded.default_project_id`, source.GuildID, project.ID)
			if err != nil {
				return fmt.Errorf("save default Discord project: %w", err)
			}
			payload["default_project_id"] = project.ID
		}

		return w.event("", TargetActor, SystemActorID, "discord.setup", payload)
	})
}

// DiscordMemberName is the Discord account a link points at.
type DiscordMemberName struct {
	UserID string
	Name   string
}

// linkDiscordPerson finds or creates the teammate's Discord agent, updates the link, and returns
// the agent the member was linked to before. A second Discord account cannot reuse an agent
// that is already linked in this guild, and changing an existing account's mapping requires
// that member or a linked workspace owner.
func linkDiscordPerson(w *writeTx, source DiscordIdentity, member DiscordMemberName, personID string) (Actor, string, error) {
	userID := member.UserID
	person, err := getActor(w.ctx, w.tx, personID)
	if err != nil {
		return Actor{}, "", err
	}
	if person.Kind != ActorHuman || !person.Active() {
		return Actor{}, "", invalid("teammate", "must be an active Sparrow teammate")
	}

	agent, err := scanActor(w.tx.QueryRowContext(w.ctx, `SELECT `+actorColumns+
		` FROM actors WHERE kind = 'agent' AND owner_actor_id = ? AND agent_name = ?`, person.ID, discordAgentName))
	if errors.Is(err, ErrNotFound) {
		agent = Actor{ID: newID(), Kind: ActorAgent, DisplayName: "Discord", AgentName: discordAgentName, OwnerActorID: person.ID}
		err = insertAgent(w, &agent)
	}
	if err != nil {
		return Actor{}, "", err
	}
	if !agent.Active() {
		return Actor{}, "", forbidden("this Discord agent is disabled")
	}

	var existingAgentID string
	err = w.tx.QueryRowContext(w.ctx, `SELECT agent_id FROM discord_links WHERE guild_id = ? AND user_id = ?`,
		source.GuildID, userID).Scan(&existingAgentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Actor{}, "", fmt.Errorf("read existing Discord link: %w", err)
	}
	if existingAgentID != "" && existingAgentID != agent.ID && source.UserID != userID {
		owner, err := discordUserIsOwner(w.ctx, w.tx, source)
		if err != nil {
			return Actor{}, "", err
		}
		if !owner {
			return Actor{}, "", forbidden("only a workspace owner or that Discord member can change an existing teammate mapping")
		}
	}

	_, err = w.exec(`INSERT INTO discord_links (guild_id, user_id, agent_id, user_name) VALUES (?, ?, ?, ?)
		ON CONFLICT(guild_id, user_id) DO UPDATE SET agent_id = excluded.agent_id, user_name = excluded.user_name`,
		source.GuildID, userID, agent.ID, member.Name)
	if err != nil {
		return Actor{}, "", mapUniqueViolation(err, "this teammate is already linked to another Discord account", "save Discord link")
	}
	return agent, existingAgentID, nil
}

// ListDiscordLinks returns every Discord member link with the teammate it acts for.
func (s *Store) ListDiscordLinks(ctx context.Context) ([]DiscordLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.guild_id, COALESCE(g.name, ''), d.user_id, d.user_name, p.id, p.display_name
		FROM discord_links d JOIN actors a ON a.id = d.agent_id JOIN actors p ON p.id = a.owner_actor_id
		LEFT JOIN discord_guild_approvals g ON g.guild_id = d.guild_id
		ORDER BY d.guild_id, p.display_name, d.user_id`)
	if err != nil {
		return nil, fmt.Errorf("list Discord links: %w", err)
	}
	defer rows.Close()

	links := []DiscordLink{}
	for rows.Next() {
		var link DiscordLink
		if err := rows.Scan(&link.GuildID, &link.GuildName, &link.UserID, &link.UserName, &link.PersonID, &link.PersonName); err != nil {
			return nil, fmt.Errorf("read Discord link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Discord links: %w", err)
	}
	return links, nil
}

// UnlinkDiscordMember lets an owner in the desktop app remove a Discord member's link.
func (s *Store) UnlinkDiscordMember(ctx context.Context, by Caller, guildID, userID string) error {
	if err := validateDiscordGuildID(guildID); err != nil {
		return err
	}
	return s.write(ctx, by, func(w *writeTx) error {
		if !w.caller.IsOwner() || w.client != ClientBrowser {
			return forbidden("only an owner in the desktop app can remove Discord links")
		}
		var agentID string
		err := w.tx.QueryRowContext(ctx, `DELETE FROM discord_links WHERE guild_id = ? AND user_id = ? RETURNING agent_id`,
			guildID, userID).Scan(&agentID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("Discord link")
		}
		if err != nil {
			return fmt.Errorf("remove Discord link: %w", err)
		}
		return w.event("", TargetActor, agentID, "discord.unlinked", map[string]any{
			"guild_id": guildID, "discord_user_id": userID, "agent_id": agentID,
		})
	})
}

func discordUserIsOwner(ctx context.Context, q querier, source DiscordIdentity) (bool, error) {
	var owner bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM discord_links d
		JOIN actors a ON a.id = d.agent_id
		JOIN actors p ON p.id = a.owner_actor_id
		WHERE d.guild_id = ? AND d.user_id = ? AND a.kind = 'agent' AND a.agent_name = ?
		AND a.disabled_at IS NULL AND p.kind = 'human' AND p.role = 'owner' AND p.disabled_at IS NULL)`,
		source.GuildID, source.UserID, discordAgentName).Scan(&owner)
	if err != nil {
		return false, fmt.Errorf("check Discord owner mapping: %w", err)
	}
	return owner, nil
}

// DiscordCaller resolves the current link and default project for a verified Discord user.
func (s *Store) DiscordCaller(ctx context.Context, source DiscordIdentity, model string) (Caller, string, error) {
	if err := requireDiscordGuild(ctx, s.db, source); err != nil {
		return Caller{}, "", err
	}
	agentID, err := discordAgentID(ctx, s.db, source)
	if err != nil {
		return Caller{}, "", err
	}
	var projectKey string
	err = s.db.QueryRowContext(ctx, `SELECT p.key FROM discord_settings d
		JOIN projects p ON p.id = d.default_project_id WHERE d.guild_id = ?`, source.GuildID).Scan(&projectKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Caller{}, "", fmt.Errorf("read default Discord project: %w", err)
	}
	return Caller{ActorID: agentID, Client: ClientDiscord, Discord: source, ReportedModel: model}, projectKey, nil
}

// discordAgentID only returns an active agent linked to an active teammate.
func discordAgentID(ctx context.Context, q querier, source DiscordIdentity) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT a.id FROM discord_links d JOIN actors a ON a.id = d.agent_id
		JOIN actors p ON p.id = a.owner_actor_id
		WHERE d.guild_id = ? AND d.user_id = ? AND a.kind = 'agent' AND a.agent_name = ?
		AND a.disabled_at IS NULL AND p.kind = 'human' AND p.disabled_at IS NULL`,
		source.GuildID, source.UserID, discordAgentName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", forbidden("your Discord account needs an active Sparrow teammate mapping")
	}
	if err != nil {
		return "", fmt.Errorf("load Discord agent: %w", err)
	}
	return id, nil
}
