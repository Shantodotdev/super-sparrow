package store

import (
	"context"
	"errors"
	"strings"
)

// MergedPullRequest is a pull request that was merged, as GitHub reports it.
type MergedPullRequest struct {
	URL    string
	Branch string
}

// CloseIssueForMergedPullRequest marks done the issue a merged pull request belongs to: the issue
// that records the pull request's URL, or else the issue its branch belongs to. It records the
// pull request's URL on the issue when the issue has none. It returns the issue and whether it
// changed anything; an issue that is already done or canceled is left as it is. It fails with
// ErrNotFound when no issue matches.
func (s *Store) CloseIssueForMergedPullRequest(ctx context.Context, by Caller, pr MergedPullRequest) (Issue, bool, error) {
	url, err := optionalHTTPURL("pull_request_url", pr.URL)
	if err != nil {
		return Issue{}, false, err
	}
	if url == "" {
		return Issue{}, false, invalid("pull_request_url", "is required")
	}

	var issue Issue
	var changed bool
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.pull_request_url = ? ORDER BY i.created_at LIMIT 1`, url)
		if errors.Is(err, ErrNotFound) && strings.TrimSpace(pr.Branch) != "" {
			current, err = findIssueForBranch(w.ctx, w.tx, pr.Branch)
		}
		if err != nil {
			return err
		}

		if current.Status.closed() {
			issue = current
			return nil
		}
		if err := closeMergedIssue(w, current, url); err != nil {
			return err
		}

		changed = true
		issue, err = getIssue(w.ctx, w.tx, `i.id = ?`, current.ID)
		return err
	})

	return issue, changed, err
}

// closeMergedIssue marks an issue done because its pull request merged, and records the pull
// request's URL on the issue when it has none. An issue that is already done or canceled is left
// as it is.
func closeMergedIssue(w *writeTx, issue Issue, url string) error {
	if issue.Status.closed() {
		return nil
	}
	if issue.PullRequestURL == "" {
		if err := updateIssueFields(w, issue, IssueUpdate{PullRequestURL: Change(url)}); err != nil {
			return err
		}
	}
	return setIssueStatusWith(w, issue, IssueDone, map[string]any{"reason": "pull_request_merged", "pull_request_url": url})
}
