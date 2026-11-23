package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func pkcs1(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func pkcs8(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestLoadGitHubAppIsOffWithoutSettings(t *testing.T) {
	app, err := LoadGitHubApp(lookup(map[string]string{}))
	if app != nil || err != nil {
		t.Fatalf("LoadGitHubApp = %v, %v", app, err)
	}
}

func TestLoadGitHubAppAcceptsKeyFormats(t *testing.T) {
	key := testKey(t)
	for name, value := range map[string]string{
		"PKCS1":         pkcs1(key),
		"PKCS8":         pkcs8(t, key),
		"literal \\n":   strings.ReplaceAll(strings.TrimSpace(pkcs1(key)), "\n", `\n`),
		"padded PKCS8":  "\n" + pkcs8(t, key) + "\n",
		"escaped PKCS8": strings.ReplaceAll(strings.TrimSpace(pkcs8(t, key)), "\n", `\n`),
	} {
		t.Run(name, func(t *testing.T) {
			env := map[string]string{"GITHUB_APP_ID": "12345", "GITHUB_APP_SLUG": "my-sparrow", "GITHUB_APP_PRIVATE_KEY": value}
			app, err := LoadGitHubApp(lookup(env))
			if err != nil {
				t.Fatalf("LoadGitHubApp: %v", err)
			}
			if app.ID != 12345 || app.Slug != "my-sparrow" || !app.PrivateKey.Equal(key) {
				t.Fatalf("app %+v", app)
			}
		})
	}
}

func TestLoadGitHubAppRejectsPartialAndMalformedSettings(t *testing.T) {
	key := pkcs1(testKey(t))
	good := map[string]string{"GITHUB_APP_ID": "12345", "GITHUB_APP_SLUG": "my-sparrow", "GITHUB_APP_PRIVATE_KEY": key}

	for _, missing := range githubAppSettings {
		env := map[string]string{}
		for name, value := range good {
			if name != missing {
				env[name] = value
			}
		}
		if _, err := LoadGitHubApp(lookup(env)); err == nil || !strings.Contains(err.Error(), missing+" is required") {
			t.Errorf("without %s: %v", missing, err)
		}
	}

	bad := map[string]string{
		"GITHUB_APP_ID":          "0",
		"GITHUB_APP_SLUG":        "My Sparrow",
		"GITHUB_APP_PRIVATE_KEY": "-----BEGIN RSA PRIVATE KEY-----\nsecret-material\n-----END RSA PRIVATE KEY-----",
	}
	for name, value := range bad {
		env := map[string]string{}
		for k, v := range good {
			env[k] = v
		}
		env[name] = value
		_, err := LoadGitHubApp(lookup(env))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s=%q: %v", name, value, err)
		} else if strings.Contains(err.Error(), "secret-material") {
			t.Errorf("error echoes the key: %v", err)
		}
	}
}

func TestGitHubAppNeedsTheWebhookSecretInDevelopment(t *testing.T) {
	env := developmentEnv(t)
	env["GITHUB_APP_ID"] = "12345"
	env["GITHUB_APP_SLUG"] = "my-sparrow"
	env["GITHUB_APP_PRIVATE_KEY"] = pkcs1(testKey(t))

	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "GITHUB_WEBHOOK_SECRET is required") {
		t.Fatalf("Load without a webhook secret: %v", err)
	}

	env["GITHUB_WEBHOOK_SECRET"] = "a-long-enough-webhook-secret"
	cfg, err := Load(lookup(env))
	if err != nil || cfg.GitHubApp == nil || cfg.GitHubApp.ID != 12345 {
		t.Fatalf("Load = %+v, %v", cfg.GitHubApp, err)
	}
}

func TestProductionWorksWithoutTheGitHubApp(t *testing.T) {
	cfg, err := Load(lookup(productionEnv(t)))
	if err != nil || cfg.GitHubApp != nil {
		t.Fatalf("Load = %+v, %v", cfg.GitHubApp, err)
	}
}
