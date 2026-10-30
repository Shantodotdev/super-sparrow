ALTER TABLE actors ADD COLUMN avatar_id TEXT NOT NULL DEFAULT ''
CHECK (avatar_id = '' OR (kind = 'human' AND avatar_id NOT GLOB '*[^a-z0-9-]*'));
