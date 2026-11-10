CREATE TABLE actor_keyboard_shortcuts (
  actor_id TEXT PRIMARY KEY REFERENCES actors (id),
  revision INTEGER NOT NULL CHECK (revision >= 1),
  bindings TEXT NOT NULL CHECK (json_valid(bindings) AND json_type(bindings) = 'object'),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;
