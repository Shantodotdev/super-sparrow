-- An agent's display name is its tool, such as "Claude Code". Agents created before this were
-- named "Claude Code (Shanto)"; their owner is already in owner_actor_id, so the name drops it.
UPDATE actors
SET
  display_name = CASE agent_name
    WHEN 'claude-code' THEN 'Claude Code'
    WHEN 'codex' THEN 'Codex'
    ELSE agent_name
  END,
  updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE kind = 'agent'
  AND display_name != CASE agent_name
    WHEN 'claude-code' THEN 'Claude Code'
    WHEN 'codex' THEN 'Codex'
    ELSE agent_name
  END;
