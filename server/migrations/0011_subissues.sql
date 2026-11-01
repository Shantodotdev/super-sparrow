ALTER TABLE issues ADD COLUMN next_subissue_number INTEGER NOT NULL DEFAULT 1
  CHECK (next_subissue_number >= 1);

CREATE TABLE subissues (
  id TEXT PRIMARY KEY,
  issue_id TEXT NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
  number INTEGER NOT NULL CHECK (number >= 1),
  title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 200),
  description TEXT NOT NULL CHECK (length(description) <= 100000),
  description_revision INTEGER NOT NULL CHECK (description_revision >= 1),
  status TEXT NOT NULL CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'canceled')),
  created_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (issue_id, number)
) STRICT;
