package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var githubAppSettings = []string{"GITHUB_APP_ID", "GITHUB_APP_SLUG", "GITHUB_APP_PRIVATE_KEY"}

var githubSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// GitHubApp holds the credentials of the GitHub App that reports pull requests and lets the
// server read them back.
type GitHubApp struct {
	ID         int64
	Slug       string
	PrivateKey *rsa.PrivateKey
}

// LoadGitHubApp leaves the GitHub App off when none of its settings is supplied, and otherwise
// requires all of them. The private key may be written with literal \n escapes so that it fits on
// one line of an environment file. Errors never repeat the key.
func LoadGitHubApp(getenv func(string) string) (*GitHubApp, error) {
	if !slices.ContainsFunc(githubAppSettings, func(name string) bool { return strings.TrimSpace(getenv(name)) != "" }) {
		return nil, nil
	}

	var app GitHubApp
	var problems []error

	id, err := required(getenv, "GITHUB_APP_ID")
	if err == nil {
		app.ID, err = strconv.ParseInt(id, 10, 64)
		if err != nil || app.ID <= 0 || strconv.FormatInt(app.ID, 10) != id {
			err = errors.New("GITHUB_APP_ID must be a positive whole number")
		}
	}
	problems = append(problems, err)

	app.Slug, err = required(getenv, "GITHUB_APP_SLUG")
	if err == nil && (len(app.Slug) > 100 || !githubSlugPattern.MatchString(app.Slug)) {
		err = errors.New("GITHUB_APP_SLUG must be the app's lowercase URL name, such as my-sparrow")
	}
	problems = append(problems, err)

	key, err := required(getenv, "GITHUB_APP_PRIVATE_KEY")
	if err == nil {
		app.PrivateKey, err = parsePrivateKey(key)
	}
	problems = append(problems, err)

	if err := errors.Join(problems...); err != nil {
		return nil, err
	}
	return &app, nil
}

// parsePrivateKey reads an RSA private key in PEM form, as PKCS#1 or PKCS#8.
func parsePrivateKey(value string) (*rsa.PrivateKey, error) {
	invalid := errors.New("GITHUB_APP_PRIVATE_KEY must be the app's RSA private key in PEM form")

	block, _ := pem.Decode([]byte(strings.ReplaceAll(value, `\n`, "\n")))
	if block == nil {
		return nil, invalid
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, invalid
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, invalid
	}
	return key, nil
}
