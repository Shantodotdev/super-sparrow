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

// Refusal is a refused request whose message is written for the person or agent who sent it.
// It wraps the sentinel error, such as ErrNotFound, that says how to answer it.
type Refusal struct {
	Message string
	Kind    error
}

func (e *Refusal) Error() string { return e.Message }
func (e *Refusal) Unwrap() error { return e.Kind }

// Refuse returns a Refusal of kind with a formatted message.
func Refuse(kind error, format string, args ...any) error {
	return &Refusal{Message: fmt.Sprintf(format, args...), Kind: kind}
}

// Explain returns the message to show the client for a refused request: the Refusal's own
// message, without the context added while the error was passed up, or err's text otherwise.
func Explain(err error) string {
	if refusal, ok := errors.AsType[*Refusal](err); ok {
		return refusal.Message
	}
	return err.Error()
}

func notFound(what string) error { return Refuse(ErrNotFound, "%s not found", what) }

func forbidden(reason string) error { return Refuse(ErrForbidden, "%s", reason) }

func conflict(reason string) error { return Refuse(ErrConflict, "%s", reason) }

// mapUniqueViolation turns a unique constraint failure into ErrConflict with reason, and wraps
// any other error with context.
func mapUniqueViolation(err error, reason, context string) error {
	if sqliteErr, ok := errors.AsType[*sqlite.Error](err); ok {
		code := sqliteErr.Code()
		if code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			return conflict(reason)
		}
	}

	return fmt.Errorf("%s: %w", context, err)
}
