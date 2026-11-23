package github

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	pageSize        = 100
	syncedPullCount = 50
)

type installationListing struct {
	ID          int64   `json:"id"`
	Account     *user   `json:"account"`
	SuspendedAt *string `json:"suspended_at"`
}

// requireApp refuses work that needs the GitHub App when it is not configured.
func (s *Service) requireApp() error {
	if s.app == nil {
		return store.Refuse(store.ErrNotFound, "the GitHub App is not configured")
	}
	return nil
}

// SyncRepository loads a repository's most recently updated pull requests and their reviews
// from GitHub, stores them, and reads the checks of the open ones. It returns how many pull
// requests it stored. A failed read of checks is logged and does not fail the sync.
func (s *Service) SyncRepository(ctx context.Context, by store.Caller, repositoryID int64) (int, error) {
	if err := s.requireApp(); err != nil {
		return 0, err
	}
	repository, err := s.store.GetGitHubRepository(ctx, repositoryID)
	if err != nil {
		return 0, err
	}
	if repository.InstallationID == 0 {
		return 0, store.Refuse(store.ErrConflict, "the GitHub App is not installed on %s", repository.FullName)
	}

	base := repositoryPath(repository.FullName)
	var listed []pullPayload
	query := "/pulls?state=all&sort=updated&direction=desc&per_page=" + strconv.Itoa(syncedPullCount)
	if err := s.app.getAsInstallation(ctx, repository.InstallationID, base+query, &listed); err != nil {
		return 0, fmt.Errorf("list pull requests of %s: %w", repository.FullName, err)
	}

	pulls := make([]store.GitHubPullRequestInput, 0, len(listed))
	reviews := map[int64][]store.GitHubReviewInput{}
	for _, item := range listed {
		number := strconv.Itoa(item.Number)

		// The list leaves out line counts and merge details, so each pull request is read whole.
		var pull pullPayload
		if err := s.app.getAsInstallation(ctx, repository.InstallationID, base+"/pulls/"+number, &pull); err != nil {
			return 0, fmt.Errorf("read pull request %s#%d: %w", repository.FullName, item.Number, err)
		}
		var submitted []reviewPayload
		if err := s.app.getAsInstallation(ctx, repository.InstallationID, base+"/pulls/"+number+"/reviews?per_page="+strconv.Itoa(pageSize), &submitted); err != nil {
			return 0, fmt.Errorf("list reviews of %s#%d: %w", repository.FullName, item.Number, err)
		}

		pulls = append(pulls, pull.input(repository))
		for _, review := range submitted {
			if input, ok := review.input(); ok {
				reviews[pull.ID] = append(reviews[pull.ID], input)
			}
		}
	}

	count, err := s.store.SyncGitHubPullRequests(ctx, by, repositoryID, pulls, reviews)
	if err != nil {
		return 0, err
	}

	for _, pull := range pulls {
		if pull.State != store.PullRequestOpen || pull.HeadSHA == "" {
			continue
		}
		if err := s.refreshChecks(ctx, repositoryID, pull.HeadSHA); err != nil {
			s.logger.ErrorContext(ctx, "refresh pull request checks", "repository", repository.FullName, "number", pull.Number, "error", err)
		}
	}
	return count, nil
}

// Refresh asks GitHub for every installation of the App and the repositories each can see, and
// stores them. A suspended installation keeps the repositories it had.
func (s *Service) Refresh(ctx context.Context, by store.Caller) error {
	if err := s.requireApp(); err != nil {
		return err
	}

	for page := 1; ; page++ {
		var installations []installationListing
		path := "/app/installations?per_page=" + strconv.Itoa(pageSize) + "&page=" + strconv.Itoa(page)
		if err := s.app.getAsApp(ctx, path, &installations); err != nil {
			return fmt.Errorf("list GitHub installations: %w", err)
		}

		for _, installation := range installations {
			if err := s.refreshInstallation(ctx, by, installation); err != nil {
				return err
			}
		}
		if len(installations) < pageSize {
			return nil
		}
	}
}

// refreshInstallation stores one installation with the repositories GitHub lists for it.
func (s *Service) refreshInstallation(ctx context.Context, by store.Caller, installation installationListing) error {
	change := store.GitHubInstallationChange{
		Installation: store.GitHubInstallation{
			ID: installation.ID, AccountLogin: installation.Account.login(), Suspended: installation.SuspendedAt != nil,
		},
	}

	if !change.Installation.Suspended {
		change.Complete = true
		for page := 1; ; page++ {
			var listing struct {
				Repositories []repositoryPayload `json:"repositories"`
			}
			path := "/installation/repositories?per_page=" + strconv.Itoa(pageSize) + "&page=" + strconv.Itoa(page)
			if err := s.app.getAsInstallation(ctx, installation.ID, path, &listing); err != nil {
				return fmt.Errorf("list repositories of installation %d: %w", installation.ID, err)
			}
			for _, repository := range listing.Repositories {
				change.Repositories = append(change.Repositories, repository.storeRepository(installation.ID))
			}
			if len(listing.Repositories) < pageSize {
				break
			}
		}
	}

	if err := s.store.ApplyGitHubInstallation(ctx, by, "", change); err != nil {
		return fmt.Errorf("save installation %d: %w", installation.ID, err)
	}
	return nil
}
