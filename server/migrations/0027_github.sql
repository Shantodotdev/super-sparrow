-- GitHub App installations, repositories linked to projects, pull requests and their reviews,
-- teammates' GitHub logins, and the Discord channel each project posts pull request news to.

-- Events gain the repository and pull_request targets. SQLite cannot change a CHECK constraint,
-- so the table is rebuilt with every row, index, and trigger it had.
DROP TRIGGER events_no_update;
DROP TRIGGER events_no_delete;

CREATE TABLE events_rebuilt (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  project_id TEXT,
  target_type TEXT NOT NULL
    CHECK (target_type IN ('actor', 'project', 'milestone', 'label', 'issue', 'review_round', 'finding', 'comment', 'repository', 'pull_request')),
  target_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  actor_id TEXT NOT NULL REFERENCES actors (id),
  reported_model TEXT,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  created_at TEXT NOT NULL
) STRICT;

INSERT INTO events_rebuilt (sequence, id, project_id, target_type, target_id, kind, actor_id, reported_model, payload, created_at)
SELECT sequence, id, project_id, target_type, target_id, kind, actor_id, reported_model, payload, created_at FROM events;

DROP TABLE events;
ALTER TABLE events_rebuilt RENAME TO events;

CREATE INDEX events_target ON events (target_type, target_id, sequence);
CREATE INDEX events_project ON events (project_id, sequence) WHERE project_id IS NOT NULL;
CREATE INDEX events_actor ON events (actor_id, sequence);
CREATE INDEX events_issue ON events (json_extract(payload, '$.issue_id'), sequence)
  WHERE json_extract(payload, '$.issue_id') IS NOT NULL;
CREATE UNIQUE INDEX discord_setup_interaction ON events (json_extract(payload, '$.interaction_id'))
  WHERE kind = 'discord.setup';

CREATE TRIGGER events_no_update
BEFORE UPDATE ON events
BEGIN
  SELECT RAISE(ABORT, 'events are append-only');
END;

CREATE TRIGGER events_no_delete
BEFORE DELETE ON events
BEGIN
  SELECT RAISE(ABORT, 'events are append-only');
END;

CREATE TABLE github_installations (
  id INTEGER PRIMARY KEY,
  account_login TEXT NOT NULL,
  suspended INTEGER NOT NULL CHECK (suspended IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE github_repositories (
  id INTEGER PRIMARY KEY,
  installation_id INTEGER REFERENCES github_installations (id) ON DELETE SET NULL,
  full_name TEXT NOT NULL,
  html_url TEXT NOT NULL,
  private INTEGER NOT NULL CHECK (private IN (0, 1)),
  project_id TEXT REFERENCES projects (id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX github_repositories_project ON github_repositories (project_id) WHERE project_id IS NOT NULL;

CREATE TABLE github_pull_requests (
  id INTEGER PRIMARY KEY,
  repository_id INTEGER NOT NULL REFERENCES github_repositories (id) ON DELETE CASCADE,
  number INTEGER NOT NULL,
  title TEXT NOT NULL,
  html_url TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('open', 'closed', 'merged')),
  draft INTEGER NOT NULL CHECK (draft IN (0, 1)),
  author_login TEXT NOT NULL,
  head_ref TEXT NOT NULL,
  base_ref TEXT NOT NULL,
  head_sha TEXT NOT NULL,
  additions INTEGER NOT NULL,
  deletions INTEGER NOT NULL,
  changed_files INTEGER NOT NULL,
  requested_reviewers TEXT NOT NULL CHECK (json_valid(requested_reviewers)),
  review_state TEXT NOT NULL CHECK (review_state IN ('none', 'approved', 'changes_requested', 'commented')),
  checks_state TEXT NOT NULL CHECK (checks_state IN ('none', 'pending', 'success', 'failure')),
  merged_by_login TEXT NOT NULL,
  issue_id TEXT REFERENCES issues (id) ON DELETE SET NULL,
  opened_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  merged_at TEXT,
  closed_at TEXT,
  UNIQUE (repository_id, number)
) STRICT;

CREATE INDEX github_pull_requests_head ON github_pull_requests (repository_id, head_sha);

CREATE TABLE github_reviews (
  id INTEGER PRIMARY KEY,
  pull_request_id INTEGER NOT NULL REFERENCES github_pull_requests (id) ON DELETE CASCADE,
  reviewer_login TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('approved', 'changes_requested', 'commented', 'dismissed')),
  html_url TEXT NOT NULL,
  submitted_at TEXT NOT NULL
) STRICT;

CREATE INDEX github_reviews_pull_request ON github_reviews (pull_request_id, submitted_at);

-- Each webhook delivery is applied once, so a redelivery neither repeats events nor posts to
-- Discord twice.
CREATE TABLE github_deliveries (
  id TEXT PRIMARY KEY,
  event TEXT NOT NULL,
  received_at TEXT NOT NULL
) STRICT;

CREATE TABLE github_accounts (
  person_id TEXT PRIMARY KEY REFERENCES actors (id),
  login TEXT NOT NULL UNIQUE COLLATE NOCASE
) STRICT;

CREATE TABLE discord_project_channels (
  project_id TEXT PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
  guild_id TEXT NOT NULL,
  channel_id TEXT NOT NULL
) STRICT;
