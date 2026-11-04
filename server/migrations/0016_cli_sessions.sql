-- A session is either a browser's cookie or a signed-in CLI's bearer secret. CLI sessions are kept
-- apart because they never read staging or production secrets.
ALTER TABLE sessions ADD COLUMN client TEXT NOT NULL DEFAULT 'browser' CHECK (client IN ('browser', 'cli'));
