CREATE TABLE discord_link_codes (
  code_hash BLOB PRIMARY KEY,
  person_id TEXT NOT NULL UNIQUE REFERENCES actors(id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
) STRICT;

ALTER TABLE discord_links ADD COLUMN user_name TEXT NOT NULL DEFAULT '';
