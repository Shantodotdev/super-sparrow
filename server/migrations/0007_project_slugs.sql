ALTER TABLE projects ADD COLUMN slug TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX projects_slug_backfill ON projects(slug) WHERE slug <> '';

CREATE TEMP TABLE project_slug_backfill (project_id TEXT NOT NULL, base TEXT NOT NULL);
CREATE TEMP TRIGGER assign_project_slug AFTER INSERT ON project_slug_backfill BEGIN
    UPDATE projects SET slug = (
        WITH RECURSIVE candidates(number, candidate) AS (
            SELECT 1, NEW.base
            UNION ALL
            SELECT number + 1, NEW.base || '-' || (number + 1)
            FROM candidates
            WHERE EXISTS (SELECT 1 FROM projects WHERE slug = candidate)
        )
        SELECT candidate FROM candidates
        WHERE NOT EXISTS (SELECT 1 FROM projects WHERE slug = candidate)
        LIMIT 1
    ) WHERE id = NEW.project_id;
END;

WITH RECURSIVE normalized(id, key, name, position, slug) AS (
    SELECT id, key, lower(name), 1, '' FROM projects
    UNION ALL
    SELECT id, key, name, position + 1,
        CASE
            WHEN substr(name, position, 1) GLOB '[a-z0-9]' THEN slug || substr(name, position, 1)
            WHEN slug <> '' AND substr(slug, -1) <> '-' THEN slug || '-'
            ELSE slug
        END
    FROM normalized WHERE position <= length(name)
), bases AS (
    SELECT id, coalesce(nullif(trim(slug, '-'), ''), lower(key)) AS base
    FROM normalized WHERE position > length(name)
)
INSERT INTO project_slug_backfill
SELECT id, CASE WHEN base IN ('issues', 'activity', 'settings', 'signin', 'findings', 'milestones', 'api', 'assets')
    THEN 'project-' || base ELSE base END
FROM bases ORDER BY id;

DROP TRIGGER assign_project_slug;
DROP TABLE project_slug_backfill;
DROP INDEX projects_slug_backfill;
CREATE UNIQUE INDEX projects_slug ON projects(slug);

CREATE TRIGGER projects_slug_required BEFORE INSERT ON projects
WHEN NEW.slug = '' OR NEW.slug GLOB '*[^a-z0-9-]*'
    OR substr(NEW.slug, 1, 1) = '-' OR substr(NEW.slug, -1) = '-' OR instr(NEW.slug, '--') > 0
    OR NEW.slug IN ('issues', 'activity', 'settings', 'signin', 'findings', 'milestones', 'api', 'assets')
BEGIN
    SELECT RAISE(ABORT, 'invalid project slug');
END;

CREATE TRIGGER projects_slug_immutable BEFORE UPDATE OF slug ON projects
WHEN NEW.slug <> OLD.slug
BEGIN
    SELECT RAISE(ABORT, 'project slug cannot change');
END;
