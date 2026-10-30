ALTER TABLE issues ADD COLUMN priority TEXT NOT NULL DEFAULT ''
CHECK (priority IN ('', 'urgent', 'high', 'medium', 'low'));
