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
	// CLISessionPrefix starts the secret of every signed-in CLI, which the CLI sends as a bearer
	// token. Browser session IDs have no prefix, since they only travel in a cookie.
	CLISessionPrefix = "ssc_"
	// ServiceTokenPrefix starts every CI token, which reads one project's staging or production
	// secrets.
	ServiceTokenPrefix = "sst_"

	codeRequestsPerEmail = 5
	codeRequestsPerIP    = 20
	wrongCodesPerIP      = 20
	codeRequestWindow    = time.Hour
	wrongCodeWindow      = time.Hour
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
	// wrongCodes counts wrong sign-in codes per client address. Right codes are not counted, so
	// a team signing in from behind one office address never uses it up.
	wrongCodes *ratelimit.Limiter
	sending    sync.WaitGroup
}

// New returns a Service. publicURL is the address people open, used in emails. codeKey is the
// secret sign-in codes are hashed with; it is kept out of the
// database so a copy of the database does not reveal codes. now supplies the current time for
// rate limits; pass time.Now outside tests.
func New(st *store.Store, mailer email.Sender, publicURL *url.URL, codeKey []byte, logger *slog.Logger, now func() time.Time) *Service {
	return &Service{
		store:      st,
		mailer:     mailer,
		publicURL:  publicURL,
		codeKey:    codeKey,
		logger:     logger,
		perEmail:   ratelimit.New(codeRequestsPerEmail, codeRequestWindow, now),
		perIP:      ratelimit.New(codeRequestsPerIP, codeRequestWindow, now),
		wrongCodes: ratelimit.New(wrongCodesPerIP, wrongCodeWindow, now),
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

	message, err := email.SignInCode(normalized, code, int(store.SignInCodeLifetime/time.Minute), s.publicURL.JoinPath("logo.png").String())
	if err != nil {
		return err
	}
	s.sendInBackground(message, "sign-in code")
	return nil
}

// SignIn is the result of a correct sign-in code: the person and their new session. Secret is
// the session ID for the cookie, or the CLI's bearer token; it is not stored and cannot be read
// again.
type SignIn struct {
	Actor   store.Actor
	Session store.Session
	Secret  string
}

// VerifyCode checks code for address and, when it is right, creates a session for the device.
// Every wrong, expired, or used code fails with store.ErrInvalidCode. Five wrong guesses cancel
// a person's codes, so each client address may enter only a few wrong codes an hour; otherwise
// anyone who knows teammates' emails could keep them all from signing in. Right codes do not
// count, so people signing in from one shared address do not use up each other's tries.
//
// client says whether the session is for a browser cookie or a signed-in CLI. A CLI secret starts
// with CLISessionPrefix.
func (s *Service) VerifyCode(ctx context.Context, address, code, deviceLabel, clientIP string, client store.Client) (SignIn, error) {
	normalized := strings.ToLower(strings.TrimSpace(address))
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return SignIn{}, &store.ValidationError{Field: "code", Message: "must be the 6-digit code from the email"}
	}
	if ok, retry := s.wrongCodes.Check(clientIP); !ok {
		return SignIn{}, &RateLimitError{RetryAfter: retry}
	}

	secret, err := newSecret()
	if err != nil {
		return SignIn{}, err
	}
	if client == store.ClientCLI {
		secret = CLISessionPrefix + secret
	}

	actor, session, err := s.store.VerifySignInCode(ctx, normalized, s.hashCode(normalized, code),
		store.NewSession{Hash: Hash(secret), DeviceLabel: deviceLabel, Client: client})
	if errors.Is(err, store.ErrInvalidCode) {
		s.wrongCodes.Record(clientIP)
	}
	if err != nil {
		return SignIn{}, err
	}

	return SignIn{Actor: actor, Session: session, Secret: secret}, nil
}

// AuthenticateSession checks a session ID from a cookie. A CLI session is refused here, so its
// secret cannot be used as a browser cookie.
func (s *Service) AuthenticateSession(ctx context.Context, secret string) (store.AuthenticatedSession, error) {
	if len(secret) != secretLength {
		return store.AuthenticatedSession{}, store.ErrUnauthenticated
	}
	return s.authenticateSession(ctx, secret, store.ClientBrowser)
}

// AuthenticateCLISession checks a signed-in CLI's bearer token. A browser session is refused here.
func (s *Service) AuthenticateCLISession(ctx context.Context, token string) (store.AuthenticatedSession, error) {
	if !strings.HasPrefix(token, CLISessionPrefix) || len(token) != len(CLISessionPrefix)+secretLength {
		return store.AuthenticatedSession{}, store.ErrUnauthenticated
	}
	return s.authenticateSession(ctx, token, store.ClientCLI)
}

// authenticateSession checks a session secret and that it was made for client.
func (s *Service) authenticateSession(ctx context.Context, secret string, client store.Client) (store.AuthenticatedSession, error) {
	checked, err := s.store.AuthenticateSession(ctx, Hash(secret))
	if err != nil {
		return store.AuthenticatedSession{}, err
	}
	if checked.Session.Client != client {
		return store.AuthenticatedSession{}, store.ErrUnauthenticated
	}
	return checked, nil
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

// NewServiceToken is a CI token just created. Secret is shown to its person once and never again.
type NewServiceToken struct {
	Token  store.ServiceToken
	Secret string
}

// CreateServiceToken creates a CI token that reads one project's staging or production config.
func (s *Service) CreateServiceToken(ctx context.Context, by store.Caller, projectID string, config store.SecretConfig, label string) (NewServiceToken, error) {
	secret, err := newSecret()
	if err != nil {
		return NewServiceToken{}, err
	}
	token := ServiceTokenPrefix + secret

	created, err := s.store.CreateServiceToken(ctx, by, projectID, store.NewServiceToken{
		Config: config, Label: label, Hash: Hash(token), DisplayHint: ServiceTokenPrefix + "…" + token[len(token)-4:],
	})
	if err != nil {
		return NewServiceToken{}, err
	}

	return NewServiceToken{Token: created, Secret: token}, nil
}

// DownloadWithServiceToken checks a CI token and returns the values of the config it reads.
func (s *Service) DownloadWithServiceToken(ctx context.Context, token string) (store.ServiceToken, store.SecretValues, error) {
	if !strings.HasPrefix(token, ServiceTokenPrefix) || len(token) != len(ServiceTokenPrefix)+secretLength {
		return store.ServiceToken{}, store.SecretValues{}, store.ErrUnauthenticated
	}
	return s.store.DownloadWithServiceToken(ctx, Hash(token))
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

	message, err := email.Invitation(actor.Email, inviter.DisplayName, s.publicURL.String(), s.publicURL.JoinPath("logo.png").String())
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
