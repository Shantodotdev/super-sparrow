package httpserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// modelHeader is where an agent names the model it runs.
const modelHeader = "Sparrow-Model"

// access is who may call a route.
type access int

const (
	public access = iota
	signedIn
	humanOnly
	ownerOnly
	agentOnly
)

// identity is who made a request, taken from their session cookie or agent token.
type identity struct {
	actor     store.Actor
	sessionID string
	tokenID   string
	// reportedModel is the model an agent names in the Sparrow-Model header. It is recorded on
	// events as reported; the server cannot verify it.
	reportedModel string
}

func (id identity) caller() store.Caller {
	return store.Caller{ActorID: id.actor.ID, ReportedModel: id.reportedModel}
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
	case level == agentOnly && id.tokenID == "":
		return forbiddenRequest("send an agent token as a bearer token; create one in settings")
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

		checked, err := s.auth.AuthenticateToken(r.Context(), strings.TrimSpace(token))
		if err != nil {
			return identity{}, err
		}
		if ok, retry := s.perToken.Allow(checked.Token.ID); !ok {
			return identity{}, &auth.RateLimitError{RetryAfter: retry}
		}
		return identity{actor: checked.Actor, tokenID: checked.Token.ID, reportedModel: strings.TrimSpace(r.Header.Get(modelHeader))}, nil
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
	return identity{actor: checked.Actor, sessionID: checked.Session.ID}, nil
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
