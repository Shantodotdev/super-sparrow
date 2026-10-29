// Package auth signs people in with emailed codes and authenticates sessions and agent tokens.
//
// It makes every secret, emails codes and invitations, and hands the store only hashes, so
// sign-in codes, session IDs, and token secrets are never stored, logged, or returned after they
// are created. Sign-in codes are hashed with a server key, since six digits are quick to guess
// from a plain hash.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/ratelimit"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	// TokenPrefix starts every agent token, so secret scanners can recognize them.
	TokenPrefix = "ssp_"

	codeRequestsPerEmail = 5
	codeRequestsPerIP    = 20
	codeChecksPerIP      = 10
	codeRequestWindow    = time.Hour
	codeCheckWindow      = time.Hour
	emailSendTimeout     = 30 * time.Second
	secretBytes          = 32
)

// secretLength is the length of an encoded session ID or token secret, without the token prefix.
var secretLength = base64.RawURLEncoding.EncodedLen(secretBytes)

// RateLimitError means a request went over a rate limit. RetryAfter says when to try again.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("too many requests; try again in %s", e.RetryAfter.Round(time.Second))
}

// Service signs people in and checks credentials. It is safe for concurrent use.
type Service struct {
	store     *store.Store
	mailer    email.Sender
	publicURL *url.URL
	codeKey   []byte
	logger    *slog.Logger
	perEmail  *ratelimit.Limiter
	perIP     *ratelimit.Limiter
	checks    *ratelimit.Limiter
	sending   sync.WaitGroup
}

// New returns a Service. publicURL is the address people open, used in emails. codeKey is the
// secret sign-in codes are hashed with; it is kept out of the
// database so a copy of the database does not reveal codes. now supplies the current time for
// rate limits; pass time.Now outside tests.
func New(st *store.Store, mailer email.Sender, publicURL *url.URL, codeKey []byte, logger *slog.Logger, now func() time.Time) *Service {
	return &Service{
		store:     st,
		mailer:    mailer,
		publicURL: publicURL,
		codeKey:   codeKey,
		logger:    logger,
		perEmail:  ratelimit.New(codeRequestsPerEmail, codeRequestWindow, now),
		perIP:     ratelimit.New(codeRequestsPerIP, codeRequestWindow, now),
		checks:    ratelimit.New(codeChecksPerIP, codeCheckWindow, now),
	}
}

// RequestCode emails a new sign-in code to address when it belongs to an active teammate. It
// returns the same result whether or not it does, and sends the email in the background so the
// response time does not tell them apart either. It fails with a RateLimitError when address or
// clientIP asked too often.
func (s *Service) RequestCode(ctx context.Context, address, clientIP string) error {
	normalized := strings.ToLower(strings.TrimSpace(address))
	if normalized == "" {
		return &store.ValidationError{Field: "email", Message: "is required"}
	}

	if ok, retry := s.perIP.Allow(clientIP); !ok {
		return &RateLimitError{RetryAfter: retry}
	}
	if ok, retry := s.perEmail.Allow(normalized); !ok {
		return &RateLimitError{RetryAfter: retry}
	}

	code, err := newCode()
	if err != nil {
		return err
	}

	issued, err := s.store.IssueSignInCode(ctx, normalized, s.hashCode(normalized, code))
	if err != nil || !issued {
		return err
	}

	message, err := email.SignInCode(normalized, code, int(store.SignInCodeLifetime/time.Minute))
	if err != nil {
		return err
	}
	s.sendInBackground(message, "sign-in code")
	return nil
}

// SignIn is the result of a correct sign-in code: the person and their new session. Secret is
// the session ID for the cookie; it is not stored and cannot be read again.
type SignIn struct {
	Actor   store.Actor
	Session store.Session
	Secret  string
}

// VerifyCode checks code for address and, when it is right, creates a session for the device.
// Every wrong, expired, or used code fails with store.ErrInvalidCode. Five wrong guesses cancel
// a person's codes, so each client address may check only a few codes an hour; otherwise anyone
// who knows a teammate's email could keep them from signing in.
func (s *Service) VerifyCode(ctx context.Context, address, code, deviceLabel, clientIP string) (SignIn, error) {
	normalized := strings.ToLower(strings.TrimSpace(address))
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return SignIn{}, &store.ValidationError{Field: "code", Message: "must be the 6-digit code from the email"}
	}
	if ok, retry := s.checks.Allow(clientIP); !ok {
		return SignIn{}, &RateLimitError{RetryAfter: retry}
	}

	secret, err := newSecret()
	if err != nil {
		return SignIn{}, err
	}

	actor, session, err := s.store.VerifySignInCode(ctx, normalized, s.hashCode(normalized, code),
		store.NewSession{Hash: Hash(secret), DeviceLabel: deviceLabel})
	if err != nil {
		return SignIn{}, err
	}

	return SignIn{Actor: actor, Session: session, Secret: secret}, nil
}

// AuthenticateSession checks a session ID from a cookie.
func (s *Service) AuthenticateSession(ctx context.Context, secret string) (store.AuthenticatedSession, error) {
	if len(secret) != secretLength {
		return store.AuthenticatedSession{}, store.ErrUnauthenticated
	}
	return s.store.AuthenticateSession(ctx, Hash(secret))
}

// AuthenticateToken checks an agent token from an Authorization header.
func (s *Service) AuthenticateToken(ctx context.Context, token string) (store.AuthenticatedToken, error) {
	if !strings.HasPrefix(token, TokenPrefix) || len(token) != len(TokenPrefix)+secretLength {
		return store.AuthenticatedToken{}, store.ErrUnauthenticated
	}
	return s.store.AuthenticateToken(ctx, Hash(token))
}

// NewToken is a token just created. Secret is shown to its person once and never again.
type NewToken struct {
	Token  store.AgentToken
	Secret string
}

// CreateToken creates an agent token for the caller's agent for agentName.
func (s *Service) CreateToken(ctx context.Context, by store.Caller, agentName, label string) (NewToken, error) {
	secret, err := newSecret()
	if err != nil {
		return NewToken{}, err
	}
	token := TokenPrefix + secret

	created, err := s.store.CreateAgentToken(ctx, by, store.NewAgentToken{
		AgentName: agentName, Label: label, Hash: Hash(token), DisplayHint: TokenPrefix + "…" + token[len(token)-4:],
	})
	if err != nil {
		return NewToken{}, err
	}

	return NewToken{Token: created, Secret: token}, nil
}

// ErrInvitationNotSent means the person was added but their invitation email failed. They can
// still sign in with their email.
var ErrInvitationNotSent = errors.New("invitation email was not sent")

// Invite adds a person to the team and emails them an invitation. When the email fails it
// returns the added person together with an error that wraps ErrInvitationNotSent.
func (s *Service) Invite(ctx context.Context, by store.Caller, input store.NewHuman) (store.Actor, error) {
	actor, err := s.store.CreateHuman(ctx, by, input)
	if err != nil {
		return store.Actor{}, err
	}

	inviter, err := s.store.GetActor(ctx, by.ActorID)
	if err != nil {
		return actor, fmt.Errorf("%w: load inviter: %w", ErrInvitationNotSent, err)
	}

	message, err := email.Invitation(actor.Email, inviter.DisplayName, s.publicURL.String())
	if err != nil {
		return actor, fmt.Errorf("%w: %w", ErrInvitationNotSent, err)
	}

	err = s.mailer.Send(ctx, message)
	if err != nil {
		return actor, fmt.Errorf("%w: %w", ErrInvitationNotSent, err)
	}

	return actor, nil
}

// Wait blocks until every email sending in the background has finished.
func (s *Service) Wait() { s.sending.Wait() }

// sendInBackground sends message without holding up the request. A failure is logged without
// the message, which may hold a code.
func (s *Service) sendInBackground(message email.Message, kind string) {
	s.sending.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		defer cancel()

		if err := s.mailer.Send(ctx, message); err != nil {
			s.logger.Error("send email failed", "kind", kind, "error", err)
		}
	})
}

// Hash returns the SHA-256 hash under which a session ID or token is stored.
func Hash(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// hashCode hashes a sign-in code together with its email under the server's code key, so equal
// codes for different people have different hashes and the hashes cannot be guessed offline.
func (s *Service) hashCode(address, code string) []byte {
	mac := hmac.New(sha256.New, s.codeKey)
	mac.Write([]byte(address + "\x00" + code))
	return mac.Sum(nil)
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate sign-in code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func newSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
