// Package logging builds the server's structured logger.
package logging

import (
	"io"
	"log/slog"
)

// New returns a JSON logger that writes records at level and above to w.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
