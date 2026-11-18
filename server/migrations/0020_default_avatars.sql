WITH avatars(id) AS (
	VALUES
		('adventurers-cat-pilot'),
		('adventurers-dragon-ember'),
		('adventurers-fox-rogue'),
		('adventurers-frog-adventurer'),
		('adventurers-owl-guardian'),
		('adventurers-rabbit-scholar'),
		('adventurers-wolf-shadow'),
		('approved-samples-cloud-companion'),
		('approved-samples-sparrow-explorer'),
		('forest-badger-warrior'),
		('forest-fox-rogue'),
		('forest-frog-adventurer'),
		('forest-lynx-shadow'),
		('forest-owl-sage'),
		('forest-squirrel-maker'),
		('forest-wolf-guardian'),
		('magical-dragon'),
		('magical-forest-spirit'),
		('magical-moon-spirit'),
		('magical-phoenix'),
		('magical-shadow-cat'),
		('magical-storm-spirit'),
		('magical-sun-spirit'),
		('nerdy-dragon-alchemist'),
		('nerdy-fox-librarian'),
		('nerdy-owl-astronomer'),
		('nerdy-raccoon-coder'),
		('nerdy-sparrow-inventor'),
		('sparrows-sparrow-01'),
		('sparrows-sparrow-02'),
		('sparrows-sparrow-03'),
		('sparrows-sparrow-04'),
		('sparrows-sparrow-05'),
		('sparrows-sparrow-06'),
		('sparrows-sparrow-07')
),
unused AS (
	SELECT id, 0 AS used FROM avatars
	WHERE id NOT IN (SELECT avatar_id FROM actors WHERE kind = 'human' AND avatar_id != '')
),
taken AS (
	SELECT id, 1 AS used FROM avatars
	WHERE id IN (SELECT avatar_id FROM actors WHERE kind = 'human' AND avatar_id != '')
),
pool AS (
	SELECT id, row_number() OVER (ORDER BY used, random()) - 1 AS position
	FROM (SELECT * FROM unused UNION ALL SELECT * FROM taken)
),
people AS (
	SELECT id, row_number() OVER (ORDER BY random()) - 1 AS position
	FROM actors
	WHERE kind = 'human' AND avatar_id = ''
)
UPDATE actors
SET avatar_id = (
	SELECT pool.id FROM pool
	WHERE pool.position = (SELECT people.position FROM people WHERE people.id = actors.id) % (SELECT count(*) FROM pool)
)
WHERE kind = 'human' AND avatar_id = '';
