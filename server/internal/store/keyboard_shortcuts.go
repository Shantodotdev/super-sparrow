package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shantodotdev/super-sparrow/server/internal/keyboard"
)

// KeyboardShortcuts contains one person's overrides and optimistic concurrency revision.
type KeyboardShortcuts struct {
	Revision  int               `json:"revision"`
	Overrides map[string]string `json:"overrides"`
}

// GetKeyboardShortcuts returns only the authenticated human caller's personal preferences.
func (s *Store) GetKeyboardShortcuts(ctx context.Context, by Caller) (KeyboardShortcuts, error) {
	actor, err := getActor(ctx, s.db, by.ActorID)
	if err != nil {
		return KeyboardShortcuts{}, err
	}
	if actor.Kind != ActorHuman || !actor.Active() {
		return KeyboardShortcuts{}, forbidden("only an active person can read keyboard shortcuts")
	}
	return getKeyboardShortcuts(ctx, s.db, actor.ID)
}

func getKeyboardShortcuts(ctx context.Context, q querier, actorID string) (KeyboardShortcuts, error) {
	preferences := KeyboardShortcuts{Overrides: map[string]string{}}
	var encoded string
	err := q.QueryRowContext(ctx, `SELECT revision, bindings FROM actor_keyboard_shortcuts WHERE actor_id = ?`, actorID).Scan(&preferences.Revision, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return preferences, nil
	}
	if err != nil {
		return KeyboardShortcuts{}, fmt.Errorf("load keyboard shortcuts for %s: %w", actorID, err)
	}
	if err := json.Unmarshal([]byte(encoded), &preferences.Overrides); err != nil {
		return KeyboardShortcuts{}, fmt.Errorf("decode keyboard shortcuts for %s: %w", actorID, err)
	}
	return preferences, nil
}

// UpdateKeyboardShortcut saves a single override, or removes it when binding is nil. It refuses
// stale revisions and commits the authenticated actor's event with the preference update.
func (s *Store) UpdateKeyboardShortcut(ctx context.Context, by Caller, revision int, action string, binding *string) (KeyboardShortcuts, error) {
	if _, ok := keyboard.Definitions()[action]; !ok {
		return KeyboardShortcuts{}, invalid("action", "is not a supported shortcut action")
	}
	var preferences KeyboardShortcuts
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("edit keyboard shortcuts"); err != nil {
			return err
		}
		var err error
		preferences, err = getKeyboardShortcuts(w.ctx, w.tx, w.caller.ID)
		if err != nil {
			return err
		}
		if revision != preferences.Revision {
			return conflict("keyboard shortcuts changed; reload before saving")
		}
		if binding == nil {
			delete(preferences.Overrides, action)
		} else {
			preferences.Overrides[action] = *binding
		}
		if err := keyboard.Validate(preferences.Overrides); err != nil {
			return invalid("binding", "%s", err.Error())
		}
		encoded, err := json.Marshal(preferences.Overrides)
		if err != nil {
			return fmt.Errorf("encode keyboard shortcuts: %w", err)
		}
		preferences.Revision++
		_, err = w.exec(`INSERT INTO actor_keyboard_shortcuts (actor_id, revision, bindings, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?) ON CONFLICT (actor_id) DO UPDATE SET revision = excluded.revision,
            bindings = excluded.bindings, updated_at = excluded.updated_at`, w.caller.ID, preferences.Revision, string(encoded), w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("save keyboard shortcuts for %s: %w", w.caller.ID, err)
		}
		return w.event("", TargetActor, w.caller.ID, "actor.shortcuts_updated", map[string]any{"action": action, "binding": binding, "revision": preferences.Revision})
	})
	return preferences, err
}
