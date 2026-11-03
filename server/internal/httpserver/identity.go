package httpserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	// modelHeader is where an agent names the model it runs.
	modelHeader = "Sparrow-Model"
	// agentHeader is where a signed-in CLI says it runs inside an agent, such as claude-code, so
	// the request acts as that person's agent instead of the person.
	agentHeader = "Sparrow-Agent"
)

// access is who may call a route.
type access int

const (
	public access = iota
	signedIn
	humanOnly
	ownerOnly
	// toolCaller is an agent token or a signed-in CLI, the callers of the MCP endpoint.
	toolCaller
)

// identity is who made a request, taken from their session cookie, signed-in CLI, or agent token.
type identity struct {
	actor store.Actor
	// client is how the request was authenticated.
	client    store.Client
	sessionID string
	tokenID   string
	// reportedModel is the model an agent names in the Sparrow-Model header. It is recorded on
	// events as reported; the server cannot verify it.
	reportedModel string
}

func (id identity) caller() store.Caller {
	return store.Caller{ActorID: id.actor.ID, ReportedModel: id.reportedModel, Client: id.client}
}

type identityKey struct{}

// identityFrom returns the identity the route's middleware resolved. It exists on every route
// that is not public.
func identityFrom(ctx context.Context) identity {
	id, _ := ctx.Value(identityKey{}).(identity)
	return id
}

// route registers handler for pattern behind the checks that level requires.
func (s *server) route(pattern string, level access, handler apiHandler) {
	s.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.authorize(w, r, level, handler); err != nil {
			s.writeError(w, r, err)
		}
	}))
}

func (s *server) authorize(w http.ResponseWriter, r *http.Request, level access, handler apiHandler) error {
	// SameSite=Lax already keeps the cookie off cross-site posts; checking Origin also stops
	// another site from signing someone in or out.
	if !safeMethod(r.Method) && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin {
		return forbiddenRequest("requests that change data must come from " + s.origin)
	}
	if level == public {
		return handler(w, r)
	}

	id, err := s.identify(w, r)
	if err != nil {
		return err
	}

	switch {
	case level == humanOnly && id.actor.Kind != store.ActorHuman:
		return forbiddenRequest("only a person can do this, not an agent")
	case level == ownerOnly && !id.actor.IsOwner():
		return forbiddenRequest("only an owner can do this")
	// Owner actions add people, grant permissions, and turn on secrets, which would let a CLI
	// session reach what only a browser may read, so they need a browser.
	case level == ownerOnly && id.client != store.ClientBrowser:
		return forbiddenRequest("owners do this in the browser, not from the CLI")
	case level == toolCaller && id.client != store.ClientAgentToken && id.client != store.ClientCLI:
		return forbiddenRequest("sign in with the sparrow CLI, or send an agent token as a bearer token")
	}

	return handler(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
}

// identify resolves the caller from an Authorization bearer token or, without one, from the
// session cookie. A request that sends a token is judged by the token alone.
func (s *server) identify(w http.ResponseWriter, r *http.Request) (identity, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			return identity{}, store.ErrUnauthenticated
		}
		token = strings.TrimSpace(token)

		if strings.HasPrefix(token, auth.CLISessionPrefix) {
			return s.identifyCLI(r, token)
		}

		checked, err := s.auth.AuthenticateToken(r.Context(), token)
		if err != nil {
			return identity{}, err
		}
		if ok, retry := s.perToken.Allow(checked.Token.ID); !ok {
			return identity{}, &auth.RateLimitError{RetryAfter: retry}
		}
		return identity{
			actor: checked.Actor, client: store.ClientAgentToken, tokenID: checked.Token.ID,
			reportedModel: strings.TrimSpace(r.Header.Get(modelHeader)),
		}, nil
	}

	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return identity{}, store.ErrUnauthenticated
	}

	checked, err := s.auth.AuthenticateSession(r.Context(), cookie.Value)
	if err != nil {
		return identity{}, err
	}
	if checked.Renewed {
		s.setSessionCookie(w, cookie.Value, checked.Session.ExpiresAt)
	}
	return identity{actor: checked.Actor, client: store.ClientBrowser, sessionID: checked.Session.ID}, nil
}

// identifyCLI resolves a signed-in CLI. Without an agent header the request is the person's own.
// With one, it acts as that person's agent, which the server cannot verify: the CLI says it runs
// inside that agent. Signed-in CLIs share the per-token rate limit, keyed by session.
func (s *server) identifyCLI(r *http.Request, token string) (identity, error) {
	checked, err := s.auth.AuthenticateCLISession(r.Context(), token)
	if err != nil {
		return identity{}, err
	}
	if ok, retry := s.perToken.Allow(checked.Session.ID); !ok {
		return identity{}, &auth.RateLimitError{RetryAfter: retry}
	}

	id := identity{actor: checked.Actor, client: store.ClientCLI, sessionID: checked.Session.ID}
	agentName := strings.TrimSpace(r.Header.Get(agentHeader))
	if agentName == "" {
		return id, nil
	}
	if !store.KnownAgentName(agentName) {
		return identity{}, &store.ValidationError{Field: agentHeader, Message: "must be claude-code or codex"}
	}

	agent, err := s.store.AgentForOwner(r.Context(), checked.Actor.ID, agentName)
	if err != nil {
		return identity{}, err
	}
	id.actor = agent
	id.reportedModel = strings.TrimSpace(r.Header.Get(modelHeader))
	return id, nil
}

func (s *server) setSessionCookie(w http.ResponseWriter, secret string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    secret,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		Secure:   s.secureCookie,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName, Value: "", Path: "/", MaxAge: -1,
		Secure: s.secureCookie, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// forbiddenRequest is a refusal decided by the HTTP layer, reported like a store refusal.
func forbiddenRequest(reason string) error {
	return store.Refuse(store.ErrForbidden, "%s", reason)
}
