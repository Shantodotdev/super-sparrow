CREATE TABLE mcp_retry_receipts (
  actor_id TEXT NOT NULL REFERENCES actors (id),
  request_key TEXT NOT NULL CHECK (length(request_key) BETWEEN 1 AND 200),
  request_hash TEXT NOT NULL,
  result TEXT NOT NULL CHECK (json_valid(result)),
  created_at TEXT NOT NULL,
  PRIMARY KEY (actor_id, request_key)
) STRICT;
