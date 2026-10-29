package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

const (
	// TokenLifetime is how long an agent token works after it is created or renewed.
	TokenLifetime = 90 * 24 * time.Hour

	// tokenTouchInterval limits how often a token's last use is recorded, so not every request writes.
	tokenTouchInterval = time.Minute
	maxTokenLabel      = 100
)

const tokenColumns = `t.id, t.actor_id, a.owner_actor_id, a.agent_name, t.label, t.display_hint,
	t.last_used_at, t.expires_at, t.revoked_at, t.created_at, t.updated_at`

const tokenFrom = ` FROM agent_tokens t JOIN actors a ON a.id = t.actor_id`

// toolNames are the display names of the agents the team uses. Other agents show their agent name.
var toolNames = map[string]string{"claude-code": "Claude Code", "codex": "Codex"}

// NewAgentToken describes a token a person creates for one of their agents. The caller makes the
// secret and passes only its hash and a hint for telling tokens apart.
type NewAgentToken struct {
	// AgentName identifies the tool, such as claude-code or codex.
	AgentName   string
	Label       string
	Hash        []byte
	DisplayHint string
}

// AuthenticatedToken is an agent token that passed its checks, with the agent it acts as.
type AuthenticatedToken struct {
	Actor Actor
	Token AgentToken
}

// CreateAgentToken creates a token for the caller's agent for input.AgentName, adding that agent
// the first time, named after the tool and its person, such as "Claude Code (Shanto)". Only a
// person can create tokens, and only for their own agents.
func (s *Store) CreateAgentToken(ctx context.Context, by Caller, input NewAgentToken) (AgentToken, error) {
	if !agentNamePattern.MatchString(input.AgentName) {
		return AgentToken{}, invalid("agent_name", "must be lowercase letters, digits, and dashes, such as claude-code")
	}
	label, err := singleLineText("label", input.Label, maxTokenLabel)
	if err != nil {
		return AgentToken{}, err
	}
	if len(input.Hash) != hashLength {
		return AgentToken{}, fmt.Errorf("token hash must be %d bytes", hashLength)
	}

	var token AgentToken
	err = s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("create agent tokens"); err != nil {
			return err
		}

		agent, err := agentFor(w, input.AgentName)
		if err != nil {
			return err
		}

		token = AgentToken{
			ID: newID(), AgentActorID: agent.ID, OwnerActorID: w.caller.ID, AgentName: agent.AgentName,
			Label: label, DisplayHint: input.DisplayHint, ExpiresAt: w.now.Add(TokenLifetime),
			CreatedAt: w.now, UpdatedAt: w.now,
		}
		_, err = w.exec(`
			INSERT INTO agent_tokens (id, actor_id, label, token_hash, display_hint, expires_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			token.ID, agent.ID, label, input.Hash, input.DisplayHint, token.ExpiresAt.Format(timeLayout), w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("create agent token: %w", err)
		}

		return w.event("", TargetActor, agent.ID, "token.created",
			map[string]any{"token_id": token.ID, "label": label, "expires_at": token.ExpiresAt.Format(timeLayout)})
	})

	return token, err
}

// agentFor returns the caller's agent for agentName, creating it when the caller has none yet.
func agentFor(w *writeTx, agentName string) (Actor, error) {
	agent, err := scanActor(w.tx.QueryRowContext(w.ctx, `
		SELECT `+actorColumns+` FROM actors WHERE kind = 'agent' AND owner_actor_id = ? AND agent_name = ?`,
		w.caller.ID, agentName))
	if err == nil {
		if !agent.Active() {
			return Actor{}, forbidden("this agent is disabled")
		}
		return agent, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Actor{}, err
	}

	agent = Actor{ID: newID(), Kind: ActorAgent, AgentName: agentName, OwnerActorID: w.caller.ID,
		DisplayName: agentDisplayName(agentName, w.caller.DisplayName)}
	if err := insertAgent(w, &agent); err != nil {
		return Actor{}, err
	}
	return agent, nil
}

// agentDisplayName names an agent after its tool and its person, such as "Claude Code (Shanto)",
// shortening the person's name to keep within the display name limit.
func agentDisplayName(agentName, ownerName string) string {
	tool, known := toolNames[agentName]
	if !known {
		tool = agentName
	}

	room := maxNameLength - utf8.RuneCountInString(tool) - len(" ()")
	if owner := []rune(ownerName); len(owner) > room {
		ownerName = string(owner[:room])
	}
	return tool + " (" + ownerName + ")"
}

// RenewAgentToken extends one of the caller's own tokens for another TokenLifetime from now,
// including a token that has already expired. A revoked token cannot be renewed.
func (s *Store) RenewAgentToken(ctx context.Context, by Caller, tokenID string) (AgentToken, error) {
	var token AgentToken
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("renew agent tokens"); err != nil {
			return err
		}

		var err error
		token, err = scanToken(w.tx.QueryRowContext(w.ctx, `SELECT `+tokenColumns+tokenFrom+` WHERE t.id = ?`, tokenID))
		if err != nil {
			return err
		}
		if token.OwnerActorID != w.caller.ID {
			return notFound("agent token")
		}
		if token.RevokedAt != nil {
			return fmt.Errorf("a revoked token cannot be renewed: %w", ErrInvalidTransition)
		}

		token.ExpiresAt, token.UpdatedAt = w.now.Add(TokenLifetime), w.now
		_, err = w.exec(`UPDATE agent_tokens SET expires_at = ?, updated_at = ? WHERE id = ?`,
			token.ExpiresAt.Format(timeLayout), w.timestamp(), token.ID)
		if err != nil {
			return fmt.Errorf("renew agent token: %w", err)
		}

		return w.event("", TargetActor, token.AgentActorID, "token.renewed",
			map[string]any{"token_id": token.ID, "expires_at": token.ExpiresAt.Format(timeLayout)})
	})

	return token, err
}

// RevokeAgentToken stops a token from working. A person can revoke their own tokens, and an
// owner can revoke anyone's.
func (s *Store) RevokeAgentToken(ctx context.Context, by Caller, tokenID string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("revoke agent tokens"); err != nil {
			return err
		}

		token, err := scanToken(w.tx.QueryRowContext(w.ctx, `SELECT `+tokenColumns+tokenFrom+` WHERE t.id = ?`, tokenID))
		if err != nil {
			return err
		}
		if token.OwnerActorID != w.caller.ID && !w.caller.IsOwner() {
			return notFound("agent token")
		}
		if token.RevokedAt != nil {
			return nil
		}

		if _, err := w.exec(`UPDATE agent_tokens SET revoked_at = ?1, updated_at = ?1 WHERE id = ?2`, w.timestamp(), token.ID); err != nil {
			return fmt.Errorf("revoke agent token: %w", err)
		}

		return w.event("", TargetActor, token.AgentActorID, "token.revoked", map[string]any{"token_id": token.ID})
	})
}

// ListAgentTokens returns the unrevoked tokens of one person's agents, or of everyone's when
// ownerActorID is empty, newest first. Expired tokens are included so they can be renewed.
func (s *Store) ListAgentTokens(ctx context.Context, ownerActorID string) ([]AgentToken, error) {
	query := `SELECT ` + tokenColumns + tokenFrom + ` WHERE t.revoked_at IS NULL`
	var args []any
	if ownerActorID != "" {
		query += ` AND a.owner_actor_id = ?`
		args = append(args, ownerActorID)
	}

	rows, err := s.db.QueryContext(ctx, query+` ORDER BY t.created_at DESC, t.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent tokens: %w", err)
	}
	defer rows.Close()

	var tokens []AgentToken
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list agent tokens: %w", err)
	}
	return tokens, nil
}

// AuthenticateToken finds the token whose secret hashes to hash and checks that it is current
// and that its agent and person are active. It records the token's last use at most once a
// minute; that is bookkeeping and writes no event.
func (s *Store) AuthenticateToken(ctx context.Context, hash []byte) (AuthenticatedToken, error) {
	token, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenColumns+tokenFrom+` WHERE t.token_hash = ?`, hash))
	if errors.Is(err, ErrNotFound) {
		return AuthenticatedToken{}, ErrUnauthenticated
	}
	if err != nil {
		return AuthenticatedToken{}, err
	}

	now := s.now().UTC().Truncate(time.Millisecond)
	if token.RevokedAt != nil || !token.ExpiresAt.After(now) {
		return AuthenticatedToken{}, ErrUnauthenticated
	}

	agent, err := getActor(ctx, s.db, token.AgentActorID)
	if err != nil {
		return AuthenticatedToken{}, err
	}
	owner, err := getActor(ctx, s.db, token.OwnerActorID)
	if err != nil {
		return AuthenticatedToken{}, err
	}
	if !agent.Active() || !owner.Active() {
		return AuthenticatedToken{}, ErrUnauthenticated
	}

	if token.LastUsedAt == nil || now.Sub(*token.LastUsedAt) >= tokenTouchInterval {
		_, err := s.db.ExecContext(ctx, `UPDATE agent_tokens SET last_used_at = ? WHERE id = ?`, now.Format(timeLayout), token.ID)
		if err != nil {
			return AuthenticatedToken{}, fmt.Errorf("record token use: %w", err)
		}
		token.LastUsedAt = &now
	}

	return AuthenticatedToken{Actor: agent, Token: token}, nil
}

func scanToken(row scanner) (AgentToken, error) {
	var token AgentToken
	var lastUsed, revoked sql.NullString
	var expires, created, updated string

	err := row.Scan(&token.ID, &token.AgentActorID, &token.OwnerActorID, &token.AgentName, &token.Label, &token.DisplayHint,
		&lastUsed, &expires, &revoked, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentToken{}, notFound("agent token")
	}
	if err != nil {
		return AgentToken{}, fmt.Errorf("read agent token: %w", err)
	}

	if token.LastUsedAt, err = parseOptionalTime(lastUsed); err != nil {
		return AgentToken{}, err
	}
	if token.ExpiresAt, err = parseTime(expires); err != nil {
		return AgentToken{}, err
	}
	if token.RevokedAt, err = parseOptionalTime(revoked); err != nil {
		return AgentToken{}, err
	}
	if token.CreatedAt, token.UpdatedAt, err = timestamps(created, updated); err != nil {
		return AgentToken{}, err
	}

	return token, nil
}
