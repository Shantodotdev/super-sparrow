-- Issue history lookups, backup run records, and links from imported records to their source.

-- Events about an issue's rounds, findings, and comments carry the issue's ID in their payload,
-- so an issue's history is one indexed lookup.
CREATE INDEX events_issue ON events (json_extract(payload, '$.issue_id'), sequence)
  WHERE json_extract(payload, '$.issue_id') IS NOT NULL;

CREATE TABLE backup_runs (
  id TEXT PRIMARY KEY,
  job TEXT NOT NULL CHECK (job IN ('sync', 'daily')),
  object_key TEXT NOT NULL,
  result TEXT NOT NULL CHECK (result IN ('running', 'succeeded', 'failed')),
  size_bytes INTEGER CHECK (size_bytes >= 0),
  error TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK ((result = 'running') = (finished_at IS NULL)),
  CHECK (result != 'succeeded' OR size_bytes IS NOT NULL),
  CHECK (result != 'failed' OR length(error) >= 1)
) STRICT;

CREATE INDEX backup_runs_job ON backup_runs (job, result, started_at);

-- An imported record remembers where it came from, so running an import again updates nothing
-- it already brought in.
CREATE TABLE import_links (
  id TEXT PRIMARY KEY,
  source TEXT NOT NULL CHECK (source IN ('linear')),
  external_id TEXT NOT NULL CHECK (length(external_id) BETWEEN 1 AND 200),
  target_type TEXT NOT NULL
    CHECK (target_type IN ('project', 'milestone', 'label', 'issue', 'review_round', 'comment')),
  target_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (source, external_id)
) STRICT;
