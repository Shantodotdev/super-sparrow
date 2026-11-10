ALTER TABLE projects ADD COLUMN secrets_enabled_at TEXT;

-- Members with this permission read and edit staging, production, and the team dev defaults.
-- Owners always can, so the flag only matters for members.
ALTER TABLE actors ADD COLUMN can_manage_environments INTEGER NOT NULL DEFAULT 0
  CHECK (can_manage_environments IN (0, 1));

-- One key in one config. A personal dev key belongs to one person; every other config belongs to
-- the team. Deleting a key keeps the row and its versions, so its history stays readable.
CREATE TABLE secrets (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  config TEXT NOT NULL CHECK (config IN ('dev_defaults', 'dev_personal', 'staging', 'production')),
  owner_actor_id TEXT REFERENCES actors (id),
  name TEXT NOT NULL
    CHECK (length(name) BETWEEN 1 AND 128 AND name GLOB '[A-Z_]*' AND name NOT GLOB '*[^A-Z0-9_]*'),
  current_version INTEGER NOT NULL CHECK (current_version >= 1),
  deleted_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK ((config = 'dev_personal') = (owner_actor_id IS NOT NULL))
) STRICT;

CREATE UNIQUE INDEX secrets_name ON secrets (project_id, config, ifnull(owner_actor_id, ''), name);
CREATE INDEX secrets_owner ON secrets (owner_actor_id) WHERE owner_actor_id IS NOT NULL;

-- Every value a key has had, encrypted by the server. A version without ciphertext records a delete.
CREATE TABLE secret_versions (
  secret_id TEXT NOT NULL REFERENCES secrets (id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK (version >= 1),
  ciphertext BLOB,
  key_id TEXT,
  actor_id TEXT NOT NULL REFERENCES actors (id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (secret_id, version),
  CHECK ((ciphertext IS NULL) = (key_id IS NULL))
) STRICT, WITHOUT ROWID;

CREATE TRIGGER secret_versions_no_update
BEFORE UPDATE ON secret_versions
BEGIN
  SELECT RAISE(ABORT, 'secret versions never change');
END;

CREATE TABLE secret_required_keys (
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  config TEXT NOT NULL CHECK (config IN ('dev', 'staging', 'production')),
  name TEXT NOT NULL
    CHECK (length(name) BETWEEN 1 AND 128 AND name GLOB '[A-Z_]*' AND name NOT GLOB '*[^A-Z0-9_]*'),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, config, name)
) STRICT, WITHOUT ROWID;

-- Read-only tokens for CI. Each reads one project's staging or production config.
CREATE TABLE service_tokens (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  config TEXT NOT NULL CHECK (config IN ('staging', 'production')),
  label TEXT NOT NULL CHECK (length(trim(label)) BETWEEN 1 AND 100),
  token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  display_hint TEXT NOT NULL,
  created_by_actor_id TEXT NOT NULL REFERENCES actors (id),
  last_used_at TEXT,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX service_tokens_project ON service_tokens (project_id);

-- Who read secret values. A personal dev entry names the person whose dev was read.
CREATE TABLE secret_access_log (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  config TEXT NOT NULL CHECK (config IN ('dev_defaults', 'dev_personal', 'staging', 'production')),
  owner_actor_id TEXT REFERENCES actors (id),
  actor_id TEXT REFERENCES actors (id),
  service_token_id TEXT REFERENCES service_tokens (id) ON DELETE CASCADE,
  action TEXT NOT NULL CHECK (action IN ('reveal', 'download')),
  names TEXT NOT NULL CHECK (json_valid(names)),
  created_at TEXT NOT NULL,
  CHECK ((actor_id IS NULL) != (service_token_id IS NULL)),
  CHECK ((config = 'dev_personal') = (owner_actor_id IS NOT NULL))
) STRICT;

CREATE INDEX secret_access_log_project ON secret_access_log (project_id, config, ifnull(owner_actor_id, ''), sequence);

CREATE TRIGGER secret_access_log_no_update
BEFORE UPDATE ON secret_access_log
BEGIN
  SELECT RAISE(ABORT, 'the access log is append-only');
END;
