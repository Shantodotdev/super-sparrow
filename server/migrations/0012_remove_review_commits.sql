-- Reviews and findings track work without source-control revisions.
ALTER TABLE review_rounds DROP COLUMN from_commit;
ALTER TABLE review_rounds DROP COLUMN to_commit;
ALTER TABLE review_rounds DROP COLUMN includes_uncommitted;
ALTER TABLE findings DROP COLUMN commit_sha;
ALTER TABLE findings DROP COLUMN fixed_in_commit;

-- Remove retired metadata from existing events while retaining their actor, sequence and content.
-- Restore the append-only guard in this same migration transaction.
DROP TRIGGER events_no_update;

UPDATE events
SET payload = json_remove(payload, '$.from_commit', '$.to_commit', '$.includes_uncommitted', '$.commit_sha', '$.fixed_in_commit', '$.commit')
WHERE target_type IN ('review_round', 'finding')
  AND (json_type(payload, '$.from_commit') IS NOT NULL
    OR json_type(payload, '$.to_commit') IS NOT NULL
    OR json_type(payload, '$.includes_uncommitted') IS NOT NULL
    OR json_type(payload, '$.commit_sha') IS NOT NULL
    OR json_type(payload, '$.fixed_in_commit') IS NOT NULL
    OR json_type(payload, '$.commit') IS NOT NULL);

CREATE TRIGGER events_no_update
BEFORE UPDATE ON events
BEGIN
  SELECT RAISE(ABORT, 'events are append-only');
END;
