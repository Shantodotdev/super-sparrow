// Package config loads the server's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	minWebhookSecretLength = 20
	minCodeKeyLength       = 32
	minBackupKeyLength     = 32
)

var r2AccountPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Environment says how strict the settings are. Production requires every setting; development
// leaves the frontend, the proxy list, the webhook, and backups optional, and turns off each one
// whose settings are absent.
type Environment string

const (
	Production  Environment = "production"
	Development Environment = "development"
)

// backupSettings are the settings of the backup jobs. Development needs all of them or none.
var backupSettings = []string{
	"BACKUP_ENCRYPTION_KEY", "BACKUP_DAILY_TIME", "BACKUP_KEEP_DAYS",
	"R2_ACCOUNT_ID", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "R2_BUCKET",
}

// Config holds every setting the server needs. Nothing has a default.
type Config struct {
	Environment  Environment
	ListenAddr   string
	DatabasePath string
	PublicURL    *url.URL
	LogLevel     slog.Level
	// ResendAPIKey authenticates the server to Resend, which sends sign-in codes and invitations.
	ResendAPIKey string
	// EmailFrom is the sender of every email, such as "Super Sparrow <sparrow@example.com>".
	EmailFrom string
	// SignInCodeKey is the secret sign-in codes are hashed with. It stays out of the database, so
	// a copy of the database does not let anyone work out a code.
	SignInCodeKey []byte
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header names the client's
	// address. It is empty in development when the server is reached directly.
	TrustedProxies []netip.Prefix
	// WebDir is the folder of the built frontend the server serves. It is "" in development, where
	// the web dev server serves the frontend.
	WebDir string
	// GitHubWebhookSecret checks GitHub webhook signatures. It is "" when the webhook is off,
	// which only development allows.
	GitHubWebhookSecret string
	// Backups configures the backup jobs. It is nil when they are off, which only development
	// allows.
	Backups *Backups
}

// Backups holds the settings of the hourly sync and the daily backup.
type Backups struct {
	// EncryptionKey encrypts every backup, and the restore command decrypts them with it.
	EncryptionKey string
	// DailyAt is when the daily backup runs, as time after midnight UTC.
	DailyAt time.Duration
	// KeepDays is how many days of daily backups to keep.
	KeepDays int
	R2       R2
}

// R2 holds the Cloudflare R2 bucket and credentials backups are uploaded to.
type R2 struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
}

// Load reads the settings through getenv, which is os.Getenv outside tests. It returns one error
// that names every setting that is missing or malformed, so a bad .env is fixed in one pass.
func Load(getenv func(string) string) (Config, error) {
	var cfg Config
	var problems []error

	environment, err := required(getenv, "APP_ENV")
	cfg.Environment = Environment(environment)
	if err == nil && cfg.Environment != Production && cfg.Environment != Development {
		err = errors.New("APP_ENV must be production or development")
	}
	problems = append(problems, err)
	production := cfg.Environment != Development

	// A setting that development may leave out: production requires it, and in development an
	// absent one turns its feature off.
	optional := func(name string) (string, error) {
		value := strings.TrimSpace(getenv(name))
		if value == "" && production {
			return "", fmt.Errorf("%s is required in production", name)
		}
		return value, nil
	}

	listenAddr, err := required(getenv, "LISTEN_ADDR")
	if err == nil {
		_, _, err = net.SplitHostPort(listenAddr)
		if err != nil {
			err = fmt.Errorf("LISTEN_ADDR must be host:port: %w", err)
		}
	}
	problems = append(problems, err)
	cfg.ListenAddr = listenAddr

	databasePath, err := required(getenv, "DATABASE_PATH")
	problems = append(problems, err)
	cfg.DatabasePath = databasePath

	publicURL, err := required(getenv, "PUBLIC_URL")
	if err == nil {
		cfg.PublicURL, err = parsePublicURL(publicURL)
	}
	if err == nil && production && cfg.PublicURL.Scheme != "https" {
		err = errors.New("PUBLIC_URL must start with https:// in production")
	}
	problems = append(problems, err)

	logLevel, err := required(getenv, "LOG_LEVEL")
	if err == nil {
		err = cfg.LogLevel.UnmarshalText([]byte(logLevel))
		if err != nil {
			err = fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error: %w", err)
		}
	}
	problems = append(problems, err)

	resendAPIKey, err := required(getenv, "RESEND_API_KEY")
	if err == nil && !strings.HasPrefix(resendAPIKey, "re_") {
		err = errors.New("RESEND_API_KEY must be a Resend API key, which starts with re_")
	}
	problems = append(problems, err)
	cfg.ResendAPIKey = resendAPIKey

	emailFrom, err := required(getenv, "EMAIL_FROM")
	if err == nil {
		_, err = mail.ParseAddress(emailFrom)
		if err != nil {
			err = fmt.Errorf("EMAIL_FROM must be an address such as Super Sparrow <sparrow@example.com>: %w", err)
		}
	}
	problems = append(problems, err)
	cfg.EmailFrom = emailFrom

	codeKey, err := required(getenv, "SIGN_IN_CODE_KEY")
	if err == nil && len(codeKey) < minCodeKeyLength {
		err = fmt.Errorf("SIGN_IN_CODE_KEY must be at least %d characters, such as the output of openssl rand -hex 32", minCodeKeyLength)
	}
	problems = append(problems, err)
	cfg.SignInCodeKey = []byte(codeKey)

	trustedProxies, err := optional("TRUSTED_PROXIES")
	if err == nil && trustedProxies != "" {
		cfg.TrustedProxies, err = parseTrustedProxies(trustedProxies)
	}
	problems = append(problems, err)

	webDir, err := optional("WEB_DIR")
	if err == nil && webDir != "" {
		cfg.WebDir, err = parseWebDir(webDir)
	}
	problems = append(problems, err)

	webhookSecret, err := optional("GITHUB_WEBHOOK_SECRET")
	if err == nil && webhookSecret != "" && len(webhookSecret) < minWebhookSecretLength {
		err = fmt.Errorf("GITHUB_WEBHOOK_SECRET must be at least %d characters", minWebhookSecretLength)
	}
	problems = append(problems, err)
	cfg.GitHubWebhookSecret = webhookSecret

	// Backups run whenever any of their settings is present, and then need all of them, so a
	// half-configured backup never passes as switched off.
	if production || slices.ContainsFunc(backupSettings, func(name string) bool { return strings.TrimSpace(getenv(name)) != "" }) {
		cfg.Backups, err = loadBackups(getenv)
		problems = append(problems, err)
	}

	if err := errors.Join(problems...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}

	return cfg, nil
}

// loadBackups reads the settings of the backup jobs, all of which they need.
func loadBackups(getenv func(string) string) (*Backups, error) {
	var backups Backups
	var problems []error

	key, err := LoadBackupKey(getenv)
	problems = append(problems, err)
	backups.EncryptionKey = key

	dailyAt, err := required(getenv, "BACKUP_DAILY_TIME")
	if err == nil {
		var at time.Time
		at, err = time.Parse("15:04", dailyAt)
		if err != nil {
			err = errors.New("BACKUP_DAILY_TIME must be a UTC time such as 03:30")
		}
		backups.DailyAt = time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute
	}
	problems = append(problems, err)

	keepDays, err := required(getenv, "BACKUP_KEEP_DAYS")
	if err == nil {
		backups.KeepDays, err = strconv.Atoi(keepDays)
		if err != nil || backups.KeepDays < 1 || backups.KeepDays > 3650 {
			err = errors.New("BACKUP_KEEP_DAYS must be a whole number of days from 1 to 3650")
		}
	}
	problems = append(problems, err)

	backups.R2, err = LoadR2(getenv)
	problems = append(problems, err)

	if err := errors.Join(problems...); err != nil {
		return nil, err
	}
	return &backups, nil
}

// LoadBackupKey reads the key backups are encrypted with, which the backup jobs and the restore
// command share.
func LoadBackupKey(getenv func(string) string) (string, error) {
	key, err := required(getenv, "BACKUP_ENCRYPTION_KEY")
	if err == nil && len(key) < minBackupKeyLength {
		err = fmt.Errorf("BACKUP_ENCRYPTION_KEY must be at least %d characters, such as the output of openssl rand -hex 32", minBackupKeyLength)
	}
	return key, err
}

// LoadR2 reads the R2 bucket settings, which the backup jobs and the restore command share.
func LoadR2(getenv func(string) string) (R2, error) {
	var r2 R2
	var problems []error
	var err error

	r2.AccountID, err = required(getenv, "R2_ACCOUNT_ID")
	if err == nil && !r2AccountPattern.MatchString(r2.AccountID) {
		err = errors.New("R2_ACCOUNT_ID must be the 32-character Cloudflare account ID")
	}
	problems = append(problems, err)

	r2.AccessKeyID, err = required(getenv, "R2_ACCESS_KEY_ID")
	problems = append(problems, err)
	r2.SecretAccessKey, err = required(getenv, "R2_SECRET_ACCESS_KEY")
	problems = append(problems, err)
	r2.Bucket, err = required(getenv, "R2_BUCKET")
	problems = append(problems, err)

	return r2, errors.Join(problems...)
}

// parseWebDir checks that the folder holds a built frontend: the _shell.html page that TanStack
// Start's SPA build writes.
func parseWebDir(value string) (string, error) {
	dir, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("WEB_DIR is not a usable path: %w", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "_shell.html")); err != nil || info.IsDir() {
		return "", fmt.Errorf("WEB_DIR must be the built frontend's folder, web/dist/client, with its _shell.html; %s has none (run make build-web)", dir)
	}
	return dir, nil
}

func required(getenv func(string) string, name string) (string, error) {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return value, nil
}

func parsePublicURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("PUBLIC_URL is not a URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("PUBLIC_URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return nil, errors.New("PUBLIC_URL must include a host")
	}

	return parsed, nil
}

// parseTrustedProxies reads a comma-separated list of IP addresses and CIDR ranges.
func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if prefix, err := netip.ParsePrefix(part); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES must be a comma-separated list of IP addresses and CIDR ranges, not %q", part)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return prefixes, nil
}
