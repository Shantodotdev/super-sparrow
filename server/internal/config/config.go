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
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
)

const minWebhookSecretLength = 20

var r2AccountPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Config holds every setting the server needs. Each one is required; there are no defaults.
type Config struct {
	ListenAddr   string
	DatabasePath string
	PublicURL    *url.URL
	LogLevel     slog.Level
	// ResendAPIKey authenticates the server to Resend, which sends sign-in codes and invitations.
	ResendAPIKey string
	// EmailFrom is the sender of every email, such as "Super Sparrow <sparrow@example.com>".
	EmailFrom string
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header names the client's
	// address. It is empty when the server is reached directly.
	TrustedProxies []netip.Prefix
	// WebDir is the folder of the built frontend the server serves, or "" when it serves none,
	// as in development, where the web dev server serves it.
	WebDir string
	// GitHubWebhookSecret checks GitHub webhook signatures. It is "" when the webhook is off.
	GitHubWebhookSecret string
	// Backups configures the backup jobs. It is nil when backups are disabled.
	Backups *Backups
}

// Backups holds the settings of the hourly sync and the daily backup.
type Backups struct {
	// Recipient is the age public key backups are encrypted to. Its private key is never on the
	// server.
	Recipient *age.X25519Recipient
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

	listenAddr, err := required(getenv, "SPARROW_LISTEN_ADDR")
	if err == nil {
		_, _, err = net.SplitHostPort(listenAddr)
		if err != nil {
			err = fmt.Errorf("SPARROW_LISTEN_ADDR must be host:port: %w", err)
		}
	}
	problems = append(problems, err)
	cfg.ListenAddr = listenAddr

	databasePath, err := required(getenv, "SPARROW_DATABASE_PATH")
	problems = append(problems, err)
	cfg.DatabasePath = databasePath

	publicURL, err := required(getenv, "SPARROW_PUBLIC_URL")
	if err == nil {
		cfg.PublicURL, err = parsePublicURL(publicURL)
	}
	problems = append(problems, err)

	logLevel, err := required(getenv, "SPARROW_LOG_LEVEL")
	if err == nil {
		err = cfg.LogLevel.UnmarshalText([]byte(logLevel))
		if err != nil {
			err = fmt.Errorf("SPARROW_LOG_LEVEL must be debug, info, warn, or error: %w", err)
		}
	}
	problems = append(problems, err)

	resendAPIKey, err := required(getenv, "SPARROW_RESEND_API_KEY")
	if err == nil && !strings.HasPrefix(resendAPIKey, "re_") {
		err = errors.New("SPARROW_RESEND_API_KEY must be a Resend API key, which starts with re_")
	}
	problems = append(problems, err)
	cfg.ResendAPIKey = resendAPIKey

	emailFrom, err := required(getenv, "SPARROW_EMAIL_FROM")
	if err == nil {
		_, err = mail.ParseAddress(emailFrom)
		if err != nil {
			err = fmt.Errorf("SPARROW_EMAIL_FROM must be an address such as Super Sparrow <sparrow@example.com>: %w", err)
		}
	}
	problems = append(problems, err)
	cfg.EmailFrom = emailFrom

	trustedProxies, err := required(getenv, "SPARROW_TRUSTED_PROXIES")
	if err == nil {
		cfg.TrustedProxies, err = parseTrustedProxies(trustedProxies)
	}
	problems = append(problems, err)

	webDir, err := required(getenv, "SPARROW_WEB_DIR")
	if err == nil && webDir != "none" {
		cfg.WebDir, err = parseWebDir(webDir)
	}
	problems = append(problems, err)

	webhookSecret, err := required(getenv, "SPARROW_GITHUB_WEBHOOK_SECRET")
	if err == nil && webhookSecret != "none" {
		if len(webhookSecret) < minWebhookSecretLength {
			err = fmt.Errorf("SPARROW_GITHUB_WEBHOOK_SECRET must be none or at least %d characters", minWebhookSecretLength)
		}
		cfg.GitHubWebhookSecret = webhookSecret
	}
	problems = append(problems, err)

	backups, err := required(getenv, "SPARROW_BACKUPS")
	switch {
	case err != nil:
		problems = append(problems, err)
	case backups == "enabled":
		cfg.Backups, err = loadBackups(getenv)
		problems = append(problems, err)
	case backups != "disabled":
		problems = append(problems, errors.New("SPARROW_BACKUPS must be enabled or disabled"))
	}

	if err := errors.Join(problems...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}

	return cfg, nil
}

// loadBackups reads the settings the backup jobs need once they are enabled.
func loadBackups(getenv func(string) string) (*Backups, error) {
	var backups Backups
	var problems []error

	recipient, err := required(getenv, "SPARROW_BACKUP_AGE_RECIPIENT")
	if err == nil {
		backups.Recipient, err = age.ParseX25519Recipient(recipient)
		if err != nil {
			err = fmt.Errorf("SPARROW_BACKUP_AGE_RECIPIENT must be an age public key, which starts with age1: %w", err)
		}
	}
	problems = append(problems, err)

	dailyAt, err := required(getenv, "SPARROW_BACKUP_DAILY_TIME")
	if err == nil {
		var at time.Time
		at, err = time.Parse("15:04", dailyAt)
		if err != nil {
			err = errors.New("SPARROW_BACKUP_DAILY_TIME must be a UTC time such as 03:30")
		}
		backups.DailyAt = time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute
	}
	problems = append(problems, err)

	keepDays, err := required(getenv, "SPARROW_BACKUP_KEEP_DAYS")
	if err == nil {
		backups.KeepDays, err = strconv.Atoi(keepDays)
		if err != nil || backups.KeepDays < 1 || backups.KeepDays > 3650 {
			err = errors.New("SPARROW_BACKUP_KEEP_DAYS must be a whole number of days from 1 to 3650")
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

// LoadR2 reads the R2 bucket settings, which the backup jobs and the restore command share.
func LoadR2(getenv func(string) string) (R2, error) {
	var r2 R2
	var problems []error
	var err error

	r2.AccountID, err = required(getenv, "SPARROW_R2_ACCOUNT_ID")
	if err == nil && !r2AccountPattern.MatchString(r2.AccountID) {
		err = errors.New("SPARROW_R2_ACCOUNT_ID must be the 32-character Cloudflare account ID")
	}
	problems = append(problems, err)

	r2.AccessKeyID, err = required(getenv, "SPARROW_R2_ACCESS_KEY_ID")
	problems = append(problems, err)
	r2.SecretAccessKey, err = required(getenv, "SPARROW_R2_SECRET_ACCESS_KEY")
	problems = append(problems, err)
	r2.Bucket, err = required(getenv, "SPARROW_R2_BUCKET")
	problems = append(problems, err)

	return r2, errors.Join(problems...)
}

// parseWebDir checks that the folder holds a built frontend: the _shell.html page that TanStack
// Start's SPA build writes.
func parseWebDir(value string) (string, error) {
	dir, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("SPARROW_WEB_DIR is not a usable path: %w", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "_shell.html")); err != nil || info.IsDir() {
		return "", fmt.Errorf("SPARROW_WEB_DIR must be none or the built frontend's folder, web/dist/client, with its _shell.html; %s has none (run make build-web)", dir)
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
		return nil, fmt.Errorf("SPARROW_PUBLIC_URL is not a URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("SPARROW_PUBLIC_URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return nil, errors.New("SPARROW_PUBLIC_URL must include a host")
	}

	return parsed, nil
}

// parseTrustedProxies reads "none" or a comma-separated list of IP addresses and CIDR ranges.
func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	if value == "none" {
		return nil, nil
	}

	var prefixes []netip.Prefix
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if prefix, err := netip.ParsePrefix(part); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("SPARROW_TRUSTED_PROXIES must be none or a comma-separated list of IP addresses and CIDR ranges, not %q", part)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return prefixes, nil
}
