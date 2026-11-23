CREATE TABLE discord_settings (
  guild_id TEXT PRIMARY KEY,
  default_project_id TEXT REFERENCES projects(id) ON DELETE SET NULL
) STRICT;

CREATE TABLE discord_links (
  guild_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  agent_id TEXT NOT NULL REFERENCES actors(id),
  PRIMARY KEY (guild_id, user_id),
  UNIQUE (guild_id, agent_id)
) STRICT;

CREATE UNIQUE INDEX discord_setup_interaction ON events (json_extract(payload, '$.interaction_id'))
WHERE kind = 'discord.setup';
