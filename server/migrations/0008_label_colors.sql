ALTER TABLE labels ADD COLUMN color TEXT NOT NULL DEFAULT '#a7c0ff'
CHECK (length(color) = 7 AND color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]');

UPDATE labels SET color = CASE lower(name)
    WHEN 'ux' THEN '#c5a8ff'
    WHEN 'ui' THEN '#c5a8ff'
    WHEN 'frontend' THEN '#c5a8ff'
    WHEN 'feature' THEN '#a1e4cd'
    WHEN 'backend' THEN '#a1e4cd'
    WHEN 'bug' THEN '#f2a9ae'
    WHEN 'security' THEN '#f2a9ae'
    WHEN 'documentation' THEN '#f2cf87'
    WHEN 'research' THEN '#f2cf87'
    WHEN 'realtime' THEN '#8fd5da'
    WHEN 'accessibility' THEN '#8fd5da'
    WHEN 'database' THEN '#8fd5da'
    ELSE '#a7c0ff'
END;
