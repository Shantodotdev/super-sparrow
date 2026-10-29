package store

import (
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	// ErrNotFound means the record does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict means the change clashes with existing data, such as a duplicate project key.
	ErrConflict = errors.New("conflict")
	// ErrForbidden means the caller may not make this change.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidTransition means the record cannot move from its current state to the requested one.
	ErrInvalidTransition = errors.New("invalid transition")
	// ErrUnauthenticated means a session or agent token is unknown, expired, revoked, or belongs
	// to a disabled actor.
	ErrUnauthenticated = errors.New("not signed in")
	// ErrInvalidCode means a sign-in code is wrong, expired, used, or canceled. The reasons are not
	// told apart, so a guesser learns nothing from them.
	ErrInvalidCode = errors.New("the code is wrong or has expired")
)

// ValidationError reports one invalid input field, so the API can name it in its response.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

func notFound(what string) error { return fmt.Errorf("%s: %w", what, ErrNotFound) }

func forbidden(reason string) error { return fmt.Errorf("%s: %w", reason, ErrForbidden) }

func conflict(reason string) error { return fmt.Errorf("%s: %w", reason, ErrConflict) }

// mapUniqueViolation turns a unique constraint failure into ErrConflict with reason, and wraps
// any other error with context.
func mapUniqueViolation(err error, reason, context string) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		if code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			return conflict(reason)
		}
	}

	return fmt.Errorf("%s: %w", context, err)
}
