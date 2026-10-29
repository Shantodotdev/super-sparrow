package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BackupJob names one of the two backup jobs.
type BackupJob string

const (
	// BackupSync replaces one copy in the bucket every hour.
	BackupSync BackupJob = "sync"
	// BackupDaily keeps one dated copy per day.
	BackupDaily BackupJob = "daily"
)

// BackupResult is how a backup run ended, or that it is still going.
type BackupResult string

const (
	BackupRunning   BackupResult = "running"
	BackupSucceeded BackupResult = "succeeded"
	BackupFailed    BackupResult = "failed"
)

// BackupRun records one run of a backup job.
type BackupRun struct {
	ID         string
	Job        BackupJob
	ObjectKey  string
	Result     BackupResult
	SizeBytes  int64
	Error      string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// BackupStatus is what settings show for one job: its last run and its last successful run.
// Either is nil when there has been none.
type BackupStatus struct {
	LastRun     *BackupRun
	LastSuccess *BackupRun
}

const backupRunColumns = `id, job, object_key, result, size_bytes, error, started_at, finished_at`

// VacuumInto writes a consistent copy of the whole database to path, which must not exist. Writes
// can continue while it runs; the copy holds the database as it was when the copy started.
func (s *Store) VacuumInto(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("copy database to %s: %w", path, err)
	}
	return nil
}

// StartBackupRun records that a job started uploading to objectKey. Backup runs are bookkeeping,
// not changes anyone made, so they write no events.
func (s *Store) StartBackupRun(ctx context.Context, job BackupJob, objectKey string) (BackupRun, error) {
	now := s.now().UTC().Truncate(time.Millisecond)
	run := BackupRun{ID: newID(), Job: job, ObjectKey: objectKey, Result: BackupRunning, StartedAt: now}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_runs (id, job, object_key, result, started_at, created_at, updated_at)
		VALUES (?, ?, ?, 'running', ?, ?, ?)`,
		run.ID, job, objectKey, now.Format(timeLayout), now.Format(timeLayout), now.Format(timeLayout))
	if err != nil {
		return BackupRun{}, fmt.Errorf("record %s backup start: %w", job, err)
	}
	return run, nil
}

// FinishBackupRun records how a run ended: its uploaded size on success, or runErr on failure.
func (s *Store) FinishBackupRun(ctx context.Context, runID string, sizeBytes int64, runErr error) error {
	now := s.now().UTC().Truncate(time.Millisecond).Format(timeLayout)
	result, size, message := BackupSucceeded, sql.NullInt64{Int64: sizeBytes, Valid: true}, sql.NullString{}
	if runErr != nil {
		result, size = BackupFailed, sql.NullInt64{}
		message = sql.NullString{String: runErr.Error(), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE backup_runs SET result = ?, size_bytes = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE id = ? AND result = 'running'`,
		result, size, message, now, now, runID)
	if err != nil {
		return fmt.Errorf("record backup result: %w", err)
	}
	return nil
}

// GetBackupStatus returns the last run and last successful run of each job.
func (s *Store) GetBackupStatus(ctx context.Context) (map[BackupJob]BackupStatus, error) {
	statuses := map[BackupJob]BackupStatus{}
	for _, job := range []BackupJob{BackupSync, BackupDaily} {
		lastRun, err := s.lastBackupRun(ctx, `job = ?`, job)
		if err != nil {
			return nil, err
		}
		lastSuccess, err := s.lastBackupRun(ctx, `job = ? AND result = 'succeeded'`, job)
		if err != nil {
			return nil, err
		}
		statuses[job] = BackupStatus{LastRun: lastRun, LastSuccess: lastSuccess}
	}
	return statuses, nil
}

func (s *Store) lastBackupRun(ctx context.Context, where string, job BackupJob) (*BackupRun, error) {
	var run BackupRun
	var size sql.NullInt64
	var message, finished sql.NullString
	var started string

	err := s.db.QueryRowContext(ctx, `SELECT `+backupRunColumns+` FROM backup_runs WHERE `+where+
		` ORDER BY started_at DESC, id DESC LIMIT 1`, job).
		Scan(&run.ID, &run.Job, &run.ObjectKey, &run.Result, &size, &message, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s backup run: %w", job, err)
	}

	run.SizeBytes, run.Error = size.Int64, message.String
	if run.StartedAt, err = parseTime(started); err != nil {
		return nil, err
	}
	if run.FinishedAt, err = parseOptionalTime(finished); err != nil {
		return nil, err
	}
	return &run, nil
}

// CheckDatabaseFile opens the database file at path read-only and runs PRAGMA integrity_check,
// as a restore does before it puts a copy in place.
func CheckDatabaseFile(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("check %s: %w", path, err)
	}
	defer rows.Close()

	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return fmt.Errorf("read integrity check: %w", err)
		}
		if line != "ok" {
			problems = append(problems, line)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check %s: %w", path, err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s failed its integrity check: %s", path, strings.Join(problems, "; "))
	}
	return nil
}
