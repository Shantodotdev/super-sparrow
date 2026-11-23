package github

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// system is the caller of changes that no person made, such as a webhook delivery.
var system = store.Caller{ActorID: store.SystemActorID}

const backgroundSyncTimeout = 2 * time.Minute

func (s *Service) installation(ctx context.Context, deliveryID string, body []byte) (string, error) {
	var payload struct {
		Action       string              `json:"action"`
		Installation installationPayload `json:"installation"`
		Repositories []repositoryPayload `json:"repositories"`
	}
	if err := decode("installation", body, &payload); err != nil {
		return "", err
	}

	change := store.GitHubInstallationChange{
		Installation: store.GitHubInstallation{
			ID: payload.Installation.ID, AccountLogin: payload.Installation.Account.login(),
			Suspended: payload.Installation.SuspendedAt != nil,
		},
	}
	switch payload.Action {
	case "created":
		change.Complete = true
	case "new_permissions_accepted", "unsuspend":
	case "suspend":
		change.Installation.Suspended = true
	case "deleted":
		change.Deleted = true
	default:
		return "ignored", nil
	}
	if payload.Action == "unsuspend" {
		change.Installation.Suspended = false
	}
	for _, repository := range payload.Repositories {
		change.Repositories = append(change.Repositories, repository.storeRepository(payload.Installation.ID))
	}

	if err := s.store.ApplyGitHubInstallation(ctx, system, deliveryID, change); err != nil {
		return "", err
	}
	if payload.Action == "created" {
		s.syncLinkedRepositories(ctx)
	}
	return "applied", nil
}

func (s *Service) installationRepositories(ctx context.Context, deliveryID string, body []byte) (string, error) {
	var payload struct {
		Action       string              `json:"action"`
		Installation installationPayload `json:"installation"`
		Added        []repositoryPayload `json:"repositories_added"`
		Removed      []repositoryPayload `json:"repositories_removed"`
	}
	if err := decode("installation_repositories", body, &payload); err != nil {
		return "", err
	}
	if payload.Action != "added" && payload.Action != "removed" {
		return "ignored", nil
	}

	change := store.GitHubInstallationChange{
		Installation: store.GitHubInstallation{
			ID: payload.Installation.ID, AccountLogin: payload.Installation.Account.login(),
			Suspended: payload.Installation.SuspendedAt != nil,
		},
	}
	for _, repository := range payload.Added {
		change.Repositories = append(change.Repositories, repository.storeRepository(payload.Installation.ID))
	}
	for _, repository := range payload.Removed {
		change.Removed = append(change.Removed, repository.ID)
	}

	if err := s.store.ApplyGitHubInstallation(ctx, system, deliveryID, change); err != nil {
		return "", err
	}
	return "applied", nil
}

// syncLinkedRepositories loads the pull requests of every installed repository that belongs to
// a project, in the background.
func (s *Service) syncLinkedRepositories(ctx context.Context) {
	if s.app == nil {
		return
	}

	background := context.WithoutCancel(ctx)
	s.jobs.Go(func() {
		ctx, cancel := context.WithTimeout(background, backgroundSyncTimeout)
		defer cancel()

		repositories, err := s.store.ListGitHubRepositories(ctx)
		if err != nil {
			s.logger.ErrorContext(ctx, "list GitHub repositories to sync", "error", err)
			return
		}
		for _, repository := range repositories {
			if repository.ProjectKey == "" || !repository.Installed {
				continue
			}
			if _, err := s.SyncRepository(ctx, system, repository.ID); err != nil {
				s.logger.ErrorContext(ctx, "sync GitHub repository", "repository", repository.FullName, "error", err)
			}
		}
	})
}

func (s *Service) pullRequest(ctx context.Context, deliveryID string, body []byte) (string, error) {
	var payload struct {
		Action            string              `json:"action"`
		PullRequest       pullPayload         `json:"pull_request"`
		Repository        repositoryPayload   `json:"repository"`
		Installation      installationPayload `json:"installation"`
		Sender            *user               `json:"sender"`
		RequestedReviewer *user               `json:"requested_reviewer"`
	}
	if err := decode("pull_request", body, &payload); err != nil {
		return "", err
	}

	input := payload.PullRequest.input(payload.Repository.storeRepository(payload.Installation.ID))
	action := store.GitHubAction{Name: payload.Action, Sender: payload.Sender.login(), Reviewer: payload.RequestedReviewer.login()}
	change, err := s.store.ApplyGitHubPullRequest(ctx, deliveryID, input, action)
	if err != nil {
		return "", err
	}
	s.notify(ctx, change)

	// A repeated delivery changes nothing, so it has no new commit to read checks for. The
	// delivery is already recorded, so a failed read is logged instead of failing it: GitHub's
	// redelivery would be ignored.
	switch payload.Action {
	case "opened", "reopened", "synchronize", "ready_for_review":
		if s.app != nil && change.PullRequest.ID != 0 && input.HeadSHA != "" {
			if err := s.refreshChecks(ctx, input.Repository.ID, input.HeadSHA); err != nil {
				s.logger.ErrorContext(ctx, "refresh pull request checks", "repository", input.Repository.FullName, "number", input.Number, "error", err)
			}
		}
	}
	return "applied", nil
}

func (s *Service) pullRequestReview(ctx context.Context, deliveryID string, body []byte) (string, error) {
	var payload struct {
		Action       string              `json:"action"`
		Review       reviewPayload       `json:"review"`
		PullRequest  pullPayload         `json:"pull_request"`
		Repository   repositoryPayload   `json:"repository"`
		Installation installationPayload `json:"installation"`
	}
	if err := decode("pull_request_review", body, &payload); err != nil {
		return "", err
	}
	switch payload.Action {
	case "submitted", "edited", "dismissed":
	default:
		return "ignored", nil
	}

	review, ok := payload.Review.input()
	if !ok {
		return "ignored", nil
	}
	input := payload.PullRequest.input(payload.Repository.storeRepository(payload.Installation.ID))
	action := store.GitHubAction{Name: payload.Action, Sender: review.ReviewerLogin}
	change, err := s.store.ApplyGitHubReview(ctx, deliveryID, input, review, action)
	if err != nil {
		return "", err
	}
	s.notify(ctx, change)
	return "applied", nil
}

func (s *Service) reviewComment(ctx context.Context, deliveryID string, body []byte) (string, error) {
	var payload struct {
		Action      string         `json:"action"`
		Comment     commentPayload `json:"comment"`
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
		Repository repositoryPayload `json:"repository"`
	}
	if err := decode("pull_request_review_comment", body, &payload); err != nil {
		return "", err
	}
	if payload.Action != "created" {
		return "ignored", nil
	}
	return s.recordComment(ctx, deliveryID, payload.Repository.ID, payload.PullRequest.Number, payload.Comment)
}

func (s *Service) issueComment(ctx context.Context, deliveryID string, body []byte) (string, error) {
	var payload struct {
		Action string `json:"action"`
		Issue  struct {
			Number      int            `json:"number"`
			PullRequest map[string]any `json:"pull_request"`
		} `json:"issue"`
		Comment    commentPayload    `json:"comment"`
		Repository repositoryPayload `json:"repository"`
	}
	if err := decode("issue_comment", body, &payload); err != nil {
		return "", err
	}
	if payload.Action != "created" || payload.Issue.PullRequest == nil {
		return "ignored", nil
	}
	return s.recordComment(ctx, deliveryID, payload.Repository.ID, payload.Issue.Number, payload.Comment)
}

// recordComment stores a comment on a pull request Sparrow knows, and ignores one on any other.
func (s *Service) recordComment(ctx context.Context, deliveryID string, repositoryID int64, number int, comment commentPayload) (string, error) {
	err := s.store.RecordGitHubComment(ctx, deliveryID, repositoryID, number, comment.User.login(), comment.HTMLURL, comment.Body)
	if errors.Is(err, store.ErrNotFound) {
		return "ignored", nil
	}
	if err != nil {
		return "", err
	}
	return "applied", nil
}

// checksChanged refreshes the checks of the commit a check or status delivery is about. Without
// the GitHub App there is nothing to read them with, so the delivery is ignored.
func (s *Service) checksChanged(ctx context.Context, event string, body []byte) (string, error) {
	var payload struct {
		Repository repositoryPayload `json:"repository"`
		SHA        string            `json:"sha"`
		CheckSuite struct {
			HeadSHA string `json:"head_sha"`
		} `json:"check_suite"`
		CheckRun struct {
			HeadSHA string `json:"head_sha"`
		} `json:"check_run"`
	}
	if err := decode(event, body, &payload); err != nil {
		return "", err
	}

	sha := strings.TrimSpace(payload.SHA)
	switch event {
	case "check_suite":
		sha = payload.CheckSuite.HeadSHA
	case "check_run":
		sha = payload.CheckRun.HeadSHA
	}
	if s.app == nil || sha == "" || payload.Repository.ID == 0 {
		return "ignored", nil
	}

	err := s.refreshChecks(ctx, payload.Repository.ID, sha)
	if errors.Is(err, store.ErrNotFound) {
		return "ignored", nil
	}
	if err != nil {
		return "", err
	}
	return "applied", nil
}
