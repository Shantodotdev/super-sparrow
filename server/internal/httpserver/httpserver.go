// Package httpserver routes HTTP requests to the API.
package httpserver

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/mcp"
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
	// Stop is closed when the server starts shutting down, which ends open event streams so
	// shutdown does not wait for them.
	Stop <-chan struct{}
	// BackupsEnabled says whether the backup jobs run, which settings show.
	BackupsEnabled bool
	// Version is the server's version, which MCP clients are told.
	Version string
	// WebDir is the folder of the built frontend to serve, or "" to serve none.
	WebDir string
	// GitHubWebhookSecret checks GitHub webhook signatures; "" turns the webhook off.
	GitHubWebhookSecret string
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
	stop           <-chan struct{}
	backupsEnabled bool
	mcp            *mcp.Server
	webhookSecret  string
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
		stop:           opts.Stop,
		backupsEnabled: opts.BackupsEnabled,
		mcp:            mcp.New(opts.Store, opts.PublicURL, opts.Logger, opts.Version),
		webhookSecret:  opts.GitHubWebhookSecret,
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

	s.route("GET /api/projects", signedIn, s.listProjects)
	s.route("POST /api/projects", humanOnly, s.createProject)
	s.route("GET /api/projects/{key}", signedIn, s.getProject)
	s.route("PATCH /api/projects/{key}", humanOnly, s.renameProject)
	s.route("DELETE /api/projects/{key}", ownerOnly, s.deleteProject)
	s.route("GET /api/projects/{key}/milestones", signedIn, s.listMilestones)
	s.route("POST /api/projects/{key}/milestones", humanOnly, s.createMilestone)
	s.route("PATCH /api/milestones/{id}", humanOnly, s.updateMilestone)
	s.route("GET /api/projects/{key}/labels", signedIn, s.listLabels)
	s.route("POST /api/projects/{key}/labels", signedIn, s.createLabel)

	s.route("GET /api/projects/{key}/issues", signedIn, s.listIssues)
	s.route("POST /api/projects/{key}/issues", signedIn, s.createIssue)
	s.route("GET /api/issues/for-branch", signedIn, s.issueForBranch)
	s.route("GET /api/issues/{key}", signedIn, s.getIssue)
	s.route("PATCH /api/issues/{key}", signedIn, s.updateIssue)
	s.route("PATCH /api/issues/{key}/checklist", signedIn, s.updateChecklist)
	s.route("GET /api/issues/{key}/events", signedIn, s.listIssueEvents)
	s.route("GET /api/issues/{key}/comments", signedIn, s.listIssueComments)
	s.route("POST /api/issues/{key}/comments", signedIn, s.addIssueComment)
	s.route("POST /api/issues/{key}/fix-summary", signedIn, s.postFixSummary)

	s.route("GET /api/issues/{key}/reviews", signedIn, s.listRounds)
	s.route("POST /api/issues/{key}/reviews", signedIn, s.startRound)
	s.route("GET /api/issues/{key}/reviews/{round}", signedIn, s.getRound)
	s.route("POST /api/issues/{key}/reviews/{round}/finish", signedIn, s.finishRound)
	s.route("POST /api/issues/{key}/reviews/{round}/findings", signedIn, s.addFinding)
	s.route("GET /api/issues/{key}/findings", signedIn, s.listIssueFindings)
	s.route("GET /api/issues/{key}/reviews/{round}/findings/{finding}", signedIn, s.getFinding)
	s.route("POST /api/issues/{key}/reviews/{round}/findings/{finding}/status", signedIn, s.changeFindingStatus)
	s.route("PUT /api/issues/{key}/reviews/{round}/findings/{finding}/duplicate", signedIn, s.markDuplicate)
	s.route("GET /api/issues/{key}/reviews/{round}/findings/{finding}/comments", signedIn, s.listFindingComments)
	s.route("POST /api/issues/{key}/reviews/{round}/findings/{finding}/comments", signedIn, s.addFindingComment)
	s.route("PATCH /api/comments/{id}", signedIn, s.editComment)

	s.route("GET /api/events", signedIn, s.listEvents)
	s.route("GET /api/events/stream", signedIn, s.streamEvents)
	s.route("GET /api/backups", humanOnly, s.backupStatus)

	// The MCP endpoint takes agent tokens only. It offers no server-to-client stream and keeps no
	// sessions, so GET and DELETE are refused as the transport allows.
	s.route("POST /mcp", agentOnly, s.serveMCP)
	s.mux.HandleFunc("GET /mcp", methodNotAllowed)
	s.mux.HandleFunc("DELETE /mcp", methodNotAllowed)

	if s.webhookSecret != "" {
		s.route("POST /api/webhooks/github", public, s.githubWebhook)
	}

	// Unknown API paths answer in JSON, so they never fall through to the frontend.
	s.mux.HandleFunc("/api/", s.noSuchEndpoint)
	if opts.WebDir != "" {
		s.mux.Handle("/", newFrontend(opts.WebDir))
	}

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

func (s *server) serveMCP(w http.ResponseWriter, r *http.Request) error {
	s.mcp.ServeHTTP(w, r, identityFrom(r.Context()).caller())
	return nil
}

// noSuchEndpoint answers an API request no route takes: 405 with the allowed methods when the
// path exists for other methods, and 404 otherwise.
func (s *server) noSuchEndpoint(w http.ResponseWriter, r *http.Request) {
	var allowed []string
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, pattern := s.mux.Handler(probe); pattern != "/api/" {
			allowed = append(allowed, method)
		}
	}

	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: r.Method + " is not allowed on " + r.URL.Path + "; use " + strings.Join(allowed, ", ")})
		return
	}
	writeJSON(w, http.StatusNotFound, errorBody{Error: "no such API endpoint: " + r.Method + " " + r.URL.Path})
}

func methodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	w.WriteHeader(http.StatusMethodNotAllowed)
}
