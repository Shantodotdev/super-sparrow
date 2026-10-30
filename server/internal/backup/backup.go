// Package backup copies the database to an R2 bucket, encrypted, and restores it.
//
// Both jobs take a consistent copy with VACUUM INTO, compress it with gzip, encrypt it with age to
// the team's public key, and upload it. The hourly sync replaces one copy; the daily backup keeps
// a dated copy per day and deletes those older than the configured number of days. The server
// holds only the public key, so it cannot read its own backups; restoring needs the private key,
// which the team keeps outside the server. A daily backup that fails is tried again with each
// hourly sync until one succeeds that day.
package backup

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	// SyncKey is where the hourly sync keeps its one copy.
	SyncKey     = "sync/sparrow.db.gz.age"
	dailyPrefix = "daily/"
	dateLayout  = "2006-01-02"
	syncEvery   = time.Hour
)

var dailyKeyPattern = regexp.MustCompile(`^daily/sparrow-(\d{4}-\d{2}-\d{2})\.db\.gz\.age$`)

// DailyKey returns where the daily backup of a day is kept. The date is day's own date, in
// whatever time zone day is in.
func DailyKey(day time.Time) string {
	return dailyPrefix + "sparrow-" + day.Format(dateLayout) + ".db.gz.age"
}

// Runner runs the backup jobs.
type Runner struct {
	store     *store.Store
	bucket    *Bucket
	recipient age.Recipient
	workDir   string
	keepDays  int
	dailyAt   time.Duration
	timeZone  *time.Location
	logger    *slog.Logger
	now       func() time.Time
}

// Options configures a Runner.
type Options struct {
	Store  *store.Store
	Bucket *Bucket
	// Recipient is the public key each copy is encrypted to. Restore needs its private key.
	Recipient age.Recipient
	// WorkDir holds the temporary copies while a job runs. Keep it on the database's disk, which
	// has room for a copy of the database.
	WorkDir  string
	KeepDays int
	// DailyAt is when the daily backup runs, as hours and minutes after midnight in TimeZone.
	DailyAt time.Duration
	// TimeZone is where DailyAt is read and where each daily backup's date comes from.
	TimeZone *time.Location
	Logger   *slog.Logger
	Now      func() time.Time
}

// NewRunner returns a Runner.
func NewRunner(opts Options) *Runner {
	return &Runner{
		store: opts.Store, bucket: opts.Bucket, recipient: opts.Recipient, workDir: opts.WorkDir,
		keepDays: opts.KeepDays, dailyAt: opts.DailyAt, timeZone: opts.TimeZone, logger: opts.Logger, now: opts.Now,
	}
}

// Run syncs once at start and then every hour, and runs the daily backup at its time each day,
// until ctx ends. A daily backup that was missed while the server was down, or that failed, runs
// at start and after each hourly sync until it succeeds. Jobs run one at a time, so they never
// overlap.
func (r *Runner) Run(ctx context.Context) {
	r.Sync(ctx)
	if r.dailyDue(ctx) {
		r.Daily(ctx)
	}

	syncTicker := time.NewTicker(syncEvery)
	defer syncTicker.Stop()
	dailyTimer := time.NewTimer(r.untilDaily(r.now()))
	defer dailyTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-syncTicker.C:
			r.Sync(ctx)
			if r.dailyDue(ctx) {
				r.Daily(ctx)
			}
		case <-dailyTimer.C:
			r.Daily(ctx)
			dailyTimer.Reset(r.untilDaily(r.now()))
		}
	}
}

// scheduled returns when the daily backup runs on the day of now, in the backup time zone.
func (r *Runner) scheduled(now time.Time) time.Time {
	local := now.In(r.timeZone)
	return time.Date(local.Year(), local.Month(), local.Day(),
		int(r.dailyAt/time.Hour), int(r.dailyAt%time.Hour/time.Minute), 0, 0, r.timeZone)
}

// untilDaily returns how long until the next daily backup time after now.
func (r *Runner) untilDaily(now time.Time) time.Duration {
	next := r.scheduled(now)
	if !next.After(now) {
		local := now.In(r.timeZone).AddDate(0, 0, 1)
		next = r.scheduled(local)
	}
	return next.Sub(now)
}

// dailyDue reports whether today's backup time has passed without a successful daily backup
// today, as after a restart across the backup time or after a failed run. Today is the date in
// the backup time zone.
func (r *Runner) dailyDue(ctx context.Context) bool {
	now := r.now()
	if now.Before(r.scheduled(now)) {
		return false
	}

	status, err := r.store.GetBackupStatus(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "could not check the last daily backup", "error", err)
		return false
	}
	last := status[store.BackupDaily].LastSuccess
	return last == nil || last.StartedAt.In(r.timeZone).Format(dateLayout) != now.In(r.timeZone).Format(dateLayout)
}

// Sync replaces the sync copy and records the run.
func (r *Runner) Sync(ctx context.Context) error {
	return r.record(ctx, store.BackupSync, SyncKey)
}

// Daily uploads today's dated copy, records the run, and deletes dated copies older than the
// number of days to keep.
func (r *Runner) Daily(ctx context.Context) error {
	today := r.now().In(r.timeZone)
	key := DailyKey(today)
	if err := r.record(ctx, store.BackupDaily, key); err != nil {
		return err
	}

	// The new copy is safely uploaded, so a failure to prune only leaves extra copies behind.
	if err := r.prune(ctx, today); err != nil {
		r.logger.ErrorContext(ctx, "deleting old daily backups failed", "error", err)
		return err
	}
	return nil
}

// record uploads a copy to key for job, recording the run's start and result. A failure is
// logged and returned.
func (r *Runner) record(ctx context.Context, job store.BackupJob, key string) error {
	run, err := r.store.StartBackupRun(ctx, job, key)
	if err != nil {
		r.logger.ErrorContext(ctx, "backup could not start", "job", job, "error", err)
		return err
	}

	started := r.now()
	size, runErr := r.upload(ctx, key)

	// Record the result even when shutdown canceled the job.
	if err := r.store.FinishBackupRun(context.WithoutCancel(ctx), run.ID, size, runErr); err != nil {
		r.logger.ErrorContext(ctx, "backup result not recorded", "job", job, "error", err)
	}
	if runErr != nil {
		r.logger.ErrorContext(ctx, "backup failed", "job", job, "key", key, "error", runErr)
		return runErr
	}

	r.logger.InfoContext(ctx, "backup uploaded", "job", job, "key", key, "bytes", size, "duration_ms", r.now().Sub(started).Milliseconds())
	return nil
}

// upload copies, compresses, encrypts, and uploads the database to key, and returns the
// uploaded size.
func (r *Runner) upload(ctx context.Context, key string) (int64, error) {
	dir, err := os.MkdirTemp(r.workDir, ".sparrow-backup-")
	if err != nil {
		return 0, fmt.Errorf("make a working folder: %w", err)
	}
	defer os.RemoveAll(dir)

	snapshot := filepath.Join(dir, "snapshot.db")
	if err := r.store.VacuumInto(ctx, snapshot); err != nil {
		return 0, err
	}

	encrypted := filepath.Join(dir, "snapshot.db.gz.age")
	if err := encryptFile(snapshot, encrypted, r.recipient); err != nil {
		return 0, err
	}
	if err := os.Remove(snapshot); err != nil {
		return 0, fmt.Errorf("remove the plain copy: %w", err)
	}

	return r.bucket.Put(ctx, key, encrypted)
}

// prune deletes the dated copies older than the number of days to keep, counting today. today is
// in the backup time zone, as the copies' dates are.
func (r *Runner) prune(ctx context.Context, today time.Time) error {
	objects, err := r.bucket.List(ctx, dailyPrefix)
	if err != nil {
		return err
	}

	// Copies' dates are parsed as UTC midnights, so today's date is compared the same way.
	oldest := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-r.keepDays)
	var problems []error
	for _, object := range objects {
		match := dailyKeyPattern.FindStringSubmatch(object.Key)
		if match == nil {
			continue
		}
		day, err := time.Parse(dateLayout, match[1])
		if err != nil || !day.Before(oldest) {
			continue
		}

		if err := r.bucket.Delete(ctx, object.Key); err != nil {
			problems = append(problems, err)
			continue
		}
		r.logger.InfoContext(ctx, "old daily backup deleted", "key", object.Key)
	}
	return errors.Join(problems...)
}

// encryptFile writes the gzip-compressed contents of source to target, encrypted to recipient.
func encryptFile(source, target string, recipient age.Recipient) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open copy: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create encrypted copy: %w", err)
	}
	defer out.Close()

	encrypting, err := age.Encrypt(out, recipient)
	if err != nil {
		return fmt.Errorf("start encryption: %w", err)
	}
	compressing, err := gzip.NewWriterLevel(encrypting, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("start compression: %w", err)
	}

	if _, err := io.Copy(compressing, in); err != nil {
		return fmt.Errorf("compress and encrypt: %w", err)
	}
	if err := compressing.Close(); err != nil {
		return fmt.Errorf("finish compression: %w", err)
	}
	if err := encrypting.Close(); err != nil {
		return fmt.Errorf("finish encryption: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("save encrypted copy: %w", err)
	}
	return out.Close()
}

// ResolveCopy turns a copy's name into its key: "sync" for the hourly copy, "daily/2026-03-01"
// for a dated copy, or a full key as List shows it.
func ResolveCopy(name string) (string, error) {
	switch {
	case name == "sync":
		return SyncKey, nil
	case strings.HasPrefix(name, dailyPrefix) && !strings.HasSuffix(name, ".age"):
		day, err := time.Parse(dateLayout, strings.TrimPrefix(name, dailyPrefix))
		if err != nil {
			return "", fmt.Errorf("copy %q must look like daily/2026-03-01", name)
		}
		return DailyKey(day), nil
	case name == SyncKey || dailyKeyPattern.MatchString(name):
		return name, nil
	}
	return "", fmt.Errorf("copy %q must be sync, daily/YYYY-MM-DD, or a key from --list", name)
}

// Restore downloads the copy at key, decrypts it with identities, the private keys the copies may
// be encrypted to, and writes it as the database at target after PRAGMA integrity_check passes. It refuses to replace an existing database, so a
// restore never overwrites data by accident; move the old file aside first.
func Restore(ctx context.Context, bucket *Bucket, key string, identities []age.Identity, target string) (int64, error) {
	for _, path := range []string{target, target + "-wal", target + "-shm"} {
		if _, err := os.Stat(path); err == nil {
			return 0, fmt.Errorf("%s already exists; move the old database aside before restoring", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("check %s: %w", path, err)
		}
	}

	dir, err := os.MkdirTemp(filepath.Dir(target), ".sparrow-restore-")
	if err != nil {
		return 0, fmt.Errorf("make a working folder next to %s: %w", target, err)
	}
	defer os.RemoveAll(dir)

	body, err := bucket.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	restored := filepath.Join(dir, "restored.db")
	size, err := decryptTo(body, identities, restored)
	if err != nil {
		return 0, err
	}
	if err := store.CheckDatabaseFile(ctx, restored); err != nil {
		return 0, err
	}

	if err := os.Rename(restored, target); err != nil {
		return 0, fmt.Errorf("move the restored database into place: %w", err)
	}
	return size, nil
}

// decryptTo decrypts and decompresses an encrypted copy into path and returns its size.
func decryptTo(encrypted io.Reader, identities []age.Identity, path string) (int64, error) {
	decrypting, err := age.Decrypt(encrypted, identities...)
	if err != nil {
		return 0, fmt.Errorf("decrypt the copy; is this the private key of the BACKUP_AGE_RECIPIENT it was made with? %w", err)
	}
	decompressing, err := gzip.NewReader(decrypting)
	if err != nil {
		return 0, fmt.Errorf("decompress the copy: %w", err)
	}
	defer decompressing.Close()

	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", path, err)
	}
	defer out.Close()

	size, err := io.Copy(out, decompressing)
	if err != nil {
		return 0, fmt.Errorf("write the restored database: %w", err)
	}
	if err := out.Sync(); err != nil {
		return 0, fmt.Errorf("save the restored database: %w", err)
	}
	return size, out.Close()
}
