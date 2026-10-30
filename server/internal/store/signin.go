package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// SignInCodeLifetime is how long an emailed sign-in code works.
	SignInCodeLifetime = 10 * time.Minute
	// MaxSignInAttempts is how many wrong codes cancel a sign-in code.
	MaxSignInAttempts = 5
	// MaxLiveSignInCodes is how many of an email's newest codes work at once, so someone else
	// requesting codes for that email cannot cancel the one its owner is about to enter.
	MaxLiveSignInCodes = 3
	// SessionLifetime is how long a session lasts after it was last used.
	SessionLifetime = 30 * 24 * time.Hour

	// sessionRenewInterval limits how often a session in use is renewed, so not every request writes.
	sessionRenewInterval = time.Hour
	// usedCodeRetention is how long spent and expired sign-in codes are kept before removal.
	usedCodeRetention = 24 * time.Hour
	hashLength        = 32
	maxDeviceLabel    = 100
)

const sessionColumns = `id, actor_id, device_label, last_seen_at, expires_at, revoked_at, created_at, updated_at`

// NewSession describes the session a successful sign-in creates. The caller makes the session
// secret and passes only its hash.
type NewSession struct {
	Hash        []byte
	DeviceLabel string
}

// AuthenticatedSession is a session that passed its checks, with the person it belongs to.
type AuthenticatedSession struct {
	Actor   Actor
	Session Session
	// Renewed is true when this check extended the session, so the cookie should be refreshed.
	Renewed bool
}

// IssueSignInCode stores a new sign-in code for email. The newest MaxLiveSignInCodes codes work;
// issuing one more cancels the oldest. It stores a code only when email belongs to an active person, and reports
// whether it did, so the caller emails codes only to teammates while answering everyone alike.
// Sign-in codes are credentials, not team data, so they write no events.
func (s *Store) IssueSignInCode(ctx context.Context, email string, codeHash []byte) (bool, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return false, err
	}
	if len(codeHash) != hashLength {
		return false, fmt.Errorf("sign-in code hash must be %d bytes", hashLength)
	}

	issued := false
	err = s.transact(ctx, func(tx *sql.Tx, now time.Time) error {
		actor, err := scanActor(tx.QueryRowContext(ctx, `SELECT `+actorColumns+` FROM actors WHERE email = ?`, normalized))
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !actor.Active() {
			return nil
		}

		stamp := now.Format(timeLayout)
		_, err = tx.ExecContext(ctx, `
			UPDATE sign_in_codes SET expires_at = ?1, updated_at = ?1
			WHERE id IN (
				SELECT id FROM sign_in_codes
				WHERE email = ?2 AND used_at IS NULL AND expires_at > ?1 AND failed_attempts < ?3
				ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET ?4
			)`, stamp, normalized, MaxSignInAttempts, MaxLiveSignInCodes-1)
		if err != nil {
			return fmt.Errorf("cancel the oldest sign-in codes: %w", err)
		}

		_, err = tx.ExecContext(ctx, `DELETE FROM sign_in_codes WHERE created_at < ?`,
			now.Add(-usedCodeRetention).Format(timeLayout))
		if err != nil {
			return fmt.Errorf("remove old sign-in codes: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO sign_in_codes (id, email, code_hash, expires_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			newID(), normalized, codeHash, now.Add(SignInCodeLifetime).Format(timeLayout), stamp, stamp)
		if err != nil {
			return fmt.Errorf("store sign-in code: %w", err)
		}

		issued = true
		return nil
	})

	return issued, err
}

// VerifySignInCode checks codeHash against email's working codes. A match uses that code, cancels
// the others, and creates a session for the person in one transaction. A mismatch counts a
// failed attempt, and the fifth cancels the codes. Every failure returns ErrInvalidCode.
func (s *Store) VerifySignInCode(ctx context.Context, email string, codeHash []byte, input NewSession) (Actor, Session, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return Actor{}, Session{}, err
	}
	label, err := singleLineText("device_label", input.DeviceLabel, maxDeviceLabel)
	if err != nil {
		return Actor{}, Session{}, err
	}
	if len(input.Hash) != hashLength {
		return Actor{}, Session{}, fmt.Errorf("session hash must be %d bytes", hashLength)
	}

	var actor Actor
	var session Session
	rejected := false
	err = s.transact(ctx, func(tx *sql.Tx, now time.Time) error {
		stamp := now.Format(timeLayout)

		matched, err := matchSignInCode(ctx, tx, normalized, stamp, codeHash)
		if err != nil {
			return err
		}

		// A wrong guess counts against every working code, so having several codes never allows
		// more than MaxSignInAttempts guesses. The failure is committed so guesses add up.
		if matched == "" {
			_, err := tx.ExecContext(ctx, `
				UPDATE sign_in_codes SET failed_attempts = failed_attempts + 1, updated_at = ?1
				WHERE email = ?2 AND used_at IS NULL AND expires_at > ?1 AND failed_attempts < ?3`,
				stamp, normalized, MaxSignInAttempts)
			if err != nil {
				return fmt.Errorf("count failed sign-in attempt: %w", err)
			}
			rejected = true
			return nil
		}

		// Use the matching code and cancel the others, which are no longer needed.
		_, err = tx.ExecContext(ctx, `
			UPDATE sign_in_codes SET used_at = CASE WHEN id = ?3 THEN ?1 ELSE used_at END,
				expires_at = CASE WHEN id = ?3 THEN expires_at ELSE ?1 END, updated_at = ?1
			WHERE email = ?2 AND used_at IS NULL AND expires_at > ?1`, stamp, normalized, matched)
		if err != nil {
			return fmt.Errorf("use sign-in code: %w", err)
		}

		// The person may have been disabled after the code was sent.
		actor, err = scanActor(tx.QueryRowContext(ctx, `SELECT `+actorColumns+` FROM actors WHERE email = ?`, normalized))
		if errors.Is(err, ErrNotFound) || (err == nil && !actor.Active()) {
			rejected = true
			return nil
		}
		if err != nil {
			return err
		}

		session = Session{
			ID: newID(), ActorID: actor.ID, DeviceLabel: label,
			LastSeenAt: now, ExpiresAt: now.Add(SessionLifetime), CreatedAt: now, UpdatedAt: now,
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO sessions (id, actor_id, session_hash, device_label, last_seen_at, expires_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			session.ID, actor.ID, input.Hash, label, stamp, session.ExpiresAt.Format(timeLayout), stamp, stamp)
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}

		w := &writeTx{tx: tx, ctx: ctx, caller: actor, now: now}
		return w.event("", TargetActor, actor.ID, "session.created",
			map[string]any{"session_id": session.ID, "device_label": label})
	})
	if err != nil {
		return Actor{}, Session{}, err
	}
	if rejected {
		return Actor{}, Session{}, ErrInvalidCode
	}

	return actor, session, nil
}

// matchSignInCode returns the ID of email's working code whose hash is codeHash, or "" when none
// matches. Every working code is compared in constant time.
func matchSignInCode(ctx context.Context, tx *sql.Tx, email, now string, codeHash []byte) (string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, code_hash FROM sign_in_codes
		WHERE email = ? AND used_at IS NULL AND expires_at > ? AND failed_attempts < ?`, email, now, MaxSignInAttempts)
	if err != nil {
		return "", fmt.Errorf("find sign-in codes: %w", err)
	}
	defer rows.Close()

	matched := ""
	for rows.Next() {
		var id string
		var stored []byte
		if err := rows.Scan(&id, &stored); err != nil {
			return "", fmt.Errorf("read sign-in code: %w", err)
		}
		if subtle.ConstantTimeCompare(stored, codeHash) == 1 {
			matched = id
		}
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("find sign-in codes: %w", err)
	}
	return matched, nil
}

// AuthenticateSession finds the session whose secret hashes to hash and checks that it is
// current and that its person is active. A session in use is renewed for another
// SessionLifetime at most once an hour; renewals are bookkeeping and write no event.
func (s *Store) AuthenticateSession(ctx context.Context, hash []byte) (AuthenticatedSession, error) {
	session, err := scanSession(s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE session_hash = ?`, hash))
	if errors.Is(err, ErrNotFound) {
		return AuthenticatedSession{}, ErrUnauthenticated
	}
	if err != nil {
		return AuthenticatedSession{}, err
	}

	now := s.now().UTC().Truncate(time.Millisecond)
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return AuthenticatedSession{}, ErrUnauthenticated
	}

	actor, err := getActor(ctx, s.db, session.ActorID)
	if err != nil {
		return AuthenticatedSession{}, err
	}
	if actor.Kind != ActorHuman || !actor.Active() {
		return AuthenticatedSession{}, ErrUnauthenticated
	}

	result := AuthenticatedSession{Actor: actor, Session: session}
	if now.Sub(session.LastSeenAt) < sessionRenewInterval {
		return result, nil
	}

	// Renew only while the session is still unrevoked, in case it was revoked a moment ago.
	expires := now.Add(SessionLifetime)
	renewed, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET last_seen_at = ?1, expires_at = ?2, updated_at = ?1
		WHERE id = ?3 AND revoked_at IS NULL`, now.Format(timeLayout), expires.Format(timeLayout), session.ID)
	if err != nil {
		return AuthenticatedSession{}, fmt.Errorf("renew session: %w", err)
	}
	if count, err := renewed.RowsAffected(); err != nil || count == 0 {
		return AuthenticatedSession{}, ErrUnauthenticated
	}

	result.Session.LastSeenAt, result.Session.ExpiresAt, result.Session.UpdatedAt = now, expires, now
	result.Renewed = true
	return result, nil
}

// SessionCurrent reports whether a session is still unrevoked and unexpired and its person still
// active. Unlike AuthenticateSession, it renews nothing, so a long-running request can check it
// repeatedly.
func (s *Store) SessionCurrent(ctx context.Context, sessionID string) (bool, error) {
	var current bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions s JOIN actors a ON a.id = s.actor_id
			WHERE s.id = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND a.disabled_at IS NULL)`,
		sessionID, s.now().UTC().Format(timeLayout)).Scan(&current)
	if err != nil {
		return false, fmt.Errorf("check session: %w", err)
	}
	return current, nil
}

// ListSessions returns a person's current sessions, most recently used first.
func (s *Store) ListSessions(ctx context.Context, actorID string) ([]Session, error) {
	now := s.now().UTC().Format(timeLayout)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sessionColumns+` FROM sessions
		WHERE actor_id = ? AND revoked_at IS NULL AND expires_at > ?
		ORDER BY last_seen_at DESC, id`, actorID, now)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

// RevokeSession signs the caller out of one of their own devices. A session that is not the
// caller's is reported as not found, so its existence is not revealed.
func (s *Store) RevokeSession(ctx context.Context, by Caller, sessionID string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("sign out devices"); err != nil {
			return err
		}

		session, err := scanSession(w.tx.QueryRowContext(w.ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, sessionID))
		if err != nil {
			return err
		}
		if session.ActorID != w.caller.ID {
			return notFound("session")
		}
		if session.RevokedAt != nil {
			return nil
		}

		if _, err := w.exec(`UPDATE sessions SET revoked_at = ?1, updated_at = ?1 WHERE id = ?2`, w.timestamp(), session.ID); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}

		return w.event("", TargetActor, w.caller.ID, "session.revoked",
			map[string]any{"session_id": session.ID, "device_label": session.DeviceLabel})
	})
}

// RevokeAllSessions signs the caller out of every device, and returns how many sessions ended.
func (s *Store) RevokeAllSessions(ctx context.Context, by Caller) (int, error) {
	var count int64
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("sign out devices"); err != nil {
			return err
		}

		result, err := w.exec(`UPDATE sessions SET revoked_at = ?1, updated_at = ?1 WHERE actor_id = ?2 AND revoked_at IS NULL`,
			w.timestamp(), w.caller.ID)
		if err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		if count, err = result.RowsAffected(); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}

		return w.event("", TargetActor, w.caller.ID, "session.revoked_all", map[string]any{"count": count})
	})

	return int(count), err
}

func scanSession(row scanner) (Session, error) {
	var session Session
	var lastSeen, expires, created, updated string
	var revoked sql.NullString

	err := row.Scan(&session.ID, &session.ActorID, &session.DeviceLabel, &lastSeen, &expires, &revoked, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, notFound("session")
	}
	if err != nil {
		return Session{}, fmt.Errorf("read session: %w", err)
	}

	if session.LastSeenAt, err = parseTime(lastSeen); err != nil {
		return Session{}, err
	}
	if session.ExpiresAt, err = parseTime(expires); err != nil {
		return Session{}, err
	}
	if session.RevokedAt, err = parseOptionalTime(revoked); err != nil {
		return Session{}, err
	}
	if session.CreatedAt, session.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Session{}, err
	}

	return session, nil
}
