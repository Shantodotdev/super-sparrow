// Package migrations holds the database migrations, embedded into the server binary.
//
// Add a migration as the next numbered file, such as 0002_create_projects.sql. Never edit a
// migration that has been merged; add a new one instead.
package migrations

import "embed"

// Files holds every file in this folder. The migration runner applies only the .sql files.
//
//go:embed *
var Files embed.FS
