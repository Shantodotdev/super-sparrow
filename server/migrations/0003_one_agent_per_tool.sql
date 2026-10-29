-- A person has one agent actor per tool, such as claude-code. Every token that person creates for
-- that tool acts as the same agent, so its history reads as one agent.
CREATE UNIQUE INDEX actors_agent_per_owner ON actors (owner_actor_id, agent_name) WHERE kind = 'agent';

-- Finds the sessions and tokens to show in settings without scanning revoked ones.
CREATE INDEX sessions_actor_active ON sessions (actor_id, last_seen_at) WHERE revoked_at IS NULL;
