package store

import (
	"fmt"
	"strings"
)

// newProjectSlug allocates a stable URL segment inside the project's write transaction.
func newProjectSlug(w *writeTx, name, key string) (string, error) {
	// Keep URL segments ASCII and collapse punctuation into one separator.
	var normalized strings.Builder
	separator := false
	for _, character := range name {
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && normalized.Len() > 0 {
				normalized.WriteByte('-')
			}
			normalized.WriteRune(character)
			separator = false
		} else {
			separator = true
		}
	}
	base := normalized.String()
	if base == "" {
		base = strings.ToLower(key)
	}
	switch base {
	case "issues", "activity", "settings", "signin", "findings", "milestones", "api", "assets":
		base = "project-" + base
	}

	// The write transaction keeps concurrent projects from claiming the same candidate.
	for number := 1; ; number++ {
		candidate := base
		if number > 1 {
			candidate = fmt.Sprintf("%s-%d", base, number)
		}
		var exists bool
		if err := w.tx.QueryRowContext(w.ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE slug = ?)`, candidate).Scan(&exists); err != nil {
			return "", fmt.Errorf("allocate project slug: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
}
