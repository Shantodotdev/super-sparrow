-- Keys an issue had before it moved to another project, so old links and branch names still find
-- it. Issue numbers are never reused, so an old key cannot belong to another issue.
CREATE TABLE issue_key_aliases (
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  number INTEGER NOT NULL CHECK (number >= 1),
  issue_id TEXT NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, number)
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_key_aliases_issue ON issue_key_aliases (issue_id);
