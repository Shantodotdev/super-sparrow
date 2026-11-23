package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	// DeviceCodePrefix starts the secret a CLI polls its sign-in request with.
	DeviceCodePrefix = "ssd_"
	// userCodeAlphabet has no vowels, so codes never spell words, and no look-alike characters.
	userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ"
	// userCodeLength is the number of letters in a user code, shown as two groups of four.
	userCodeLength = 8
)

// StartedCLILogin is a new CLI sign-in request. DeviceCode stays with the CLI, which polls with
// it; UserCode is what the person approves in the desktop app. Neither is stored.
type StartedCLILogin struct {
	DeviceCode string
	UserCode   string
	ExpiresAt  time.Time
}

// CLIClaim is the answer to a CLI polling its request. SignIn is set only when Status is
// store.CLILoginClaimed, the one time the CLI receives its session.
type CLIClaim struct {
	Status store.CLILoginStatus
	SignIn SignIn
}

// StartCLILogin creates a sign-in request for a CLI on deviceLabel. clientIP is shown to the
// person who approves it and limits how often one address may ask.
func (s *Service) StartCLILogin(ctx context.Context, deviceLabel, clientIP string) (StartedCLILogin, error) {
	if ok, retry := s.cliLogins.Allow(clientIP); !ok {
		return StartedCLILogin{}, &RateLimitError{RetryAfter: retry}
	}

	secret, err := newSecret()
	if err != nil {
		return StartedCLILogin{}, err
	}
	deviceCode := DeviceCodePrefix + secret
	userCode, err := newUserCode()
	if err != nil {
		return StartedCLILogin{}, err
	}

	login, err := s.store.CreateCLILogin(ctx, store.NewCLILogin{
		DeviceCodeHash: Hash(deviceCode), UserCodeHash: s.hashUserCode(userCode),
		DeviceLabel: deviceLabel, RequestedFrom: clientIP,
	})
	if err != nil {
		return StartedCLILogin{}, err
	}

	return StartedCLILogin{DeviceCode: deviceCode, UserCode: formatUserCode(userCode), ExpiresAt: login.ExpiresAt}, nil
}

// FindCLILogin returns the pending request for userCode, for the person about to approve it.
func (s *Service) FindCLILogin(ctx context.Context, userCode string) (store.CLILogin, error) {
	normalized, err := normalizeUserCode(userCode)
	if err != nil {
		return store.CLILogin{}, err
	}
	return s.store.GetCLILogin(ctx, s.hashUserCode(normalized))
}

// DecideCLILogin approves or denies the pending request for userCode as the caller.
func (s *Service) DecideCLILogin(ctx context.Context, by store.Caller, userCode string, approve bool) (store.CLILogin, error) {
	normalized, err := normalizeUserCode(userCode)
	if err != nil {
		return store.CLILogin{}, err
	}
	return s.store.DecideCLILogin(ctx, by, s.hashUserCode(normalized), approve)
}

// ClaimCLILogin answers a CLI polling with deviceCode. The first poll after approval creates the
// CLI session and returns its secret; any malformed or unknown device code reads as expired.
func (s *Service) ClaimCLILogin(ctx context.Context, deviceCode string) (CLIClaim, error) {
	if !strings.HasPrefix(deviceCode, DeviceCodePrefix) || len(deviceCode) != len(DeviceCodePrefix)+secretLength {
		return CLIClaim{Status: store.CLILoginExpired}, nil
	}

	secret, err := newSecret()
	if err != nil {
		return CLIClaim{}, err
	}
	secret = CLISessionPrefix + secret

	status, actor, session, err := s.store.ClaimCLILogin(ctx, Hash(deviceCode), Hash(secret))
	if err != nil {
		return CLIClaim{}, err
	}
	if status != store.CLILoginApproved {
		return CLIClaim{Status: status}, nil
	}
	return CLIClaim{Status: store.CLILoginClaimed, SignIn: SignIn{Actor: actor, Session: session, Secret: secret}}, nil
}

// hashUserCode hashes a normalized user code under the server's code key, so the short codes
// cannot be guessed offline from a copy of the database.
func (s *Service) hashUserCode(code string) []byte {
	mac := hmac.New(sha256.New, s.codeKey)
	mac.Write([]byte("cli-login\x00" + code))
	return mac.Sum(nil)
}

func newUserCode() (string, error) {
	var code strings.Builder
	limit := big.NewInt(int64(len(userCodeAlphabet)))
	for range userCodeLength {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate user code: %w", err)
		}
		code.WriteByte(userCodeAlphabet[n.Int64()])
	}
	return code.String(), nil
}

// formatUserCode shows a code as two groups of four, such as BCDF-GHJK.
func formatUserCode(code string) string {
	return code[:userCodeLength/2] + "-" + code[userCodeLength/2:]
}

// normalizeUserCode accepts a code in any case, with or without its dash and spaces.
func normalizeUserCode(code string) (string, error) {
	normalized := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	if len(normalized) != userCodeLength || strings.Trim(normalized, userCodeAlphabet) != "" {
		return "", &store.ValidationError{Field: "user_code", Message: "must be the 8-letter code the CLI shows, such as BCDF-GHJK"}
	}
	return normalized, nil
}
