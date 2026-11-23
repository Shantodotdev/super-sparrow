// Package github connects Super Sparrow to a GitHub App: it turns webhook deliveries into
// stored pull requests and reads pull requests, reviews, and checks back through the GitHub API.
package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// APIURL is the base address of GitHub's REST API.
const APIURL = "https://api.github.com"

const (
	// maxResponseBytes bounds one API response.
	maxResponseBytes = 8 << 20
	// tokenRefreshMargin is how long before expiry a cached installation token stops being used.
	tokenRefreshMargin = time.Minute
	jwtBackdate        = time.Minute
	jwtLifetime        = 9 * time.Minute
)

// App is a client of the GitHub API that acts as one GitHub App. It signs a short-lived JSON Web
// Token with the App's private key, trades it for installation access tokens, and caches those
// until shortly before they expire. It is safe for concurrent use.
type App struct {
	id         int64
	installURL string
	key        *rsa.PrivateKey
	baseURL    string
	client     *http.Client
	now        func() time.Time

	mu     sync.Mutex
	tokens map[int64]installationToken
}

type installationToken struct {
	value   string
	expires time.Time
}

// APIError is a response from GitHub that was not a success.
type APIError struct {
	Method  string
	Path    string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub %s %s answered %d: %s", e.Method, e.Path, e.Status, e.Message)
}

// NewApp returns a client for the App with the given ID, slug, and private key. baseURL is APIURL
// outside tests; client carries the request timeout.
func NewApp(id int64, slug string, key *rsa.PrivateKey, baseURL string, client *http.Client, now func() time.Time) (*App, error) {
	switch {
	case id <= 0:
		return nil, errors.New("create GitHub App client: the App ID must be positive")
	case slug == "":
		return nil, errors.New("create GitHub App client: the App slug is required")
	case key == nil:
		return nil, errors.New("create GitHub App client: the private key is required")
	case baseURL == "":
		return nil, errors.New("create GitHub App client: the API URL is required")
	case client == nil:
		return nil, errors.New("create GitHub App client: the HTTP client is required")
	case now == nil:
		return nil, errors.New("create GitHub App client: the clock is required")
	}
	return &App{id: id, installURL: "https://github.com/apps/" + slug + "/installations/new", key: key, baseURL: strings.TrimRight(baseURL, "/"), client: client, now: now, tokens: map[int64]installationToken{}}, nil
}

// appJWT signs the token that authenticates as the App itself.
func (a *App) appJWT() (string, error) {
	now := a.now()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("encode JWT header: %w", err)
	}
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-jwtBackdate).Unix(),
		"exp": now.Add(jwtLifetime).Unix(),
		"iss": strconv.FormatInt(a.id, 10),
	})
	if err != nil {
		return "", fmt.Errorf("encode JWT claims: %w", err)
	}

	encoding := base64.RawURLEncoding
	signingInput := encoding.EncodeToString(header) + "." + encoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return signingInput + "." + encoding.EncodeToString(signature), nil
}

// installationToken returns an access token for the installation, from the cache while it is
// still good. The lock is held while fetching, so concurrent callers share one request.
func (a *App) installationToken(ctx context.Context, installationID int64) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if cached, ok := a.tokens[installationID]; ok && a.now().Before(cached.expires.Add(-tokenRefreshMargin)) {
		return cached.value, nil
	}

	jwt, err := a.appJWT()
	if err != nil {
		return "", err
	}
	var response struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := a.do(ctx, http.MethodPost, path, jwt, &response); err != nil {
		return "", fmt.Errorf("get access token for installation %d: %w", installationID, err)
	}
	if response.Token == "" || response.ExpiresAt.IsZero() {
		return "", fmt.Errorf("get access token for installation %d: GitHub sent no token or expiry", installationID)
	}

	a.tokens[installationID] = installationToken{value: response.Token, expires: response.ExpiresAt}
	return response.Token, nil
}

func (a *App) forgetToken(installationID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, installationID)
}

// getAsApp sends a GET authenticated as the App and decodes the JSON answer into dst.
func (a *App) getAsApp(ctx context.Context, path string, dst any) error {
	jwt, err := a.appJWT()
	if err != nil {
		return err
	}
	return a.do(ctx, http.MethodGet, path, jwt, dst)
}

// getAsInstallation sends a GET authenticated as an installation. A 401 means the cached token
// was revoked early, so it fetches a fresh one and tries once more.
func (a *App) getAsInstallation(ctx context.Context, installationID int64, path string, dst any) error {
	for attempt := 0; ; attempt++ {
		token, err := a.installationToken(ctx, installationID)
		if err != nil {
			return err
		}
		err = a.do(ctx, http.MethodGet, path, token, dst)

		var apiErr *APIError
		if attempt == 0 && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			a.forgetToken(installationID)
			continue
		}
		return err
	}
}

// do sends one request with the bearer credential and decodes a JSON answer into dst. Errors
// carry the method, path, and GitHub's message, and never the credential.
func (a *App) do(ctx context.Context, method, path, credential string, dst any) error {
	request, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build GitHub request %s %s: %w", method, path, err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "super-sparrow")
	request.Header.Set("Authorization", "Bearer "+credential)

	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("send GitHub request %s %s: %w", method, path, redactURLError(err))
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read GitHub response %s %s: %w", method, path, err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("read GitHub response %s %s: more than %d bytes", method, path, maxResponseBytes)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		var failure struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &failure) != nil || failure.Message == "" {
			failure.Message = http.StatusText(response.StatusCode)
		}
		return &APIError{Method: method, Path: path, Status: response.StatusCode, Message: failure.Message}
	}

	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(dst); err != nil {
		return fmt.Errorf("decode GitHub response %s %s: %w", method, path, err)
	}
	return nil
}

// redactURLError drops the request URL from a transport error, keeping the cause.
func redactURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
