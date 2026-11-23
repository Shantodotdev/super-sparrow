package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const notifyTimeout = 20 * time.Second

// Notifier is told about pull request changes worth announcing, such as in Discord. It decides
// for itself what to do with each one.
type Notifier interface {
	PullRequestChanged(ctx context.Context, change store.GitHubPullRequestChange)
}

// Service applies GitHub webhook deliveries to the store and talks to GitHub as the App.
type Service struct {
	store    *store.Store
	app      *App
	notifier Notifier
	logger   *slog.Logger
	jobs     sync.WaitGroup
}

// NewService returns a service. app is nil when the GitHub App is not configured, in which case
// deliveries are still applied but nothing is read back from GitHub. notifier may be nil; it must
// not be a nil pointer wrapped in the interface.
func NewService(st *store.Store, app *App, notifier Notifier, logger *slog.Logger) (*Service, error) {
	switch {
	case st == nil:
		return nil, errors.New("create GitHub service: the store is required")
	case logger == nil:
		return nil, errors.New("create GitHub service: the logger is required")
	}
	return &Service{store: st, app: app, notifier: notifier, logger: logger}, nil
}

// Wait blocks until the background work the service started, such as notifications, has finished.
func (s *Service) Wait() { s.jobs.Wait() }

// Configured reports whether the GitHub App's credentials are set, so the service can read
// from GitHub.
func (s *Service) Configured() bool { return s.app != nil }

// InstallURL is the page where the App is installed on an account, or "" without an App.
func (s *Service) InstallURL() string {
	if s.app == nil {
		return ""
	}
	return s.app.installURL
}

// notifies lists the changes the notifier hears about.
func notifies(kind string) bool {
	switch kind {
	case "github.pull_request.opened", "github.pull_request.ready_for_review", "github.pull_request.merged":
		return true
	}
	return false
}

// notify tells the notifier about a change in the background, so a slow announcement never holds
// up the delivery's answer.
func (s *Service) notify(ctx context.Context, change store.GitHubPullRequestChange) {
	if s.notifier == nil || !notifies(change.Kind) {
		return
	}

	background := context.WithoutCancel(ctx)
	s.jobs.Go(func() {
		ctx, cancel := context.WithTimeout(background, notifyTimeout)
		defer cancel()
		s.notifier.PullRequestChanged(ctx, change)
	})
}

// invalidDelivery is the refusal for a body that is not the event it claims to be.
func invalidDelivery(event string) error {
	return &store.ValidationError{Field: "body", Message: fmt.Sprintf("must be a %s event", event)}
}

// decode reads a delivery's JSON body.
func decode(event string, body []byte, dst any) error {
	if err := json.Unmarshal(body, dst); err != nil {
		return invalidDelivery(event)
	}
	return nil
}

// HandleDelivery applies one webhook delivery whose signature the caller has already checked. It
// returns "pong" for a ping, "ignored" for an event or action Sparrow has no use for, and
// "applied" otherwise. Deliveries are recorded by ID, so a repeated one changes nothing.
func (s *Service) HandleDelivery(ctx context.Context, event, deliveryID string, body []byte) (string, error) {
	switch event {
	case "ping":
		return "pong", nil
	case "installation":
		return s.installation(ctx, deliveryID, body)
	case "installation_repositories":
		return s.installationRepositories(ctx, deliveryID, body)
	case "pull_request":
		return s.pullRequest(ctx, deliveryID, body)
	case "pull_request_review":
		return s.pullRequestReview(ctx, deliveryID, body)
	case "pull_request_review_comment":
		return s.reviewComment(ctx, deliveryID, body)
	case "issue_comment":
		return s.issueComment(ctx, deliveryID, body)
	case "check_suite", "check_run", "status":
		return s.checksChanged(ctx, event, body)
	default:
		return "ignored", nil
	}
}
