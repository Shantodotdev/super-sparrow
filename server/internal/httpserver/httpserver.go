// Package httpserver routes HTTP requests to the API.
package httpserver

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/ratelimit"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	tokenRequestsPerMinute = 300
	maxBodyBytes           = 1 << 20
)

// Options holds what the HTTP handler needs.
type Options struct {
	Logger *slog.Logger
	DB     *sql.DB
	Store  *store.Store
	Auth   *auth.Service
	// PublicURL is the address people open. Its scheme decides whether the session cookie is
	// Secure, and its origin is the only one allowed to send changes with that cookie.
	PublicURL *url.URL
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header is believed.
	TrustedProxies []netip.Prefix
	// Now supplies the current time for per-token rate limits; pass time.Now outside tests.
	Now func() time.Time
}

type server struct {
	logger         *slog.Logger
	db             *sql.DB
	store          *store.Store
	auth           *auth.Service
	origin         string
	secureCookie   bool
	cookieName     string
	trustedProxies []netip.Prefix
	perToken       *ratelimit.Limiter
	mux            *http.ServeMux
}

// New returns the server's HTTP handler, with request logging and panic recovery around every
// route.
func New(opts Options) http.Handler {
	s := &server{
		logger:         opts.Logger,
		db:             opts.DB,
		store:          opts.Store,
		auth:           opts.Auth,
		origin:         opts.PublicURL.Scheme + "://" + opts.PublicURL.Host,
		secureCookie:   opts.PublicURL.Scheme == "https",
		cookieName:     "sparrow_session",
		trustedProxies: opts.TrustedProxies,
		perToken:       ratelimit.New(tokenRequestsPerMinute, time.Minute, opts.Now),
		mux:            http.NewServeMux(),
	}
	// Over HTTPS the __Host- prefix makes browsers refuse the cookie from any other host or path.
	if s.secureCookie {
		s.cookieName = "__Host-sparrow_session"
	}

	s.mux.HandleFunc("GET /api/health", s.health)

	s.route("POST /api/auth/code", public, s.requestCode)
	s.route("POST /api/auth/verify", public, s.verifyCode)
	s.route("POST /api/auth/sign-out", humanOnly, s.signOut)
	s.route("GET /api/me", signedIn, s.me)

	s.route("GET /api/sessions", humanOnly, s.listSessions)
	s.route("DELETE /api/sessions", humanOnly, s.revokeAllSessions)
	s.route("DELETE /api/sessions/{id}", humanOnly, s.revokeSession)

	s.route("GET /api/people", signedIn, s.listPeople)
	s.route("POST /api/people", ownerOnly, s.invitePerson)
	s.route("POST /api/people/{id}/disable", ownerOnly, s.disablePerson)

	s.route("GET /api/tokens", humanOnly, s.listTokens)
	s.route("POST /api/tokens", humanOnly, s.createToken)
	s.route("POST /api/tokens/{id}/renew", humanOnly, s.renewToken)
	s.route("DELETE /api/tokens/{id}", humanOnly, s.revokeToken)

	return recoverPanics(opts.Logger, logRequests(opts.Logger, s.mux))
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
