ALTER TABLE projects ADD COLUMN next_context_number INTEGER NOT NULL DEFAULT 1
  CHECK (next_context_number >= 1);

-- Images people and agents attach to plans, descriptions, and contexts. Markdown refers to them
-- by URL, so an image belongs to its project rather than to one record.
CREATE TABLE attachments (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  content_type TEXT NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/gif', 'image/webp')),
  byte_size INTEGER NOT NULL CHECK (byte_size BETWEEN 1 AND 10485760),
  data BLOB NOT NULL CHECK (length(data) = byte_size),
  created_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX attachments_project ON attachments (project_id);

-- Contexts hold notes, inspiration, and screenshots outside the issue tracker, numbered per
-- project and shown as WP-C3. A context may point at the issue it informs.
CREATE TABLE contexts (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  number INTEGER NOT NULL CHECK (number >= 1),
  title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 200),
  body TEXT NOT NULL CHECK (length(body) <= 100000),
  body_revision INTEGER NOT NULL CHECK (body_revision >= 1),
  issue_id TEXT REFERENCES issues (id) ON DELETE SET NULL,
  created_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (project_id, number)
) STRICT;

CREATE INDEX contexts_issue ON contexts (issue_id) WHERE issue_id IS NOT NULL;

CREATE TRIGGER contexts_issue_same_project_insert
BEFORE INSERT ON contexts
WHEN NEW.issue_id IS NOT NULL
  AND (SELECT project_id FROM issues WHERE id = NEW.issue_id) IS NOT NEW.project_id
BEGIN
  SELECT RAISE(ABORT, 'a context and its issue must be in the same project');
END;

CREATE TRIGGER contexts_issue_same_project_update
BEFORE UPDATE OF issue_id ON contexts
WHEN NEW.issue_id IS NOT NULL
  AND (SELECT project_id FROM issues WHERE id = NEW.issue_id) IS NOT NEW.project_id
BEGIN
  SELECT RAISE(ABORT, 'a context and its issue must be in the same project');
END;
