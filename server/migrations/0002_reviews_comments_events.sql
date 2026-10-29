-- Review rounds, findings, comments, and the append-only event log.

CREATE TABLE review_rounds (
  id TEXT PRIMARY KEY,
  issue_id TEXT NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
  number INTEGER NOT NULL CHECK (number >= 1),
  from_commit TEXT,
  to_commit TEXT NOT NULL,
  includes_uncommitted INTEGER NOT NULL CHECK (includes_uncommitted IN (0, 1)),
  plan_coverage TEXT NOT NULL,
  summary TEXT NOT NULL,
  next_finding_number INTEGER NOT NULL DEFAULT 1 CHECK (next_finding_number >= 1),
  author_actor_id TEXT NOT NULL REFERENCES actors (id),
  finished_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (issue_id, number)
) STRICT;

-- An issue has at most one round in progress at a time.
CREATE UNIQUE INDEX review_rounds_one_open ON review_rounds (issue_id) WHERE finished_at IS NULL;

CREATE TABLE findings (
  id TEXT PRIMARY KEY,
  review_round_id TEXT NOT NULL REFERENCES review_rounds (id) ON DELETE CASCADE,
  number INTEGER NOT NULL CHECK (number >= 1),
  severity TEXT NOT NULL CHECK (severity IN ('blocking', 'should_fix', 'nit')),
  title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 200),
  problem TEXT NOT NULL CHECK (length(trim(problem)) >= 1),
  fix TEXT NOT NULL CHECK (length(trim(fix)) >= 1),
  evidence TEXT NOT NULL,
  file_path TEXT,
  line INTEGER CHECK (line >= 1),
  commit_sha TEXT,
  status TEXT NOT NULL
    CHECK (status IN ('open', 'fix', 'wontfix', 'later', 'fixed', 'verified')),
  decision_reason TEXT,
  decided_by_actor_id TEXT REFERENCES actors (id),
  decided_at TEXT,
  found_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  fixed_by_actor_id TEXT REFERENCES actors (id),
  fixed_in_commit TEXT,
  fixed_at TEXT,
  verified_by_actor_id TEXT REFERENCES actors (id),
  verified_at TEXT,
  duplicate_of_finding_id TEXT REFERENCES findings (id) ON DELETE SET NULL,
  follow_up_issue_id TEXT REFERENCES issues (id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (review_round_id, number),
  CHECK (duplicate_of_finding_id IS NULL OR duplicate_of_finding_id != id),
  CHECK (line IS NULL OR file_path IS NOT NULL),
  CHECK (status != 'wontfix' OR length(trim(decision_reason)) >= 1),
  CHECK (status = 'open' OR (decided_by_actor_id IS NOT NULL AND decided_at IS NOT NULL)),
  CHECK (status NOT IN ('fixed', 'verified') OR (fixed_by_actor_id IS NOT NULL AND fixed_at IS NOT NULL)),
  CHECK (status != 'verified' OR (verified_by_actor_id IS NOT NULL AND verified_at IS NOT NULL))
) STRICT;

CREATE INDEX findings_status ON findings (review_round_id, status);
CREATE INDEX findings_duplicate ON findings (duplicate_of_finding_id)
  WHERE duplicate_of_finding_id IS NOT NULL;
CREATE INDEX findings_follow_up ON findings (follow_up_issue_id) WHERE follow_up_issue_id IS NOT NULL;

CREATE TABLE comments (
  id TEXT PRIMARY KEY,
  issue_id TEXT REFERENCES issues (id) ON DELETE CASCADE,
  finding_id TEXT REFERENCES findings (id) ON DELETE CASCADE,
  parent_comment_id TEXT REFERENCES comments (id) ON DELETE CASCADE,
  body TEXT NOT NULL CHECK (length(trim(body)) BETWEEN 1 AND 20000),
  author_actor_id TEXT NOT NULL REFERENCES actors (id),
  edited_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK ((issue_id IS NULL) != (finding_id IS NULL))
) STRICT;

CREATE INDEX comments_issue ON comments (issue_id, created_at) WHERE issue_id IS NOT NULL;
CREATE INDEX comments_finding ON comments (finding_id, created_at) WHERE finding_id IS NOT NULL;
CREATE INDEX comments_parent ON comments (parent_comment_id) WHERE parent_comment_id IS NOT NULL;

-- A reply must be on the same issue or finding as the comment it answers.
CREATE TRIGGER comments_reply_same_target
BEFORE INSERT ON comments
WHEN NEW.parent_comment_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM comments
    WHERE id = NEW.parent_comment_id
      AND issue_id IS NEW.issue_id
      AND finding_id IS NEW.finding_id
  )
BEGIN
  SELECT RAISE(ABORT, 'a reply must be on the same target as its parent');
END;

-- Events have no updated_at: they never change. project_id and target_id are plain values, not
-- foreign keys, so a project's history outlives its deletion.
CREATE TABLE events (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  project_id TEXT,
  target_type TEXT NOT NULL
    CHECK (target_type IN ('actor', 'project', 'milestone', 'label', 'issue', 'review_round', 'finding', 'comment')),
  target_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  actor_id TEXT NOT NULL REFERENCES actors (id),
  reported_model TEXT,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  created_at TEXT NOT NULL
) STRICT;

CREATE INDEX events_target ON events (target_type, target_id, sequence);
CREATE INDEX events_project ON events (project_id, sequence) WHERE project_id IS NOT NULL;
CREATE INDEX events_actor ON events (actor_id, sequence);

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
