package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const discordAgentName = "discord"

// DiscordLinkCodeLifetime is how long a Discord link code can be redeemed after it is created.
const DiscordLinkCodeLifetime = 10 * time.Minute

const maxDiscordUserName = 100

// DiscordIdentity identifies the account from a verified guild interaction.
type DiscordIdentity struct {
	GuildID string
	UserID  string
	// Configured comes from Sparrow’s Discord configuration, outside the event payload.
	Configured bool
}

// DiscordMember is a Discord account in the guild that a link points at.
type DiscordMember struct {
	UserID string
	Name   string
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

// CreateDiscordLinkCode stores the hash of a new link code for the signed-in person and returns
// when it expires. A new code replaces the person's earlier one. Codes are credentials, not team
// data, so creating one writes no event; redeeming one does.
func (s *Store) CreateDiscordLinkCode(ctx context.Context, by Caller, codeHash []byte) (time.Time, error) {
	if len(codeHash) != hashLength {
		return time.Time{}, fmt.Errorf("link code hash for Discord must be %d bytes", hashLength)
	}

	var expires time.Time
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("create a Discord link code"); err != nil {
			return err
		}
		if w.client != ClientBrowser {
			return forbidden("Discord link codes are created in the desktop app")
		}

		if _, err := w.exec(`DELETE FROM discord_link_codes WHERE person_id = ? OR expires_at <= ?`,
			w.caller.ID, w.timestamp()); err != nil {
			return fmt.Errorf("remove old Discord link codes: %w", err)
		}
		expires = w.now.Add(DiscordLinkCodeLifetime)
		_, err := w.exec(`INSERT INTO discord_link_codes (code_hash, person_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
			codeHash, w.caller.ID, expires.Format(timeLayout), w.timestamp())
		if err != nil {
			return fmt.Errorf("save Discord link code: %w", err)
		}
		return nil
	})
	return expires, err
}

// RedeemDiscordLinkCode links the verified Discord user to the person who created the code. The
// code proves the Sparrow side and the signed Gateway event proves the Discord side, so the link
// replaces the member's earlier one and moves the person's link away from any other account in
// the guild. A repeated delivery of the same message returns the person it already linked.
func (s *Store) RedeemDiscordLinkCode(ctx context.Context, source DiscordIdentity, interactionID, userName string, codeHash []byte) (Actor, error) {
	userName, err := optionalText("discord_user_name", userName, maxDiscordUserName)
	if err != nil {
		return Actor{}, err
	}

	var person Actor
	err = s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		if err := requireDiscordGuild(ctx, w.tx, source); err != nil {
			return err
		}
		receipt, applied, err := discordSetupReceipt(w, interactionID)
		if err != nil {
			return err
		}
		if applied {
			person, err = getActor(ctx, w.tx, receipt.PersonID)
			return err
		}

		// Each code is used once. A wrong or expired code matches nothing.
		var personID string
		err = w.tx.QueryRowContext(ctx, `DELETE FROM discord_link_codes WHERE code_hash = ? AND expires_at > ?
			RETURNING person_id`, codeHash, w.timestamp()).Scan(&personID)
		if errors.Is(err, sql.ErrNoRows) {
			return forbidden("that link code is wrong or has expired. Get a new one in Sparrow Settings → Discord")
		}
		if err != nil {
			return fmt.Errorf("redeem Discord link code: %w", err)
		}
		person, err = activeTeammate(w, personID)
		if err != nil {
			return err
		}
		agent, err := discordAgentFor(w, person)
		if err != nil {
			return err
		}

		previous, err := linkedAgent(w, source.GuildID, source.UserID)
		if err != nil {
			return err
		}
		var displaced string
		err = w.tx.QueryRowContext(ctx, `DELETE FROM discord_links WHERE guild_id = ? AND agent_id = ? AND user_id <> ?
			RETURNING user_id`, source.GuildID, agent.ID, source.UserID).Scan(&displaced)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("move Discord link: %w", err)
		}
		if err := saveDiscordLink(w, source.GuildID, DiscordMember{UserID: source.UserID, Name: userName}, agent); err != nil {
			return err
		}

		payload := map[string]any{
			"guild_id": source.GuildID, "discord_user_id": source.UserID, "interaction_id": interactionID, "method": "code",
			"person_id": person.ID, "linked_discord_user_id": source.UserID, "linked_agent_id": agent.ID,
		}
		if previous != "" {
			payload["previous_agent_id"] = previous
		}
		if displaced != "" {
			payload["displaced_discord_user_id"] = displaced
		}
		return w.event("", TargetActor, agent.ID, "discord.setup", payload)
	})
	return person, err
}

// LinkDiscordMember lets a Discord caller linked to an active owner link another member of the
// guild after confirming it. The change runs as the owner's Discord agent.
func (s *Store) LinkDiscordMember(ctx context.Context, by Caller, interactionID string, member DiscordMember, personID string) (Actor, error) {
	name, err := optionalText("discord_user_name", member.Name, maxDiscordUserName)
	if err != nil {
		return Actor{}, err
	}
	member.Name = name

	var person Actor
	err = s.write(ctx, by, func(w *writeTx) error {
		if w.client != ClientDiscord {
			return forbidden("Discord members are linked from Discord")
		}
		owner, err := discordUserIsOwner(ctx, w.tx, w.discord)
		if err != nil {
			return err
		}
		if !owner {
			return forbidden("only a workspace owner can link another Discord member. They can link themselves with a code from Sparrow Settings → Discord")
		}
		receipt, applied, err := discordSetupReceipt(w, interactionID)
		if err != nil {
			return err
		}
		if applied {
			person, err = getActor(ctx, w.tx, receipt.PersonID)
			return err
		}

		person, err = activeTeammate(w, personID)
		if err != nil {
			return err
		}
		agent, err := discordAgentFor(w, person)
		if err != nil {
			return err
		}
		previous, err := linkedAgent(w, w.discord.GuildID, member.UserID)
		if err != nil {
			return err
		}
		if err := saveDiscordLink(w, w.discord.GuildID, member, agent); err != nil {
			return err
		}

		payload := map[string]any{
			"guild_id": w.discord.GuildID, "discord_user_id": w.discord.UserID, "interaction_id": interactionID, "method": "owner",
			"person_id": person.ID, "linked_discord_user_id": member.UserID, "linked_agent_id": agent.ID,
		}
		if previous != "" {
			payload["previous_agent_id"] = previous
		}
		return w.event("", TargetActor, agent.ID, "discord.setup", payload)
	})
	return person, err
}

// SetDiscordDefaultProject changes the guild's default project for a linked Discord caller who
// confirmed it. The change runs as the caller's Discord agent.
func (s *Store) SetDiscordDefaultProject(ctx context.Context, by Caller, interactionID, projectKey string) (Project, error) {
	var project Project
	err := s.write(ctx, by, func(w *writeTx) error {
		if w.client != ClientDiscord {
			return forbidden("the Discord default project is set from Discord")
		}
		var err error
		project, err = getProject(ctx, w.tx, "key = ?", projectKey)
		if err != nil {
			return err
		}
		_, applied, err := discordSetupReceipt(w, interactionID)
		if err != nil || applied {
			return err
		}

		_, err = w.exec(`INSERT INTO discord_settings (guild_id, default_project_id) VALUES (?, ?)
			ON CONFLICT(guild_id) DO UPDATE SET default_project_id = excluded.default_project_id`, w.discord.GuildID, project.ID)
		if err != nil {
			return fmt.Errorf("save default Discord project: %w", err)
		}
		return w.event("", TargetActor, w.caller.ID, "discord.setup", map[string]any{
			"guild_id": w.discord.GuildID, "discord_user_id": w.discord.UserID, "interaction_id": interactionID,
			"default_project_id": project.ID,
		})
	})
	return project, err
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

// DiscordCallerIsOwner reports whether the verified Discord user is linked to an active owner.
func (s *Store) DiscordCallerIsOwner(ctx context.Context, source DiscordIdentity) (bool, error) {
	return discordUserIsOwner(ctx, s.db, source)
}

type discordReceipt struct {
	PersonID string
}

// discordSetupReceipt finds the setup an earlier delivery of the same interaction applied.
func discordSetupReceipt(w *writeTx, interactionID string) (discordReceipt, bool, error) {
	var personID sql.NullString
	err := w.tx.QueryRowContext(w.ctx, `SELECT json_extract(payload, '$.person_id') FROM events
		WHERE kind = 'discord.setup' AND json_extract(payload, '$.interaction_id') = ?`, interactionID).Scan(&personID)
	if errors.Is(err, sql.ErrNoRows) {
		return discordReceipt{}, false, nil
	}
	if err != nil {
		return discordReceipt{}, false, fmt.Errorf("check Discord setup receipt: %w", err)
	}
	return discordReceipt{PersonID: personID.String}, true, nil
}

func activeTeammate(w *writeTx, personID string) (Actor, error) {
	person, err := getActor(w.ctx, w.tx, personID)
	if err != nil {
		return Actor{}, err
	}
	if person.Kind != ActorHuman || !person.Active() {
		return Actor{}, invalid("teammate", "must be an active Sparrow teammate")
	}
	return person, nil
}

// discordAgentFor finds or creates the teammate's Discord agent.
func discordAgentFor(w *writeTx, person Actor) (Actor, error) {
	agent, err := scanActor(w.tx.QueryRowContext(w.ctx, `SELECT `+actorColumns+
		` FROM actors WHERE kind = 'agent' AND owner_actor_id = ? AND agent_name = ?`, person.ID, discordAgentName))
	if errors.Is(err, ErrNotFound) {
		agent = Actor{ID: newID(), Kind: ActorAgent, DisplayName: "Discord", AgentName: discordAgentName, OwnerActorID: person.ID}
		err = insertAgent(w, &agent)
	}
	if err != nil {
		return Actor{}, err
	}
	if !agent.Active() {
		return Actor{}, forbidden("this Discord agent is disabled")
	}
	return agent, nil
}

func linkedAgent(w *writeTx, guildID, userID string) (string, error) {
	var agentID string
	err := w.tx.QueryRowContext(w.ctx, `SELECT agent_id FROM discord_links WHERE guild_id = ? AND user_id = ?`,
		guildID, userID).Scan(&agentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read existing Discord link: %w", err)
	}
	return agentID, nil
}

func saveDiscordLink(w *writeTx, guildID string, member DiscordMember, agent Actor) error {
	_, err := w.exec(`INSERT INTO discord_links (guild_id, user_id, agent_id, user_name) VALUES (?, ?, ?, ?)
		ON CONFLICT(guild_id, user_id) DO UPDATE SET agent_id = excluded.agent_id, user_name = excluded.user_name`,
		guildID, member.UserID, agent.ID, member.Name)
	if err != nil {
		return mapUniqueViolation(err, "this teammate is already linked to another Discord account", "save Discord link")
	}
	return nil
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
