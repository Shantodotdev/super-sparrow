package github

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type checkRun struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type checkRunList struct {
	CheckRuns []checkRun `json:"check_runs"`
}

type combinedStatus struct {
	State      string `json:"state"`
	TotalCount int    `json:"total_count"`
}

// repositoryPath is the API path of a repository, such as /repos/owner/name.
func repositoryPath(fullName string) string {
	owner, name, _ := strings.Cut(fullName, "/")
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
}

// combineChecks reduces a commit's check runs and commit statuses to one state: any failure
// wins, then anything unfinished, then success. Neutral and skipped runs count as success.
func combineChecks(runs []checkRun, status combinedStatus) string {
	failed, pending := false, false
	for _, run := range runs {
		if run.Status != "completed" {
			pending = true
			continue
		}
		switch run.Conclusion {
		case "failure", "timed_out", "cancelled", "action_required", "startup_failure":
			failed = true
		}
	}

	hasStatuses := status.TotalCount > 0
	if hasStatuses {
		switch status.State {
		case "failure", "error":
			failed = true
		case "pending":
			pending = true
		}
	}

	switch {
	case failed:
		return store.ChecksFailure
	case pending:
		return store.ChecksPending
	case len(runs) > 0 || hasStatuses:
		return store.ChecksSuccess
	default:
		return store.ChecksNone
	}
}

// refreshChecks reads a commit's check runs and statuses from GitHub and stores the combined
// state on every pull request whose head it is. A repository the App is not installed on is
// skipped.
func (s *Service) refreshChecks(ctx context.Context, repositoryID int64, sha string) error {
	repository, err := s.store.GetGitHubRepository(ctx, repositoryID)
	if err != nil {
		return fmt.Errorf("refresh checks: %w", err)
	}
	if repository.InstallationID == 0 {
		return nil
	}

	base := repositoryPath(repository.FullName) + "/commits/" + url.PathEscape(sha)
	var runs checkRunList
	if err := s.app.getAsInstallation(ctx, repository.InstallationID, base+"/check-runs?per_page=100", &runs); err != nil {
		return fmt.Errorf("refresh checks of %s: %w", repository.FullName, err)
	}
	var status combinedStatus
	if err := s.app.getAsInstallation(ctx, repository.InstallationID, base+"/status", &status); err != nil {
		return fmt.Errorf("refresh checks of %s: %w", repository.FullName, err)
	}

	if _, err := s.store.SetGitHubChecks(ctx, repositoryID, sha, combineChecks(runs.CheckRuns, status)); err != nil {
		return fmt.Errorf("refresh checks of %s: %w", repository.FullName, err)
	}
	return nil
}
