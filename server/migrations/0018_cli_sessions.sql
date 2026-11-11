-- A session is either the desktop app's cookie or a signed-in CLI's bearer secret.
ALTER TABLE sessions ADD COLUMN client TEXT NOT NULL DEFAULT 'browser' CHECK (client IN ('browser', 'cli'));

-- A CLI asks to sign in and a person approves it in the desktop app. The CLI keeps the device
-- secret and polls with it; the person types or opens the short user code. Only hashes are kept.
CREATE TABLE cli_login_requests (
  id TEXT PRIMARY KEY,
  device_code_hash BLOB NOT NULL UNIQUE,
  user_code_hash BLOB NOT NULL UNIQUE,
  device_label TEXT NOT NULL,
  requested_from TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'denied', 'claimed')),
  approved_by_actor_id TEXT REFERENCES actors (id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX cli_login_requests_expires_at ON cli_login_requests (expires_at);
