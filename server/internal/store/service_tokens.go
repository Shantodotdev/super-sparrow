package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// ServiceTokenLifetime is how long a CI token works after it is created or renewed.
	ServiceTokenLifetime = 365 * 24 * time.Hour
	// ServiceTokenWarning is how long before expiry a token shows as expiring soon.
	ServiceTokenWarning = 14 * 24 * time.Hour
)

const serviceTokenColumns = `id, project_id, config, label, display_hint, created_by_actor_id, last_used_at, expires_at,
	revoked_at, created_at, updated_at`

// ServiceToken lets CI read one project's staging or production config. Only a hash of its secret
// is stored.
type ServiceToken struct {
	ID          string
	ProjectID   string
	Config      SecretConfig
	Label       string
	DisplayHint string
	CreatedBy   string
	LastUsedAt  *time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewServiceToken describes a CI token to create. The caller makes the secret and passes only
// its hash and a hint for telling tokens apart.
type NewServiceToken struct {
	Config      SecretConfig
	Label       string
	Hash        []byte
	DisplayHint string
}

// CreateServiceToken adds a CI token for a project's staging or production config.
func (s *Store) CreateServiceToken(ctx context.Context, by Caller, projectID string, input NewServiceToken) (ServiceToken, error) {
	if !input.Config.sensitive() {
		return ServiceToken{}, invalid("config", "must be staging or production")
	}
	label, err := singleLineText("label", input.Label, maxTokenLabel)
	if err != nil {
		return ServiceToken{}, err
	}
	if len(input.Hash) != hashLength {
		return ServiceToken{}, fmt.Errorf("token hash must be %d bytes", hashLength)
	}

	var token ServiceToken
	err = s.write(ctx, by, func(w *writeTx) error {
		viewer, project, err := w.secretAccess(by, projectID)
		if err != nil {
			return err
		}
		if !viewer.canManage() {
			return forbidden("CI tokens need the staging and production permission, in the browser")
		}

		token = ServiceToken{
			ID: newID(), ProjectID: project.ID, Config: input.Config, Label: label, DisplayHint: input.DisplayHint,
			CreatedBy: w.caller.ID, ExpiresAt: w.now.Add(ServiceTokenLifetime), CreatedAt: w.now, UpdatedAt: w.now,
		}
		_, err = w.exec(`
			INSERT INTO service_tokens (id, project_id, config, label, token_hash, display_hint, created_by_actor_id,
				expires_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			token.ID, project.ID, string(input.Config), label, input.Hash, input.DisplayHint, w.caller.ID,
			token.ExpiresAt.Format(timeLayout), w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("create service token: %w", err)
		}

		return w.serviceTokenEvent(token, "created")
	})
	return token, err
}

// ListServiceTokens returns a project's CI tokens, newest first, including revoked and expired ones.
func (s *Store) ListServiceTokens(ctx context.Context, by Caller, projectID string) ([]ServiceToken, error) {
	viewer, project, err := s.secretAccess(ctx, by, projectID)
	if err != nil {
		return nil, err
	}
	if !viewer.canManage() {
		return nil, forbidden("CI tokens need the staging and production permission, in the browser")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+serviceTokenColumns+` FROM service_tokens WHERE project_id = ? ORDER BY created_at DESC, id DESC`, project.ID)
	if err != nil {
		return nil, fmt.Errorf("list service tokens: %w", err)
	}
	defer rows.Close()

	tokens := []ServiceToken{}
	for rows.Next() {
		token, err := scanServiceToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

// RenewServiceToken extends an unrevoked CI token to a full lifetime from now.
func (s *Store) RenewServiceToken(ctx context.Context, by Caller, tokenID string) (ServiceToken, error) {
	return s.changeServiceToken(ctx, by, tokenID, "renewed", func(w *writeTx, token *ServiceToken) error {
		if token.RevokedAt != nil {
			return conflict("a revoked token cannot be renewed; create a new one")
		}
		token.ExpiresAt, token.UpdatedAt = w.now.Add(ServiceTokenLifetime), w.now
		_, err := w.exec(`UPDATE service_tokens SET expires_at = ?, updated_at = ? WHERE id = ?`,
			token.ExpiresAt.Format(timeLayout), w.timestamp(), token.ID)
		return err
	})
}

// RevokeServiceToken stops a CI token from working. Revoking it again changes nothing.
func (s *Store) RevokeServiceToken(ctx context.Context, by Caller, tokenID string) error {
	_, err := s.changeServiceToken(ctx, by, tokenID, "revoked", func(w *writeTx, token *ServiceToken) error {
		if token.RevokedAt != nil {
			return errAlreadyRevoked
		}
		token.RevokedAt, token.UpdatedAt = &w.now, w.now
		_, err := w.exec(`UPDATE service_tokens SET revoked_at = ?1, updated_at = ?1 WHERE id = ?2`, w.timestamp(), token.ID)
		return err
	})
	if errors.Is(err, errAlreadyRevoked) {
		return nil
	}
	return err
}

// errAlreadyRevoked rolls back a revoke that would change nothing, so it writes no event.
var errAlreadyRevoked = errors.New("service token already revoked")

// changeServiceToken loads a token the caller may manage, applies change, and records action.
func (s *Store) changeServiceToken(ctx context.Context, by Caller, tokenID, action string,
	change func(w *writeTx, token *ServiceToken) error) (ServiceToken, error) {
	var token ServiceToken
	err := s.write(ctx, by, func(w *writeTx) error {
		var err error
		token, err = scanServiceToken(w.tx.QueryRowContext(w.ctx, `SELECT `+serviceTokenColumns+` FROM service_tokens WHERE id = ?`, tokenID))
		if err != nil {
			return err
		}
		viewer, _, err := w.secretAccess(by, token.ProjectID)
		if err != nil {
			return err
		}
		if !viewer.canManage() {
			return forbidden("CI tokens need the staging and production permission, in the browser")
		}

		if err := change(w, &token); err != nil {
			return err
		}
		return w.serviceTokenEvent(token, action)
	})
	return token, err
}

// revokeServiceTokensBy revokes every working CI token creatorID made, recording each revoke.
func revokeServiceTokensBy(w *writeTx, creatorID string) error {
	rows, err := w.tx.QueryContext(w.ctx, `
		SELECT `+serviceTokenColumns+` FROM service_tokens WHERE created_by_actor_id = ? AND revoked_at IS NULL`, creatorID)
	if err != nil {
		return fmt.Errorf("list service tokens to revoke: %w", err)
	}
	var tokens []ServiceToken
	for rows.Next() {
		token, err := scanServiceToken(rows)
		if err != nil {
			rows.Close()
			return err
		}
		tokens = append(tokens, token)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list service tokens to revoke: %w", err)
	}

	for _, token := range tokens {
		token.RevokedAt, token.UpdatedAt = &w.now, w.now
		if _, err := w.exec(`UPDATE service_tokens SET revoked_at = ?1, updated_at = ?1 WHERE id = ?2`, w.timestamp(), token.ID); err != nil {
			return fmt.Errorf("revoke service token: %w", err)
		}
		if err := w.serviceTokenEvent(token, "revoked"); err != nil {
			return err
		}
	}
	return nil
}

// serviceTokenEvent records action on a CI token in its project's history.
func (w *writeTx) serviceTokenEvent(token ServiceToken, action string) error {
	return w.event(token.ProjectID, TargetProject, token.ProjectID, "service_token."+action,
		map[string]any{"token_id": token.ID, "config": token.Config, "label": token.Label, "expires_at": token.ExpiresAt.Format(timeLayout)})
}

// DownloadWithServiceToken returns the values of the config a CI token reads and records the
// download. Every wrong, revoked, or expired token fails with ErrUnauthenticated.
func (s *Store) DownloadWithServiceToken(ctx context.Context, hash []byte) (ServiceToken, SecretValues, error) {
	var token ServiceToken
	var values SecretValues
	err := s.transact(ctx, func(tx *sql.Tx, now time.Time) error {
		var err error
		token, err = scanServiceToken(tx.QueryRowContext(ctx, `SELECT `+serviceTokenColumns+` FROM service_tokens WHERE token_hash = ?`, hash))
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if token.RevokedAt != nil || !token.ExpiresAt.After(now) {
			return ErrUnauthenticated
		}
		if _, err := secretsProject(ctx, tx, token.ProjectID); err != nil {
			return err
		}

		values, err = s.readValues(ctx, tx, token.ProjectID, token.Config, "")
		if err != nil {
			return err
		}

		stamp := now.Format(timeLayout)
		if _, err := tx.ExecContext(ctx, `UPDATE service_tokens SET last_used_at = ? WHERE id = ?`, stamp, token.ID); err != nil {
			return fmt.Errorf("record service token use: %w", err)
		}
		token.LastUsedAt = &now
		return insertAccessLog(ctx, tx, token.ProjectID, token.Config, "", "", token.ID, AccessDownload, values.Names, stamp)
	})
	return token, values, err
}

// scanServiceToken reads one service_tokens row selected with serviceTokenColumns.
func scanServiceToken(row scanner) (ServiceToken, error) {
	var token ServiceToken
	var lastUsed, revoked sql.NullString
	var expires, created, updated string

	err := row.Scan(&token.ID, &token.ProjectID, &token.Config, &token.Label, &token.DisplayHint, &token.CreatedBy,
		&lastUsed, &expires, &revoked, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceToken{}, notFound("service token")
	}
	if err != nil {
		return ServiceToken{}, fmt.Errorf("read service token: %w", err)
	}

	if token.LastUsedAt, err = parseOptionalTime(lastUsed); err != nil {
		return ServiceToken{}, err
	}
	if token.ExpiresAt, err = parseTime(expires); err != nil {
		return ServiceToken{}, err
	}
	if token.RevokedAt, err = parseOptionalTime(revoked); err != nil {
		return ServiceToken{}, err
	}
	if token.CreatedAt, token.UpdatedAt, err = timestamps(created, updated); err != nil {
		return ServiceToken{}, err
	}
	return token, nil
}
