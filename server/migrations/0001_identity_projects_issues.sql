-- Actors, credentials, projects, milestones, labels, and issues.
-- Times are UTC text in the form 2026-01-02T15:04:05.000Z, written by the server.

CREATE TABLE actors (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('human', 'agent', 'system')),
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 100),
  email TEXT CHECK (email = lower(email)),
  role TEXT CHECK (role IN ('owner', 'member')),
  agent_name TEXT,
  owner_actor_id TEXT REFERENCES actors (id),
  disabled_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK (
    (kind = 'human' AND email IS NOT NULL AND role IS NOT NULL
      AND agent_name IS NULL AND owner_actor_id IS NULL)
    OR (kind = 'agent' AND email IS NULL AND role IS NULL
      AND agent_name IS NOT NULL AND owner_actor_id IS NOT NULL)
    OR (kind = 'system' AND email IS NULL AND role IS NULL
      AND agent_name IS NULL AND owner_actor_id IS NULL)
  )
) STRICT;

CREATE UNIQUE INDEX actors_email ON actors (email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX actors_single_system ON actors (kind) WHERE kind = 'system';
CREATE INDEX actors_owner ON actors (owner_actor_id) WHERE owner_actor_id IS NOT NULL;

-- The one system actor records actions no person or agent took, such as creating the first owner.
INSERT INTO actors (id, kind, display_name, created_at, updated_at)
VALUES ('00000000-0000-7000-8000-000000000000', 'system', 'Super Sparrow',
  '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');

-- An agent may only belong to an active human.
CREATE TRIGGER actors_agent_owner_is_human
BEFORE INSERT ON actors
WHEN NEW.kind = 'agent'
  AND NOT EXISTS (SELECT 1 FROM actors WHERE id = NEW.owner_actor_id AND kind = 'human')
BEGIN
  SELECT RAISE(ABORT, 'an agent must belong to a human');
END;

CREATE TABLE agent_tokens (
  id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL REFERENCES actors (id),
  label TEXT NOT NULL CHECK (length(trim(label)) BETWEEN 1 AND 100),
  token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  display_hint TEXT NOT NULL,
  last_used_at TEXT,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX agent_tokens_actor ON agent_tokens (actor_id);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  actor_id TEXT NOT NULL REFERENCES actors (id),
  session_hash BLOB NOT NULL UNIQUE CHECK (length(session_hash) = 32),
  device_label TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX sessions_actor ON sessions (actor_id);

CREATE TABLE sign_in_codes (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL CHECK (email = lower(email)),
  code_hash BLOB NOT NULL CHECK (length(code_hash) = 32),
  expires_at TEXT NOT NULL,
  failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
  used_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX sign_in_codes_email ON sign_in_codes (email, created_at);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL UNIQUE
    CHECK (length(key) BETWEEN 2 AND 10 AND key NOT GLOB '*[^A-Z0-9]*' AND key GLOB '[A-Z]*'),
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
  next_issue_number INTEGER NOT NULL DEFAULT 1 CHECK (next_issue_number >= 1),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE milestones (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
  position INTEGER NOT NULL CHECK (position >= 0),
  status TEXT NOT NULL CHECK (status IN ('planned', 'active', 'done')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (project_id, name)
) STRICT;

CREATE TABLE labels (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 50),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (project_id, name)
) STRICT;

CREATE TABLE issues (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  number INTEGER NOT NULL CHECK (number >= 1),
  title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 200),
  plan TEXT NOT NULL,
  status TEXT NOT NULL
    CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'canceled')),
  parent_issue_id TEXT REFERENCES issues (id) ON DELETE SET NULL,
  milestone_id TEXT REFERENCES milestones (id) ON DELETE SET NULL,
  assignee_actor_id TEXT REFERENCES actors (id),
  branch_name TEXT,
  pull_request_url TEXT,
  next_review_round_number INTEGER NOT NULL DEFAULT 1 CHECK (next_review_round_number >= 1),
  created_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (project_id, number),
  CHECK (parent_issue_id IS NULL OR parent_issue_id != id)
) STRICT;

CREATE INDEX issues_project_status ON issues (project_id, status);
CREATE INDEX issues_parent ON issues (parent_issue_id) WHERE parent_issue_id IS NOT NULL;
CREATE INDEX issues_milestone ON issues (milestone_id) WHERE milestone_id IS NOT NULL;
CREATE INDEX issues_assignee ON issues (assignee_actor_id) WHERE assignee_actor_id IS NOT NULL;
CREATE UNIQUE INDEX issues_branch ON issues (project_id, branch_name) WHERE branch_name IS NOT NULL;

-- A parent issue and a milestone must belong to the issue's own project.
CREATE TRIGGER issues_same_project_insert
BEFORE INSERT ON issues
WHEN (NEW.parent_issue_id IS NOT NULL
    AND (SELECT project_id FROM issues WHERE id = NEW.parent_issue_id) IS NOT NEW.project_id)
  OR (NEW.milestone_id IS NOT NULL
    AND (SELECT project_id FROM milestones WHERE id = NEW.milestone_id) IS NOT NEW.project_id)
BEGIN
  SELECT RAISE(ABORT, 'parent issue and milestone must be in the same project');
END;

CREATE TRIGGER issues_same_project_update
BEFORE UPDATE OF parent_issue_id, milestone_id ON issues
WHEN (NEW.parent_issue_id IS NOT NULL
    AND (SELECT project_id FROM issues WHERE id = NEW.parent_issue_id) IS NOT NEW.project_id)
  OR (NEW.milestone_id IS NOT NULL
    AND (SELECT project_id FROM milestones WHERE id = NEW.milestone_id) IS NOT NEW.project_id)
BEGIN
  SELECT RAISE(ABORT, 'parent issue and milestone must be in the same project');
END;

CREATE TABLE issue_labels (
  issue_id TEXT NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
  label_id TEXT NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
  PRIMARY KEY (issue_id, label_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_labels_label ON issue_labels (label_id);

CREATE TRIGGER issue_labels_same_project
BEFORE INSERT ON issue_labels
WHEN (SELECT project_id FROM issues WHERE id = NEW.issue_id)
  IS NOT (SELECT project_id FROM labels WHERE id = NEW.label_id)
BEGIN
  SELECT RAISE(ABORT, 'a label must be in the same project as its issue');
END;
