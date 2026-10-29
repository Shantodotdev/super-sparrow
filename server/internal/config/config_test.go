package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validEnv() map[string]string {
	return map[string]string{
		"SPARROW_LISTEN_ADDR":           "127.0.0.1:8090",
		"SPARROW_DATABASE_PATH":         "data/sparrow.db",
		"SPARROW_PUBLIC_URL":            "https://sparrow.example.com",
		"SPARROW_LOG_LEVEL":             "info",
		"SPARROW_RESEND_API_KEY":        "re_test_key",
		"SPARROW_EMAIL_FROM":            "Super Sparrow <sparrow@example.com>",
		"SPARROW_TRUSTED_PROXIES":       "127.0.0.1, 10.0.0.0/8",
		"SPARROW_WEB_DIR":               "none",
		"SPARROW_GITHUB_WEBHOOK_SECRET": "none",
		"SPARROW_BACKUPS":               "disabled",
	}
}

func lookup(env map[string]string) func(string) string {
	return func(name string) string { return env[name] }
}

func TestLoadReadsEverySetting(t *testing.T) {
	cfg, err := Load(lookup(validEnv()))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:8090" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DatabasePath != "data/sparrow.db" {
		t.Errorf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.PublicURL.String() != "https://sparrow.example.com" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v", cfg.LogLevel)
	}
	if cfg.ResendAPIKey != "re_test_key" {
		t.Errorf("ResendAPIKey = %q", cfg.ResendAPIKey)
	}
	if cfg.EmailFrom != "Super Sparrow <sparrow@example.com>" {
		t.Errorf("EmailFrom = %q", cfg.EmailFrom)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0].String() != "127.0.0.1/32" ||
		cfg.TrustedProxies[1].String() != "10.0.0.0/8" {
		t.Errorf("TrustedProxies = %v", cfg.TrustedProxies)
	}
}

func TestLoadAcceptsNoTrustedProxies(t *testing.T) {
	env := validEnv()
	env["SPARROW_TRUSTED_PROXIES"] = "none"

	cfg, err := Load(lookup(env))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies = %v, want none", cfg.TrustedProxies)
	}
}

func TestLoadNamesEveryMissingSetting(t *testing.T) {
	_, err := Load(lookup(map[string]string{}))
	if err == nil {
		t.Fatal("Load accepted an empty environment")
	}

	for name := range validEnv() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}
}

func TestLoadRejectsMalformedSettings(t *testing.T) {
	cases := map[string]string{
		"SPARROW_LISTEN_ADDR":     "8090",
		"SPARROW_PUBLIC_URL":      "sparrow.example.com",
		"SPARROW_LOG_LEVEL":       "loud",
		"SPARROW_RESEND_API_KEY":  "sk_live_123",
		"SPARROW_EMAIL_FROM":      "not an address",
		"SPARROW_TRUSTED_PROXIES": "10.0.0.0/8, proxy.local",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			env := validEnv()
			env[name] = value

			_, err := Load(lookup(env))
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("Load did not reject %s=%q: %v", name, value, err)
			}
		})
	}
}

func TestLoadReadsTheOptionalParts(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "_shell.html"), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := validEnv()
	env["SPARROW_WEB_DIR"] = webDir
	env["SPARROW_GITHUB_WEBHOOK_SECRET"] = "a-long-enough-webhook-secret"
	env["SPARROW_BACKUPS"] = "enabled"
	env["SPARROW_BACKUP_AGE_RECIPIENT"] = "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p"
	env["SPARROW_BACKUP_DAILY_TIME"] = "03:30"
	env["SPARROW_BACKUP_KEEP_DAYS"] = "30"
	env["SPARROW_R2_ACCOUNT_ID"] = "0123456789abcdef0123456789abcdef"
	env["SPARROW_R2_ACCESS_KEY_ID"] = "key-id"
	env["SPARROW_R2_SECRET_ACCESS_KEY"] = "secret"
	env["SPARROW_R2_BUCKET"] = "sparrow-backups"

	cfg, err := Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WebDir != webDir || cfg.GitHubWebhookSecret != "a-long-enough-webhook-secret" {
		t.Errorf("web dir %q, webhook secret %q", cfg.WebDir, cfg.GitHubWebhookSecret)
	}
	if cfg.Backups == nil || cfg.Backups.DailyAt != 3*time.Hour+30*time.Minute || cfg.Backups.KeepDays != 30 || cfg.Backups.R2.Bucket != "sparrow-backups" {
		t.Errorf("backups %+v", cfg.Backups)
	}
}

func TestLoadNamesBadOptionalParts(t *testing.T) {
	env := validEnv()
	env["SPARROW_WEB_DIR"] = t.TempDir()
	env["SPARROW_GITHUB_WEBHOOK_SECRET"] = "short"
	env["SPARROW_BACKUPS"] = "enabled"
	env["SPARROW_BACKUP_DAILY_TIME"] = "25:00"

	_, err := Load(lookup(env))
	if err == nil {
		t.Fatal("Load accepted bad settings")
	}
	for _, name := range []string{
		"SPARROW_WEB_DIR", "SPARROW_GITHUB_WEBHOOK_SECRET", "SPARROW_BACKUP_AGE_RECIPIENT is required",
		"SPARROW_BACKUP_DAILY_TIME", "SPARROW_BACKUP_KEEP_DAYS is required", "SPARROW_R2_BUCKET is required",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not mention %s:\n%v", name, err)
		}
	}

	env["SPARROW_BACKUPS"] = "sometimes"
	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "SPARROW_BACKUPS must be enabled or disabled") {
		t.Errorf("SPARROW_BACKUPS=sometimes: %v", err)
	}
}
