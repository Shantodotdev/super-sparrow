package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// developmentEnv holds only the settings development requires.
func developmentEnv() map[string]string {
	return map[string]string{
		"APP_ENV":          "development",
		"LISTEN_ADDR":      "127.0.0.1:8090",
		"DATABASE_PATH":    "data/sparrow.db",
		"PUBLIC_URL":       "http://127.0.0.1:8090",
		"LOG_LEVEL":        "info",
		"RESEND_API_KEY":   "re_test_key",
		"EMAIL_FROM":       "Super Sparrow <sparrow@example.com>",
		"SIGN_IN_CODE_KEY": "0123456789abcdef0123456789abcdef",
	}
}

// productionEnv holds every setting, as production requires.
func productionEnv(t *testing.T) map[string]string {
	t.Helper()
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "_shell.html"), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := developmentEnv()
	env["APP_ENV"] = "production"
	env["PUBLIC_URL"] = "https://sparrow.example.com"
	env["TRUSTED_PROXIES"] = "127.0.0.1, 10.0.0.0/8"
	env["WEB_DIR"] = webDir
	env["GITHUB_WEBHOOK_SECRET"] = "a-long-enough-webhook-secret"
	env["BACKUP_AGE_RECIPIENT"] = "age1gsdyrs7gwehv0unafknt05d0kkl202qp4uazx4y7y5acp8n7uudq3p92hk"
	env["BACKUP_DAILY_TIME"] = "03:00"
	env["BACKUP_TIME_ZONE"] = "Asia/Dhaka"
	env["BACKUP_KEEP_DAYS"] = "30"
	env["R2_ACCOUNT_ID"] = "0123456789abcdef0123456789abcdef"
	env["R2_ACCESS_KEY_ID"] = "key-id"
	env["R2_SECRET_ACCESS_KEY"] = "secret"
	env["R2_BUCKET"] = "sparrow-backups"
	return env
}

func lookup(env map[string]string) func(string) string {
	return func(name string) string { return env[name] }
}

func TestProductionReadsEverySetting(t *testing.T) {
	env := productionEnv(t)
	cfg, err := Load(lookup(env))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}

	if cfg.Environment != Production || cfg.ListenAddr != "127.0.0.1:8090" || cfg.DatabasePath != "data/sparrow.db" {
		t.Errorf("environment %q, listen %q, database %q", cfg.Environment, cfg.ListenAddr, cfg.DatabasePath)
	}
	if cfg.PublicURL.String() != "https://sparrow.example.com" || cfg.LogLevel != slog.LevelInfo {
		t.Errorf("public URL %q, log level %v", cfg.PublicURL, cfg.LogLevel)
	}
	if cfg.ResendAPIKey != "re_test_key" || cfg.EmailFrom != "Super Sparrow <sparrow@example.com>" {
		t.Errorf("Resend key %q, sender %q", cfg.ResendAPIKey, cfg.EmailFrom)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0].String() != "127.0.0.1/32" ||
		cfg.TrustedProxies[1].String() != "10.0.0.0/8" {
		t.Errorf("TrustedProxies = %v", cfg.TrustedProxies)
	}
	if cfg.WebDir != env["WEB_DIR"] || cfg.GitHubWebhookSecret != "a-long-enough-webhook-secret" {
		t.Errorf("web dir %q, webhook secret %q", cfg.WebDir, cfg.GitHubWebhookSecret)
	}
	if cfg.Backups == nil || cfg.Backups.Recipient.String() != "age1gsdyrs7gwehv0unafknt05d0kkl202qp4uazx4y7y5acp8n7uudq3p92hk" ||
		cfg.Backups.DailyAt != 3*time.Hour || cfg.Backups.TimeZone.String() != "Asia/Dhaka" || cfg.Backups.KeepDays != 30 || cfg.Backups.R2.Bucket != "sparrow-backups" {
		t.Errorf("backups %+v", cfg.Backups)
	}
}

func TestProductionRequiresEverySetting(t *testing.T) {
	for name := range productionEnv(t) {
		t.Run(name, func(t *testing.T) {
			env := productionEnv(t)
			delete(env, name)

			_, err := Load(lookup(env))
			if err == nil || !strings.Contains(err.Error(), name+" is required") {
				t.Fatalf("Load without %s: %v", name, err)
			}
		})
	}
}

func TestProductionRequiresHTTPS(t *testing.T) {
	env := productionEnv(t)
	env["PUBLIC_URL"] = "http://sparrow.example.com"

	_, err := Load(lookup(env))
	if err == nil || !strings.Contains(err.Error(), "PUBLIC_URL must start with https://") {
		t.Fatalf("Load accepted an http public URL in production: %v", err)
	}
}

func TestDevelopmentTurnsOffWhatIsAbsent(t *testing.T) {
	cfg, err := Load(lookup(developmentEnv()))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if cfg.Environment != Development || cfg.WebDir != "" || cfg.GitHubWebhookSecret != "" || len(cfg.TrustedProxies) != 0 || cfg.Backups != nil {
		t.Errorf("development config %+v", cfg)
	}
}

func TestDevelopmentNeedsAllBackupSettingsOrNone(t *testing.T) {
	env := developmentEnv()
	env["R2_BUCKET"] = "sparrow-backups-test"

	_, err := Load(lookup(env))
	if err == nil {
		t.Fatal("Load accepted half of the backup settings")
	}
	for _, name := range []string{"BACKUP_AGE_RECIPIENT", "BACKUP_DAILY_TIME", "BACKUP_TIME_ZONE", "BACKUP_KEEP_DAYS", "R2_ACCOUNT_ID"} {
		if !strings.Contains(err.Error(), name+" is required") {
			t.Errorf("error does not name %s:\n%v", name, err)
		}
	}
}

func TestLoadNamesEveryMissingSetting(t *testing.T) {
	_, err := Load(lookup(map[string]string{}))
	if err == nil {
		t.Fatal("Load accepted an empty environment")
	}

	for name := range developmentEnv() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}
}

func TestLoadRejectsMalformedSettings(t *testing.T) {
	cases := map[string]string{
		"APP_ENV":               "staging",
		"LISTEN_ADDR":           "8090",
		"PUBLIC_URL":            "sparrow.example.com",
		"LOG_LEVEL":             "loud",
		"RESEND_API_KEY":        "sk_live_123",
		"EMAIL_FROM":            "not an address",
		"SIGN_IN_CODE_KEY":      "too-short",
		"TRUSTED_PROXIES":       "10.0.0.0/8, proxy.local",
		"WEB_DIR":               "/no/such/folder",
		"GITHUB_WEBHOOK_SECRET": "short",
		"BACKUP_AGE_RECIPIENT":  "age1notakey",
		"BACKUP_DAILY_TIME":     "25:00",
		"BACKUP_TIME_ZONE":      "Mars/Olympus",
		"BACKUP_KEEP_DAYS":      "0",
		"R2_ACCOUNT_ID":         "not-an-account",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			env := productionEnv(t)
			env[name] = value

			_, err := Load(lookup(env))
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("Load did not reject %s=%q: %v", name, value, err)
			}
		})
	}
}
