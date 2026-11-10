-- Files people and agents attach to an issue. The bytes live in the attachments folder on disk,
-- named by the attachment's ID; the row keeps what is needed to serve and verify them.
CREATE TABLE attachments (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  issue_id TEXT NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
  filename TEXT NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
  content_type TEXT NOT NULL CHECK (length(content_type) BETWEEN 1 AND 255),
  byte_size INTEGER NOT NULL CHECK (byte_size >= 1),
  sha256 TEXT NOT NULL CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
  created_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX attachments_issue ON attachments (issue_id);
CREATE INDEX attachments_project ON attachments (project_id);

CREATE TRIGGER attachments_issue_same_project
BEFORE INSERT ON attachments
WHEN (SELECT project_id FROM issues WHERE id = NEW.issue_id) IS NOT NEW.project_id
BEGIN
  SELECT RAISE(ABORT, 'an attachment and its issue must be in the same project');
END;
