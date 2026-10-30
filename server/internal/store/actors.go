package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

const actorColumns = `id, kind, display_name, avatar_id, email, role, agent_name, owner_actor_id, disabled_at, created_at, updated_at`

// NewHuman describes a person to add to the team.
type NewHuman struct {
	DisplayName string
	Email       string
	Role        Role
}

// NewAgent describes an agent that works for the calling person.
type NewAgent struct {
	DisplayName string
	// AgentName identifies the tool, such as claude-code or codex.
	AgentName string
}

// CreateHuman adds a person to the team. Only an owner or the system can add people.
func (s *Store) CreateHuman(ctx context.Context, by Caller, input NewHuman) (Actor, error) {
	name, err := singleLineText("display_name", input.DisplayName, maxNameLength)
	if err != nil {
		return Actor{}, err
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return Actor{}, err
	}
	if !input.Role.valid() {
		return Actor{}, invalid("role", "must be owner or member")
	}

	actor := Actor{ID: newID(), Kind: ActorHuman, DisplayName: name, Email: email, Role: input.Role}
	err = s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireOwner("add people"); err != nil {
			return err
		}
		return insertHuman(w, &actor)
	})

	return actor, err
}

// CreateFirstOwner adds the team's first owner, recorded as the system actor. It fails with
// ErrConflict once anyone has been added, so it cannot be used to take over a running team.
func (s *Store) CreateFirstOwner(ctx context.Context, displayName, email string) (Actor, error) {
	name, err := singleLineText("display_name", displayName, maxNameLength)
	if err != nil {
		return Actor{}, err
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return Actor{}, err
	}

	actor := Actor{ID: newID(), Kind: ActorHuman, DisplayName: name, Email: normalized, Role: RoleOwner}
	err = s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		var people int
		if err := w.tx.QueryRowContext(w.ctx, `SELECT count(*) FROM actors WHERE kind = 'human'`).Scan(&people); err != nil {
			return fmt.Errorf("count people: %w", err)
		}
		if people > 0 {
			return conflict("the team already has people; an owner adds the rest")
		}
		return insertHuman(w, &actor)
	})

	return actor, err
}

func insertHuman(w *writeTx, actor *Actor) error {
	actor.CreatedAt, actor.UpdatedAt = w.now, w.now
	_, err := w.exec(`
		INSERT INTO actors (id, kind, display_name, email, role, created_at, updated_at)
		VALUES (?, 'human', ?, ?, ?, ?, ?)`,
		actor.ID, actor.DisplayName, actor.Email, actor.Role, w.timestamp(), w.timestamp())
	if err != nil {
		return mapUniqueViolation(err, "a person with this email already exists", "create person")
	}

	return w.event("", TargetActor, actor.ID, "actor.created",
		map[string]any{"kind": ActorHuman, "display_name": actor.DisplayName, "email": actor.Email, "role": actor.Role})
}

// CreateAgent adds an agent that works for the calling person. Only a person can add agents,
// and only for themselves.
func (s *Store) CreateAgent(ctx context.Context, by Caller, input NewAgent) (Actor, error) {
	name, err := singleLineText("display_name", input.DisplayName, maxNameLength)
	if err != nil {
		return Actor{}, err
	}
	if !agentNamePattern.MatchString(input.AgentName) {
		return Actor{}, invalid("agent_name", "must be lowercase letters, digits, and dashes, such as claude-code")
	}

	actor := Actor{ID: newID(), Kind: ActorAgent, DisplayName: name, AgentName: input.AgentName}
	err = s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("add agents"); err != nil {
			return err
		}

		actor.OwnerActorID = w.caller.ID
		return insertAgent(w, &actor)
	})

	return actor, err
}

func insertAgent(w *writeTx, actor *Actor) error {
	actor.CreatedAt, actor.UpdatedAt = w.now, w.now
	_, err := w.exec(`
		INSERT INTO actors (id, kind, display_name, agent_name, owner_actor_id, created_at, updated_at)
		VALUES (?, 'agent', ?, ?, ?, ?, ?)`,
		actor.ID, actor.DisplayName, actor.AgentName, actor.OwnerActorID, w.timestamp(), w.timestamp())
	if err != nil {
		return mapUniqueViolation(err, "this person already has a "+actor.AgentName+" agent", "create agent")
	}

	return w.event("", TargetActor, actor.ID, "actor.created",
		map[string]any{"kind": ActorAgent, "display_name": actor.DisplayName, "agent_name": actor.AgentName, "owner_actor_id": actor.OwnerActorID})
}

// SetRole changes a person's role. Only an owner can change roles, and the last active owner
// cannot be made a member.
func (s *Store) SetRole(ctx context.Context, by Caller, actorID string, role Role) error {
	if !role.valid() {
		return invalid("role", "must be owner or member")
	}

	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireOwner("change roles"); err != nil {
			return err
		}

		target, err := getActor(w.ctx, w.tx, actorID)
		if err != nil {
			return err
		}
		if target.Kind != ActorHuman {
			return invalid("actor_id", "only a person has a role")
		}
		if target.Role == role {
			return nil
		}
		if target.IsOwner() && role != RoleOwner {
			if err := ensureAnotherOwner(w, target.ID); err != nil {
				return err
			}
		}

		if _, err := w.exec(`UPDATE actors SET role = ?, updated_at = ? WHERE id = ?`, role, w.timestamp(), target.ID); err != nil {
			return fmt.Errorf("change role: %w", err)
		}

		return w.event("", TargetActor, target.ID, "actor.role_changed",
			map[string]any{"from": target.Role, "to": role})
	})
}

// DisableActor stops a person or agent from making changes. Disabling a person also disables
// their agents and revokes every session and agent token of theirs, in the same transaction.
// Only an owner can disable actors, the system actor cannot be disabled, and the last active
// owner cannot be disabled.
func (s *Store) DisableActor(ctx context.Context, by Caller, actorID string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireOwner("disable people and agents"); err != nil {
			return err
		}

		target, err := getActor(w.ctx, w.tx, actorID)
		if err != nil {
			return err
		}
		if target.Kind == ActorSystem {
			return invalid("actor_id", "the system actor cannot be disabled")
		}
		if !target.Active() {
			return nil
		}
		if target.IsOwner() {
			if err := ensureAnotherOwner(w, target.ID); err != nil {
				return err
			}
		}

		// The person and all of their agents.
		affected := `SELECT id FROM actors WHERE id = ?1 OR owner_actor_id = ?1`
		statements := []string{
			`UPDATE actors SET disabled_at = ?2, updated_at = ?2 WHERE disabled_at IS NULL AND id IN (` + affected + `)`,
			`UPDATE sessions SET revoked_at = ?2, updated_at = ?2 WHERE revoked_at IS NULL AND actor_id IN (` + affected + `)`,
			`UPDATE agent_tokens SET revoked_at = ?2, updated_at = ?2 WHERE revoked_at IS NULL AND actor_id IN (` + affected + `)`,
		}
		for _, statement := range statements {
			if _, err := w.exec(statement, target.ID, w.timestamp()); err != nil {
				return fmt.Errorf("disable actor: %w", err)
			}
		}

		return w.event("", TargetActor, target.ID, "actor.disabled", map[string]any{"kind": target.Kind})
	})
}

// ensureAnotherOwner fails when actorID is the only active owner, so the team always keeps one.
func ensureAnotherOwner(w *writeTx, actorID string) error {
	var others int
	err := w.tx.QueryRowContext(w.ctx, `
		SELECT count(*) FROM actors
		WHERE kind = 'human' AND role = 'owner' AND disabled_at IS NULL AND id != ?`, actorID).Scan(&others)
	if err != nil {
		return fmt.Errorf("count owners: %w", err)
	}
	if others == 0 {
		return conflict("the team must keep at least one active owner")
	}
	return nil
}

// GetActor returns the actor with the given ID.
func (s *Store) GetActor(ctx context.Context, id string) (Actor, error) {
	return getActor(ctx, s.db, id)
}

// GetActorByEmail returns the person with the given email, matched without regard to case.
func (s *Store) GetActorByEmail(ctx context.Context, email string) (Actor, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return Actor{}, err
	}

	return scanActor(s.db.QueryRowContext(ctx, `SELECT `+actorColumns+` FROM actors WHERE email = ?`, normalized))
}

// ListActors returns every actor, people first, then agents, each in name order.
func (s *Store) ListActors(ctx context.Context) ([]Actor, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+actorColumns+` FROM actors
		ORDER BY CASE kind WHEN 'system' THEN 0 WHEN 'human' THEN 1 ELSE 2 END, display_name, id`)
	if err != nil {
		return nil, fmt.Errorf("list actors: %w", err)
	}
	defer rows.Close()

	var actors []Actor
	for rows.Next() {
		actor, err := scanActor(rows)
		if err != nil {
			return nil, err
		}
		actors = append(actors, actor)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list actors: %w", err)
	}
	return actors, nil
}

func getActor(ctx context.Context, q querier, id string) (Actor, error) {
	return scanActor(q.QueryRowContext(ctx, `SELECT `+actorColumns+` FROM actors WHERE id = ?`, id))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanActor(row scanner) (Actor, error) {
	var actor Actor
	var email, role, agentName, owner, disabled sql.NullString
	var created, updated string

	err := row.Scan(&actor.ID, &actor.Kind, &actor.DisplayName, &actor.AvatarID, &email, &role, &agentName, &owner, &disabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Actor{}, notFound("actor")
	}
	if err != nil {
		return Actor{}, fmt.Errorf("read actor: %w", err)
	}

	actor.Email, actor.Role, actor.AgentName, actor.OwnerActorID = email.String, Role(role.String), agentName.String, owner.String
	if actor.DisabledAt, err = parseOptionalTime(disabled); err != nil {
		return Actor{}, err
	}
	if actor.CreatedAt, actor.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Actor{}, err
	}

	return actor, nil
}

// ProfileUpdate contains the caller's editable human profile fields.
type ProfileUpdate struct {
	DisplayName Field[string]
	AvatarID    Field[string]
}

// UpdateProfile updates only the authenticated person's own profile and records their actor.
func (s *Store) UpdateProfile(ctx context.Context, by Caller, update ProfileUpdate) (Actor, error) {
	if update.DisplayName.Set {
		name, err := singleLineText("display_name", update.DisplayName.Value, maxNameLength)
		if err != nil {
			return Actor{}, err
		}
		update.DisplayName.Value = name
	}
	if update.AvatarID.Set && update.AvatarID.Value != "" && !slices.Contains(profileAvatarIDs, update.AvatarID.Value) {
		return Actor{}, invalid("avatar_id", "must be one of the available avatars or empty for initials")
	}
	var actor Actor
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("edit your profile"); err != nil {
			return err
		}
		actor = w.caller
		changes := map[string]any{}
		applyChange(update.DisplayName, &actor.DisplayName, "display_name", changes)
		applyChange(update.AvatarID, &actor.AvatarID, "avatar_id", changes)
		if len(changes) == 0 {
			return nil
		}
		if _, err := w.exec(`UPDATE actors SET display_name = ?, avatar_id = ?, updated_at = ? WHERE id = ?`, actor.DisplayName, actor.AvatarID, w.timestamp(), actor.ID); err != nil {
			return fmt.Errorf("update profile %s: %w", actor.ID, err)
		}
		actor.UpdatedAt = w.now
		return w.event("", TargetActor, actor.ID, "actor.updated", changes)
	})
	return actor, err
}
