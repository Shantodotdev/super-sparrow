CREATE TABLE discord_guild_approvals (
  guild_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'revoked')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;
