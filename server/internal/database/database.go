// Package database opens the SQLite database and applies its migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

// Open opens the SQLite database at path, creating it if needed, and checks the connection.
//
// Every connection uses WAL mode so reads never wait for writes, enforces foreign keys, waits up
// to five seconds for a lock instead of failing at once, and starts transactions with
// BEGIN IMMEDIATE so a transaction that writes never fails halfway when it upgrades its lock.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	query := url.Values{}
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Set("_txlock", "immediate")

	db, err := sql.Open("sqlite", "file:"+path+"?"+query.Encode())
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to database %s: %w", path, err)
	}

	return db, nil
}
