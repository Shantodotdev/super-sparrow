package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CLILoginLifetime is how long a CLI waits for its sign-in to be approved in the desktop app.
const CLILoginLifetime = 10 * time.Minute

// cliLoginRetention is how long finished and expired CLI sign-in requests are kept before removal.
const cliLoginRetention = 24 * time.Hour

// CLILoginStatus is where a CLI sign-in request stands.
type CLILoginStatus string

const (
	CLILoginPending  CLILoginStatus = "pending"
	CLILoginApproved CLILoginStatus = "approved"
	CLILoginDenied   CLILoginStatus = "denied"
	// CLILoginClaimed means the CLI has received its session; the request cannot be used again.
	CLILoginClaimed CLILoginStatus = "claimed"
	// CLILoginExpired is never stored: a request past its expiry, or one that does not exist.
	CLILoginExpired CLILoginStatus = "expired"
)

const cliLoginColumns = `id, device_label, requested_from, status, expires_at, created_at`

// NewCLILogin is a CLI's request to sign in. The caller makes both codes and passes only their
// hashes: the device code stays with the CLI, and the user code is what the person approves.
type NewCLILogin struct {
	DeviceCodeHash []byte
	UserCodeHash   []byte
	DeviceLabel    string
	// RequestedFrom is the address the CLI asked from, shown to the person who approves it.
	RequestedFrom string
}

// CLILogin is a CLI sign-in request as the approving person sees it.
type CLILogin struct {
	ID            string
	DeviceLabel   string
	RequestedFrom string
	Status        CLILoginStatus
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// CreateCLILogin stores a new CLI sign-in request that waits CLILoginLifetime for a person to
// approve it. Requests are credentials, not team data, so they write no events; the session an
// approved request creates does.
func (s *Store) CreateCLILogin(ctx context.Context, input NewCLILogin) (CLILogin, error) {
	label, err := singleLineText("device_label", input.DeviceLabel, maxDeviceLabel)
	if err != nil {
		return CLILogin{}, err
	}
	if len(input.DeviceCodeHash) != hashLength || len(input.UserCodeHash) != hashLength {
		return CLILogin{}, fmt.Errorf("CLI sign-in code hashes must be %d bytes", hashLength)
	}
	from, err := singleLineText("requested_from", input.RequestedFrom, maxDeviceLabel)
	if err != nil {
		return CLILogin{}, err
	}

	var login CLILogin
	err = s.transact(ctx, func(tx *sql.Tx, now time.Time) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cli_login_requests WHERE expires_at < ?`,
			now.Add(-cliLoginRetention).Format(timeLayout)); err != nil {
			return fmt.Errorf("remove old CLI sign-in requests: %w", err)
		}

		login = CLILogin{
			ID: newID(), DeviceLabel: label, RequestedFrom: from, Status: CLILoginPending,
			ExpiresAt: now.Add(CLILoginLifetime), CreatedAt: now,
		}
		stamp := now.Format(timeLayout)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO cli_login_requests (id, device_code_hash, user_code_hash, device_label, requested_from, status,
				expires_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'pending', ?, ?, ?)`,
			login.ID, input.DeviceCodeHash, input.UserCodeHash, label, from,
			login.ExpiresAt.Format(timeLayout), stamp, stamp)
		if err != nil {
			return fmt.Errorf("store CLI sign-in request: %w", err)
		}
		return nil
	})
	return login, err
}

// GetCLILogin finds the pending request with userCodeHash, for the person about to approve it.
// A request that is missing, expired, or already decided is not found.
func (s *Store) GetCLILogin(ctx context.Context, userCodeHash []byte) (CLILogin, error) {
	now := s.now().UTC().Format(timeLayout)
	return scanCLILogin(s.db.QueryRowContext(ctx, `
		SELECT `+cliLoginColumns+` FROM cli_login_requests
		WHERE user_code_hash = ? AND status = 'pending' AND expires_at > ?`, userCodeHash, now))
}

// DecideCLILogin approves or denies the pending request with userCodeHash. Only an active person
// may decide, and an approved request signs the CLI in as that person.
func (s *Store) DecideCLILogin(ctx context.Context, by Caller, userCodeHash []byte, approve bool) (CLILogin, error) {
	var login CLILogin
	err := s.write(ctx, by, func(w *writeTx) error {
		if w.caller.Kind != ActorHuman {
			return forbidden("only a person can approve a CLI sign-in")
		}
		// A signed-in CLI must not sign in more CLIs, so approval needs the desktop app.
		if w.client != ClientBrowser {
			return forbidden("approve CLI sign-ins in the desktop app")
		}

		var err error
		login, err = scanCLILogin(w.tx.QueryRowContext(ctx, `
			SELECT `+cliLoginColumns+` FROM cli_login_requests
			WHERE user_code_hash = ? AND status = 'pending' AND expires_at > ?`, userCodeHash, w.now.Format(timeLayout)))
		if err != nil {
			return err
		}

		login.Status = CLILoginDenied
		approver := sql.NullString{}
		if approve {
			login.Status = CLILoginApproved
			approver = nullString(w.caller.ID)
		}
		_, err = w.exec(`UPDATE cli_login_requests SET status = ?, approved_by_actor_id = ?, updated_at = ? WHERE id = ?`,
			login.Status, approver, w.now.Format(timeLayout), login.ID)
		if err != nil {
			return fmt.Errorf("decide CLI sign-in request: %w", err)
		}
		return nil
	})
	return login, err
}

// ClaimCLILogin is the CLI checking on its request with deviceCodeHash. Once the request is
// approved, the first claim creates a CLI session with sessionHash for the approving person and
// uses up the request. Otherwise it returns the status with no session: pending, denied, or
// expired, which also covers unknown and already claimed requests.
func (s *Store) ClaimCLILogin(ctx context.Context, deviceCodeHash, sessionHash []byte) (CLILoginStatus, Actor, Session, error) {
	if len(sessionHash) != hashLength {
		return "", Actor{}, Session{}, fmt.Errorf("session hash must be %d bytes", hashLength)
	}

	status := CLILoginExpired
	var actor Actor
	var session Session
	err := s.transact(ctx, func(tx *sql.Tx, now time.Time) error {
		var id, label, stored, expires string
		var approver sql.NullString
		err := tx.QueryRowContext(ctx, `
			SELECT id, device_label, status, approved_by_actor_id, expires_at FROM cli_login_requests WHERE device_code_hash = ?`,
			deviceCodeHash).Scan(&id, &label, &stored, &approver, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read CLI sign-in request: %w", err)
		}

		expiresAt, err := parseTime(expires)
		if err != nil {
			return err
		}
		if CLILoginStatus(stored) == CLILoginClaimed || !now.Before(expiresAt) {
			return nil
		}
		status = CLILoginStatus(stored)
		if status != CLILoginApproved {
			return nil
		}

		// The person may have been disabled after approving.
		actor, err = scanActor(tx.QueryRowContext(ctx, `SELECT `+actorColumns+` FROM actors WHERE id = ?`, approver.String))
		if err != nil {
			return err
		}
		if !actor.Active() {
			status = CLILoginDenied
			return nil
		}

		if _, err := tx.ExecContext(ctx, `UPDATE cli_login_requests SET status = 'claimed', updated_at = ? WHERE id = ?`,
			now.Format(timeLayout), id); err != nil {
			return fmt.Errorf("use CLI sign-in request: %w", err)
		}
		session, err = createSession(ctx, tx, now, actor, ClientCLI, sessionHash, label, map[string]any{"approved_in_app": true})
		return err
	})
	if err != nil {
		return "", Actor{}, Session{}, err
	}
	return status, actor, session, nil
}

func scanCLILogin(row scanner) (CLILogin, error) {
	var login CLILogin
	var expires, created string

	err := row.Scan(&login.ID, &login.DeviceLabel, &login.RequestedFrom, &login.Status, &expires, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return CLILogin{}, notFound("CLI sign-in request")
	}
	if err != nil {
		return CLILogin{}, fmt.Errorf("read CLI sign-in request: %w", err)
	}

	if login.ExpiresAt, err = parseTime(expires); err != nil {
		return CLILogin{}, err
	}
	if login.CreatedAt, err = parseTime(created); err != nil {
		return CLILogin{}, err
	}
	return login, nil
}
