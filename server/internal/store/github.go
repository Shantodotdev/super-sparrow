package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Pull request states as Sparrow stores them. GitHub reports a merged pull request as closed;
// Sparrow tells the two apart.
const (
	PullRequestOpen   = "open"
	PullRequestClosed = "closed"
	PullRequestMerged = "merged"
)

// Review and check summaries of a pull request.
const (
	ReviewNone             = "none"
	ReviewApproved         = "approved"
	ReviewChangesRequested = "changes_requested"
	ReviewCommented        = "commented"
	ReviewDismissed        = "dismissed"

	ChecksNone    = "none"
	ChecksPending = "pending"
	ChecksSuccess = "success"
	ChecksFailure = "failure"
)

const (
	maxGitHubTitle   = 512
	maxGitHubText    = 255
	maxCommentQuote  = 280
	maxGitHubLogins  = 50
	maxSyncedPulls   = 500
	maxReviewsPerPR  = 300
	gitHubLoginLimit = 39
)

var gitHubLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// GitHubInstallation is one installation of the GitHub App on an account.
type GitHubInstallation struct {
	ID           int64  `json:"id"`
	AccountLogin string `json:"account_login"`
	Suspended    bool   `json:"suspended"`
}

// GitHubRepository is a repository the GitHub App can see, and the project it belongs to.
type GitHubRepository struct {
	ID             int64  `json:"id"`
	InstallationID int64  `json:"-"`
	FullName       string `json:"full_name"`
	URL            string `json:"url"`
	Private        bool   `json:"private"`
	Installed      bool   `json:"installed"`
	ProjectID      string `json:"-"`
	ProjectKey     string `json:"project_key"`
}

// MarshalJSON writes a repository with no project as project_key null.
func (r GitHubRepository) MarshalJSON() ([]byte, error) {
	type plain GitHubRepository
	return json.Marshal(struct {
		plain
		ProjectKey *string `json:"project_key"`
	}{plain(r), emptyAsNull(r.ProjectKey)})
}

// emptyAsNull returns nil for "", so JSON shows a missing value as null.
func emptyAsNull(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// GitHubReview is one submitted review of a pull request.
type GitHubReview struct {
	ID            int64     `json:"-"`
	ReviewerLogin string    `json:"reviewer_login"`
	State         string    `json:"state"`
	URL           string    `json:"url"`
	SubmittedAt   time.Time `json:"submitted_at"`
}

// GitHubPullRequest is a pull request as Sparrow stores it, with what the app shows beside it.
type GitHubPullRequest struct {
	ID                 int64          `json:"id"`
	RepositoryID       int64          `json:"-"`
	Repository         string         `json:"repository"`
	Number             int            `json:"number"`
	Title              string         `json:"title"`
	URL                string         `json:"url"`
	State              string         `json:"state"`
	Draft              bool           `json:"draft"`
	AuthorLogin        string         `json:"author_login"`
	AuthorPersonID     string         `json:"author_person_id"`
	HeadRef            string         `json:"head_ref"`
	BaseRef            string         `json:"base_ref"`
	HeadSHA            string         `json:"-"`
	Additions          int            `json:"additions"`
	Deletions          int            `json:"deletions"`
	ChangedFiles       int            `json:"changed_files"`
	RequestedReviewers []string       `json:"requested_reviewers"`
	ReviewState        string         `json:"review_state"`
	ChecksState        string         `json:"checks_state"`
	MergedByLogin      string         `json:"merged_by_login"`
	IssueID            string         `json:"-"`
	IssueKey           string         `json:"issue_key"`
	ProjectID          string         `json:"-"`
	OpenedAt           time.Time      `json:"opened_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	MergedAt           *time.Time     `json:"merged_at"`
	ClosedAt           *time.Time     `json:"closed_at"`
	Reviews            []GitHubReview `json:"reviews"`
}

// MarshalJSON writes a pull request with no linked issue or known author as null fields.
func (pr GitHubPullRequest) MarshalJSON() ([]byte, error) {
	type plain GitHubPullRequest
	return json.Marshal(struct {
		plain
		AuthorPersonID *string `json:"author_person_id"`
		IssueKey       *string `json:"issue_key"`
	}{plain(pr), emptyAsNull(pr.AuthorPersonID), emptyAsNull(pr.IssueKey)})
}

// GitHubPullRequestInput is a pull request as a webhook delivery or the GitHub API reports it.
// Partial inputs come from deliveries that carry only part of the pull request, such as reviews:
// they never overwrite the counts and merge details a full report stored.
type GitHubPullRequestInput struct {
	Repository         GitHubRepository
	ID                 int64
	Number             int
	Title              string
	URL                string
	State              string
	Draft              bool
	AuthorLogin        string
	HeadRef            string
	BaseRef            string
	HeadSHA            string
	Additions          int
	Deletions          int
	ChangedFiles       int
	RequestedReviewers []string
	MergedByLogin      string
	OpenedAt           time.Time
	UpdatedAt          time.Time
	MergedAt           *time.Time
	ClosedAt           *time.Time
	Partial            bool
}

// GitHubAction says what happened to a pull request in a webhook delivery.
type GitHubAction struct {
	// Name is GitHub's action, such as opened, closed, or synchronize.
	Name string
	// Sender is the GitHub login of whoever did it.
	Sender string
	// Reviewer is the login asked for a review, for review_requested.
	Reviewer string
}

// GitHubPullRequestChange reports what a delivery did to a pull request. Kind is the event
// recorded, such as github.pull_request.opened; it is "" when the delivery was already applied
// or changed nothing worth recording.
type GitHubPullRequestChange struct {
	PullRequest GitHubPullRequest
	Kind        string
}

// GitHubInstallationChange is a delivery or refresh that reports an installation's repositories.
type GitHubInstallationChange struct {
	Installation GitHubInstallation
	// Deleted removes the installation; its repositories stay, no longer installed.
	Deleted      bool
	Repositories []GitHubRepository
	// Complete says Repositories lists every repository of the installation, so any other
	// repository that was installed through it no longer is.
	Complete bool
	Removed  []int64
}

// GitHubReviewInput is a review from a webhook delivery or the GitHub API.
type GitHubReviewInput struct {
	ID            int64
	ReviewerLogin string
	State         string
	URL           string
	SubmittedAt   time.Time
}

// GitHubAccount is a teammate's GitHub login.
type GitHubAccount struct {
	PersonID string `json:"person_id"`
	Login    string `json:"login"`
}

// DiscordProjectChannel is the Discord channel a project posts pull request news to.
type DiscordProjectChannel struct {
	ProjectID  string `json:"-"`
	ProjectKey string `json:"project_key"`
	GuildID    string `json:"guild_id"`
	GuildName  string `json:"guild_name"`
	ChannelID  string `json:"channel_id"`
}

// claimDelivery records a webhook delivery and reports whether it is new. An empty ID is a
// change that did not come from a delivery, such as a refresh, and is always applied.
func claimDelivery(w *writeTx, deliveryID, event string) (bool, error) {
	if deliveryID == "" {
		return true, nil
	}
	if len(deliveryID) > 100 {
		return false, invalid("delivery", "must be at most 100 characters")
	}
	result, err := w.exec(`INSERT INTO github_deliveries (id, event, received_at) VALUES (?, ?, ?) ON CONFLICT (id) DO NOTHING`,
		deliveryID, event, w.timestamp())
	if err != nil {
		return false, fmt.Errorf("record GitHub delivery %s: %w", deliveryID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("record GitHub delivery %s: %w", deliveryID, err)
	}
	return rows == 1, nil
}

// ApplyGitHubInstallation records an installation and the repositories it reports, and records
// an event for each repository that becomes or stops being installed.
func (s *Store) ApplyGitHubInstallation(ctx context.Context, by Caller, deliveryID string, change GitHubInstallationChange) error {
	if change.Installation.ID <= 0 {
		return invalid("installation", "is required")
	}
	login, err := singleLineText("account_login", change.Installation.AccountLogin, maxGitHubText)
	if err != nil {
		return err
	}
	for i := range change.Repositories {
		if err := validateRepository(&change.Repositories[i]); err != nil {
			return err
		}
	}

	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireOwner("refresh GitHub installations"); err != nil {
			return err
		}
		fresh, err := claimDelivery(w, deliveryID, "installation")
		if err != nil || !fresh {
			return err
		}

		installationID := change.Installation.ID
		previous, err := installedRepositoryIDs(w, installationID)
		if err != nil {
			return err
		}

		if change.Deleted {
			if _, err := w.exec(`DELETE FROM github_installations WHERE id = ?`, installationID); err != nil {
				return fmt.Errorf("remove GitHub installation %d: %w", installationID, err)
			}
			return recordRemovedRepositories(w, previous)
		}

		_, err = w.exec(`INSERT INTO github_installations (id, account_login, suspended, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET account_login = excluded.account_login, suspended = excluded.suspended, updated_at = excluded.updated_at`,
			installationID, login, change.Installation.Suspended, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("save GitHub installation %d: %w", installationID, err)
		}

		present := map[int64]bool{}
		for _, repository := range change.Repositories {
			repository.InstallationID = installationID
			if _, err := saveRepository(w, repository); err != nil {
				return err
			}
			present[repository.ID] = true
			if !slices.Contains(previous, repository.ID) {
				if err := repositoryEvent(w, repository.ID, "github.repository.added", nil); err != nil {
					return err
				}
			}
		}

		var removed []int64
		for _, id := range previous {
			if (change.Complete && !present[id]) || slices.Contains(change.Removed, id) {
				removed = append(removed, id)
			}
		}
		for _, id := range removed {
			if _, err := w.exec(`UPDATE github_repositories SET installation_id = NULL, updated_at = ? WHERE id = ?`, w.timestamp(), id); err != nil {
				return fmt.Errorf("detach GitHub repository %d: %w", id, err)
			}
		}
		return recordRemovedRepositories(w, removed)
	})
}

func installedRepositoryIDs(w *writeTx, installationID int64) ([]int64, error) {
	rows, err := w.tx.QueryContext(w.ctx, `SELECT id FROM github_repositories WHERE installation_id = ? ORDER BY id`, installationID)
	if err != nil {
		return nil, fmt.Errorf("list installed GitHub repositories: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read installed GitHub repository: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func recordRemovedRepositories(w *writeTx, ids []int64) error {
	for _, id := range ids {
		if err := repositoryEvent(w, id, "github.repository.removed", nil); err != nil {
			return err
		}
	}
	return nil
}

// repositoryEvent records an event about a repository, in its project when it has one.
func repositoryEvent(w *writeTx, repositoryID int64, kind string, extra map[string]any) error {
	repository, err := getRepository(w.ctx, w.tx, repositoryID)
	if err != nil {
		return err
	}
	payload := map[string]any{"repository": repository.FullName, "url": repository.URL}
	for key, value := range extra {
		payload[key] = value
	}
	return w.event(repository.ProjectID, TargetRepository, strconv.FormatInt(repositoryID, 10), kind, payload)
}

func validateRepository(repository *GitHubRepository) error {
	if repository.ID <= 0 {
		return invalid("repository", "is required")
	}
	name, err := singleLineText("repository", repository.FullName, maxGitHubText)
	if err != nil {
		return err
	}
	if !strings.Contains(name, "/") {
		return invalid("repository", "must be owner/name")
	}
	repository.FullName = name
	url, err := gitHubURL("repository_url", repository.URL)
	if err != nil {
		return err
	}
	repository.URL = url
	return nil
}

// gitHubURL accepts only https links to github.com, so stored links are safe to open.
func gitHubURL(field, value string) (string, error) {
	value, err := optionalHTTPURL(field, value)
	if err != nil {
		return "", err
	}
	if value == "" || !strings.HasPrefix(value, "https://github.com/") {
		return "", invalid(field, "must be an https://github.com/ link")
	}
	return value, nil
}

// saveRepository inserts a repository or updates its name and visibility. An installation ID of
// 0 keeps the stored one, since pull request deliveries do not say which installation sent them.
func saveRepository(w *writeTx, repository GitHubRepository) (bool, error) {
	var installation any
	if repository.InstallationID != 0 {
		installation = repository.InstallationID
	}
	result, err := w.exec(`INSERT INTO github_repositories (id, installation_id, full_name, html_url, private, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET full_name = excluded.full_name, html_url = excluded.html_url, private = excluded.private,
			installation_id = COALESCE(excluded.installation_id, github_repositories.installation_id), updated_at = excluded.updated_at`,
		repository.ID, installation, repository.FullName, repository.URL, repository.Private, w.timestamp(), w.timestamp())
	if err != nil {
		return false, fmt.Errorf("save GitHub repository %s: %w", repository.FullName, err)
	}
	rows, _ := result.RowsAffected()
	return rows == 1, nil
}

const repositoryColumns = `r.id, COALESCE(r.installation_id, 0), r.full_name, r.html_url, r.private, COALESCE(r.project_id, ''), COALESCE(p.key, '')`

func scanRepository(row scanner) (GitHubRepository, error) {
	var repository GitHubRepository
	err := row.Scan(&repository.ID, &repository.InstallationID, &repository.FullName, &repository.URL, &repository.Private,
		&repository.ProjectID, &repository.ProjectKey)
	if errors.Is(err, sql.ErrNoRows) {
		return GitHubRepository{}, notFound("GitHub repository")
	}
	if err != nil {
		return GitHubRepository{}, fmt.Errorf("read GitHub repository: %w", err)
	}
	repository.Installed = repository.InstallationID != 0
	return repository, nil
}

func getRepository(ctx context.Context, q querier, id int64) (GitHubRepository, error) {
	return scanRepository(q.QueryRowContext(ctx, `SELECT `+repositoryColumns+`
		FROM github_repositories r LEFT JOIN projects p ON p.id = r.project_id WHERE r.id = ?`, id))
}

// GetGitHubRepository returns one repository.
func (s *Store) GetGitHubRepository(ctx context.Context, id int64) (GitHubRepository, error) {
	return getRepository(ctx, s.db, id)
}

// ListGitHubRepositories returns every repository the GitHub App has reported, by name.
func (s *Store) ListGitHubRepositories(ctx context.Context) ([]GitHubRepository, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+repositoryColumns+`
		FROM github_repositories r LEFT JOIN projects p ON p.id = r.project_id ORDER BY lower(r.full_name)`)
	if err != nil {
		return nil, fmt.Errorf("list GitHub repositories: %w", err)
	}
	defer rows.Close()

	repositories := []GitHubRepository{}
	for rows.Next() {
		repository, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, repository)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list GitHub repositories: %w", err)
	}
	return repositories, nil
}

// ListGitHubInstallations returns every installation of the GitHub App.
func (s *Store) ListGitHubInstallations(ctx context.Context) ([]GitHubInstallation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_login, suspended FROM github_installations ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list GitHub installations: %w", err)
	}
	defer rows.Close()

	installations := []GitHubInstallation{}
	for rows.Next() {
		var installation GitHubInstallation
		if err := rows.Scan(&installation.ID, &installation.AccountLogin, &installation.Suspended); err != nil {
			return nil, fmt.Errorf("read GitHub installation: %w", err)
		}
		installations = append(installations, installation)
	}
	return installations, rows.Err()
}

// LinkGitHubRepository puts a repository's pull requests in a project, or takes them out of
// every project when projectKey is "". Only an owner in the desktop app can do this.
func (s *Store) LinkGitHubRepository(ctx context.Context, by Caller, repositoryID int64, projectKey string) (GitHubRepository, error) {
	var repository GitHubRepository
	err := s.write(ctx, by, func(w *writeTx) error {
		if !w.caller.IsOwner() || w.client != ClientBrowser {
			return forbidden("only an owner in the desktop app can link GitHub repositories")
		}
		current, err := getRepository(ctx, w.tx, repositoryID)
		if err != nil {
			return err
		}

		var projectID any
		if projectKey != "" {
			project, err := getProject(ctx, w.tx, "key = ?", strings.ToUpper(strings.TrimSpace(projectKey)))
			if err != nil {
				return err
			}
			projectID = project.ID
			if project.ID == current.ProjectID {
				repository = current
				return nil
			}
		} else if current.ProjectID == "" {
			repository = current
			return nil
		}

		if _, err := w.exec(`UPDATE github_repositories SET project_id = ?, updated_at = ? WHERE id = ?`, projectID, w.timestamp(), repositoryID); err != nil {
			return fmt.Errorf("link GitHub repository %d: %w", repositoryID, err)
		}
		repository, err = getRepository(ctx, w.tx, repositoryID)
		if err != nil {
			return err
		}
		if repository.ProjectID != "" {
			return repositoryEvent(w, repositoryID, "github.repository.linked", map[string]any{"project_key": repository.ProjectKey})
		}

		// An unlink is recorded in the project the repository left, where its history is.
		return w.event(current.ProjectID, TargetRepository, strconv.FormatInt(repositoryID, 10), "github.repository.linked", map[string]any{
			"repository": repository.FullName, "url": repository.URL, "project_key": nil, "previous_project_key": current.ProjectKey,
		})
	})
	return repository, err
}

// ApplyGitHubPullRequest records a pull request delivery: it saves the pull request, links it
// to the issue it belongs to, marks that issue done when the pull request merges, and records
// the event that says what happened. A delivery already applied changes nothing.
func (s *Store) ApplyGitHubPullRequest(ctx context.Context, deliveryID string, input GitHubPullRequestInput, action GitHubAction) (GitHubPullRequestChange, error) {
	if err := validatePullRequest(&input); err != nil {
		return GitHubPullRequestChange{}, err
	}

	var change GitHubPullRequestChange
	err := s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		fresh, err := claimDelivery(w, deliveryID, "pull_request")
		if err != nil || !fresh {
			return err
		}

		previous, existed, err := savePullRequest(w, input)
		if err != nil {
			return err
		}
		current, err := getPullRequest(w.ctx, w.tx, input.ID)
		if err != nil {
			return err
		}

		kind, extra := pullRequestEventKind(action, previous, existed, current)
		if kind == "" {
			change.PullRequest = current
			return nil
		}
		if kind == "github.pull_request.merged" && current.IssueID != "" {
			if err := closeIssueForMergedPullRequest(w, current); err != nil {
				return err
			}
		}
		if err := pullRequestEvent(w, current, kind, action.Sender, extra); err != nil {
			return err
		}

		change = GitHubPullRequestChange{PullRequest: current, Kind: kind}
		return nil
	})
	return change, err
}

// pullRequestEventKind names the event a pull request delivery records.
func pullRequestEventKind(action GitHubAction, previous GitHubPullRequest, existed bool, current GitHubPullRequest) (string, map[string]any) {
	switch action.Name {
	case "opened", "reopened", "ready_for_review", "converted_to_draft":
		return "github.pull_request." + action.Name, nil
	case "closed":
		if current.State == PullRequestMerged {
			return "github.pull_request.merged", map[string]any{"merged_by": current.MergedByLogin}
		}
		return "github.pull_request.closed", nil
	case "review_requested":
		return "github.pull_request.review_requested", map[string]any{"reviewer": action.Reviewer}
	case "synchronize":
		return "github.pull_request.pushed", nil
	case "edited":
		if existed && previous.Title != current.Title {
			return "github.pull_request.edited", map[string]any{"previous_title": previous.Title}
		}
		if !existed {
			return "github.pull_request.edited", nil
		}
		return "", nil
	}
	if !existed {
		return "github.pull_request.updated", nil
	}
	return "", nil
}

func pullRequestEvent(w *writeTx, pr GitHubPullRequest, kind, sender string, extra map[string]any) error {
	payload := map[string]any{
		"repository": pr.Repository, "number": pr.Number, "title": pr.Title, "url": pr.URL, "github_login": sender,
		"pull_request_id": pr.ID,
	}
	if pr.IssueID != "" {
		payload["issue_id"] = pr.IssueID
	}
	for key, value := range extra {
		payload[key] = value
	}
	return w.event(pr.ProjectID, TargetPullRequest, strconv.FormatInt(pr.ID, 10), kind, payload)
}

func validatePullRequest(input *GitHubPullRequestInput) error {
	if err := validateRepository(&input.Repository); err != nil {
		return err
	}
	if input.ID <= 0 || input.Number <= 0 {
		return invalid("pull_request", "must have an ID and a number")
	}
	title, err := optionalText("title", strings.Join(strings.Fields(input.Title), " "), maxGitHubTitle)
	if err != nil {
		return err
	}
	input.Title = title
	if input.URL, err = gitHubURL("pull_request_url", input.URL); err != nil {
		return err
	}
	if input.State != PullRequestOpen && input.State != PullRequestClosed && input.State != PullRequestMerged {
		return invalid("state", "must be open, closed, or merged")
	}
	for _, field := range []*string{&input.AuthorLogin, &input.HeadRef, &input.BaseRef, &input.HeadSHA, &input.MergedByLogin} {
		if *field, err = optionalText("pull_request", *field, maxGitHubText); err != nil {
			return err
		}
	}
	if len(input.RequestedReviewers) > maxGitHubLogins {
		input.RequestedReviewers = input.RequestedReviewers[:maxGitHubLogins]
	}
	if input.RequestedReviewers == nil {
		input.RequestedReviewers = []string{}
	}
	if input.OpenedAt.IsZero() || input.UpdatedAt.IsZero() {
		return invalid("pull_request", "must have created and updated times")
	}
	return nil
}

// savePullRequest inserts or updates a pull request and links it to its issue. It returns the
// pull request as it was before, and whether it existed.
func savePullRequest(w *writeTx, input GitHubPullRequestInput) (GitHubPullRequest, bool, error) {
	if _, err := saveRepository(w, input.Repository); err != nil {
		return GitHubPullRequest{}, false, err
	}

	previous, err := getPullRequest(w.ctx, w.tx, input.ID)
	existed := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return GitHubPullRequest{}, false, err
	}

	reviewers, err := json.Marshal(input.RequestedReviewers)
	if err != nil {
		return GitHubPullRequest{}, false, fmt.Errorf("encode requested reviewers: %w", err)
	}
	if existed && input.Partial {
		_, err = w.exec(`UPDATE github_pull_requests SET title = ?, html_url = ?, state = CASE WHEN state = 'merged' THEN state ELSE ? END,
			draft = ?, head_ref = ?, base_ref = ?, head_sha = ?, updated_at = max(updated_at, ?) WHERE id = ?`,
			input.Title, input.URL, input.State, input.Draft, input.HeadRef, input.BaseRef, input.HeadSHA, formatTime(input.UpdatedAt), input.ID)
	} else {
		_, err = w.exec(`INSERT INTO github_pull_requests (id, repository_id, number, title, html_url, state, draft, author_login,
				head_ref, base_ref, head_sha, additions, deletions, changed_files, requested_reviewers, review_state, checks_state,
				merged_by_login, opened_at, updated_at, merged_at, closed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'none', 'none', ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET title = excluded.title, html_url = excluded.html_url, state = excluded.state,
				draft = excluded.draft, head_ref = excluded.head_ref, base_ref = excluded.base_ref,
				checks_state = CASE WHEN head_sha = excluded.head_sha THEN checks_state ELSE 'none' END, head_sha = excluded.head_sha,
				additions = excluded.additions, deletions = excluded.deletions, changed_files = excluded.changed_files,
				requested_reviewers = excluded.requested_reviewers, merged_by_login = excluded.merged_by_login,
				updated_at = excluded.updated_at, merged_at = excluded.merged_at, closed_at = excluded.closed_at`,
			input.ID, input.Repository.ID, input.Number, input.Title, input.URL, input.State, input.Draft, input.AuthorLogin,
			input.HeadRef, input.BaseRef, input.HeadSHA, input.Additions, input.Deletions, input.ChangedFiles, string(reviewers),
			input.MergedByLogin, formatTime(input.OpenedAt), formatTime(input.UpdatedAt), optionalTime(input.MergedAt), optionalTime(input.ClosedAt))
	}
	if err != nil {
		return GitHubPullRequest{}, false, fmt.Errorf("save pull request %s#%d: %w", input.Repository.FullName, input.Number, err)
	}

	if !existed || previous.IssueID == "" {
		issue, err := getIssue(w.ctx, w.tx, `i.pull_request_url = ? ORDER BY i.created_at LIMIT 1`, input.URL)
		if errors.Is(err, ErrNotFound) && input.HeadRef != "" {
			issue, err = findIssueForBranch(w.ctx, w.tx, input.HeadRef)
		}
		if err == nil {
			if _, err := w.exec(`UPDATE github_pull_requests SET issue_id = ? WHERE id = ?`, issue.ID, input.ID); err != nil {
				return GitHubPullRequest{}, false, fmt.Errorf("link pull request to issue %s: %w", issue.Key(), err)
			}
		} else if !errors.Is(err, ErrNotFound) {
			return GitHubPullRequest{}, false, err
		}
	}
	return previous, existed, nil
}

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func optionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

const pullRequestColumns = `pr.id, pr.repository_id, r.full_name, pr.number, pr.title, pr.html_url, pr.state, pr.draft, pr.author_login,
	COALESCE(ga.person_id, ''), pr.head_ref, pr.base_ref, pr.head_sha, pr.additions, pr.deletions, pr.changed_files,
	pr.requested_reviewers, pr.review_state, pr.checks_state, pr.merged_by_login, COALESCE(pr.issue_id, ''),
	COALESCE(ip.key || '-' || i.number, ''), COALESCE(r.project_id, ''), pr.opened_at, pr.updated_at, pr.merged_at, pr.closed_at`

const pullRequestFrom = ` FROM github_pull_requests pr
	JOIN github_repositories r ON r.id = pr.repository_id
	LEFT JOIN github_accounts ga ON ga.login = pr.author_login
	LEFT JOIN issues i ON i.id = pr.issue_id
	LEFT JOIN projects ip ON ip.id = i.project_id`

func scanPullRequest(row scanner) (GitHubPullRequest, error) {
	var pr GitHubPullRequest
	var reviewers, opened, updated string
	var merged, closed sql.NullString
	err := row.Scan(&pr.ID, &pr.RepositoryID, &pr.Repository, &pr.Number, &pr.Title, &pr.URL, &pr.State, &pr.Draft, &pr.AuthorLogin,
		&pr.AuthorPersonID, &pr.HeadRef, &pr.BaseRef, &pr.HeadSHA, &pr.Additions, &pr.Deletions, &pr.ChangedFiles,
		&reviewers, &pr.ReviewState, &pr.ChecksState, &pr.MergedByLogin, &pr.IssueID, &pr.IssueKey, &pr.ProjectID,
		&opened, &updated, &merged, &closed)
	if errors.Is(err, sql.ErrNoRows) {
		return GitHubPullRequest{}, notFound("pull request")
	}
	if err != nil {
		return GitHubPullRequest{}, fmt.Errorf("read pull request: %w", err)
	}

	if err := json.Unmarshal([]byte(reviewers), &pr.RequestedReviewers); err != nil {
		return GitHubPullRequest{}, fmt.Errorf("read requested reviewers: %w", err)
	}
	if pr.OpenedAt, pr.UpdatedAt, err = timestamps(opened, updated); err != nil {
		return GitHubPullRequest{}, err
	}
	for _, field := range []struct {
		value  sql.NullString
		target **time.Time
	}{{merged, &pr.MergedAt}, {closed, &pr.ClosedAt}} {
		if field.value.Valid {
			parsed, err := parseTime(field.value.String)
			if err != nil {
				return GitHubPullRequest{}, err
			}
			*field.target = &parsed
		}
	}
	pr.Reviews = []GitHubReview{}
	return pr, nil
}

func getPullRequest(ctx context.Context, q querier, id int64) (GitHubPullRequest, error) {
	return scanPullRequest(q.QueryRowContext(ctx, `SELECT `+pullRequestColumns+pullRequestFrom+` WHERE pr.id = ?`, id))
}

// ListGitHubPullRequests returns a project's pull requests with their reviews, most recently
// updated first. state is open, merged, closed, or all.
func (s *Store) ListGitHubPullRequests(ctx context.Context, projectKey, state string) ([]GitHubPullRequest, error) {
	if state != PullRequestOpen && state != PullRequestMerged && state != PullRequestClosed && state != "all" {
		return nil, invalid("state", "must be open, merged, closed, or all")
	}
	project, err := getProject(ctx, s.db, "key = ?", strings.ToUpper(strings.TrimSpace(projectKey)))
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT `+pullRequestColumns+pullRequestFrom+`
		WHERE r.project_id = ? AND (? = 'all' OR pr.state = ?)
		ORDER BY pr.updated_at DESC, pr.id DESC LIMIT 500`, project.ID, state, state)
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	defer rows.Close()

	pulls := []GitHubPullRequest{}
	index := map[int64]int{}
	for rows.Next() {
		pr, err := scanPullRequest(rows)
		if err != nil {
			return nil, err
		}
		index[pr.ID] = len(pulls)
		pulls = append(pulls, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	rows.Close()

	reviews, err := s.db.QueryContext(ctx, `SELECT v.pull_request_id, v.reviewer_login, v.state, v.html_url, v.submitted_at
		FROM github_reviews v JOIN github_pull_requests pr ON pr.id = v.pull_request_id
		JOIN github_repositories r ON r.id = pr.repository_id
		WHERE r.project_id = ? ORDER BY v.submitted_at`, project.ID)
	if err != nil {
		return nil, fmt.Errorf("list pull request reviews: %w", err)
	}
	defer reviews.Close()
	for reviews.Next() {
		var pullID int64
		var review GitHubReview
		var submitted string
		if err := reviews.Scan(&pullID, &review.ReviewerLogin, &review.State, &review.URL, &submitted); err != nil {
			return nil, fmt.Errorf("read pull request review: %w", err)
		}
		if review.SubmittedAt, err = parseTime(submitted); err != nil {
			return nil, err
		}
		if i, ok := index[pullID]; ok {
			pulls[i].Reviews = append(pulls[i].Reviews, review)
		}
	}
	return pulls, reviews.Err()
}

// ApplyGitHubReview records a review delivery and updates the pull request's review summary.
// action is submitted, edited, or dismissed.
func (s *Store) ApplyGitHubReview(ctx context.Context, deliveryID string, pull GitHubPullRequestInput, review GitHubReviewInput, action GitHubAction) (GitHubPullRequestChange, error) {
	pull.Partial = true
	if err := validatePullRequest(&pull); err != nil {
		return GitHubPullRequestChange{}, err
	}
	if err := validateReview(&review); err != nil {
		return GitHubPullRequestChange{}, err
	}

	var change GitHubPullRequestChange
	err := s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		fresh, err := claimDelivery(w, deliveryID, "pull_request_review")
		if err != nil || !fresh {
			return err
		}
		if _, _, err := savePullRequest(w, pull); err != nil {
			return err
		}
		if err := saveReview(w, pull.ID, review); err != nil {
			return err
		}
		if err := summarizeReviews(w, pull.ID); err != nil {
			return err
		}
		current, err := getPullRequest(w.ctx, w.tx, pull.ID)
		if err != nil {
			return err
		}

		kind := "github.review.submitted"
		if action.Name == "dismissed" {
			kind = "github.review.dismissed"
		} else if action.Name != "submitted" {
			change.PullRequest = current
			return nil
		}
		if err := pullRequestEvent(w, current, kind, review.ReviewerLogin, map[string]any{"state": review.State, "review_url": review.URL}); err != nil {
			return err
		}
		change = GitHubPullRequestChange{PullRequest: current, Kind: kind}
		return nil
	})
	return change, err
}

func validateReview(review *GitHubReviewInput) error {
	if review.ID <= 0 {
		return invalid("review", "is required")
	}
	switch review.State {
	case ReviewApproved, ReviewChangesRequested, ReviewCommented, ReviewDismissed:
	default:
		return invalid("review_state", "must be approved, changes_requested, commented, or dismissed")
	}
	var err error
	if review.ReviewerLogin, err = singleLineText("reviewer", review.ReviewerLogin, maxGitHubText); err != nil {
		return err
	}
	if review.URL, err = gitHubURL("review_url", review.URL); err != nil {
		return err
	}
	if review.SubmittedAt.IsZero() {
		return invalid("review", "must have a submitted time")
	}
	return nil
}

func saveReview(w *writeTx, pullID int64, review GitHubReviewInput) error {
	_, err := w.exec(`INSERT INTO github_reviews (id, pull_request_id, reviewer_login, state, html_url, submitted_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET state = excluded.state, html_url = excluded.html_url`,
		review.ID, pullID, review.ReviewerLogin, review.State, review.URL, formatTime(review.SubmittedAt))
	if err != nil {
		return fmt.Errorf("save review %d: %w", review.ID, err)
	}
	return nil
}

// summarizeReviews sets a pull request's review state from each reviewer's latest decision:
// any change request wins over approvals, and plain comments count only when nobody decided.
func summarizeReviews(w *writeTx, pullID int64) error {
	rows, err := w.tx.QueryContext(w.ctx, `SELECT reviewer_login, state FROM github_reviews WHERE pull_request_id = ? ORDER BY submitted_at, id`, pullID)
	if err != nil {
		return fmt.Errorf("read reviews: %w", err)
	}
	latest := map[string]string{}
	commented := false
	for rows.Next() {
		var login, state string
		if err := rows.Scan(&login, &state); err != nil {
			rows.Close()
			return fmt.Errorf("read review: %w", err)
		}
		switch state {
		case ReviewCommented:
			commented = true
		default:
			latest[strings.ToLower(login)] = state
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("read reviews: %w", err)
	}

	summary := ReviewNone
	if commented {
		summary = ReviewCommented
	}
	for _, state := range latest {
		if state == ReviewChangesRequested {
			summary = ReviewChangesRequested
			break
		}
		if state == ReviewApproved {
			summary = ReviewApproved
		}
	}
	if _, err := w.exec(`UPDATE github_pull_requests SET review_state = ? WHERE id = ?`, summary, pullID); err != nil {
		return fmt.Errorf("save review summary: %w", err)
	}
	return nil
}

// RecordGitHubComment records a comment on a pull request Sparrow already knows. Comments on
// other pull requests and on plain GitHub issues are ignored and return ErrNotFound.
func (s *Store) RecordGitHubComment(ctx context.Context, deliveryID string, repositoryID int64, number int, login, url, body string) error {
	url, err := gitHubURL("comment_url", url)
	if err != nil {
		return err
	}
	login, err = singleLineText("github_login", login, maxGitHubText)
	if err != nil {
		return err
	}
	excerpt := strings.Join(strings.Fields(body), " ")
	if runes := []rune(excerpt); len(runes) > maxCommentQuote {
		excerpt = string(runes[:maxCommentQuote-1]) + "…"
	}

	return s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		var id int64
		err := w.tx.QueryRowContext(ctx, `SELECT id FROM github_pull_requests WHERE repository_id = ? AND number = ?`, repositoryID, number).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("pull request")
		}
		if err != nil {
			return fmt.Errorf("find commented pull request: %w", err)
		}
		fresh, err := claimDelivery(w, deliveryID, "comment")
		if err != nil || !fresh {
			return err
		}
		pr, err := getPullRequest(ctx, w.tx, id)
		if err != nil {
			return err
		}
		return pullRequestEvent(w, pr, "github.comment.created", login, map[string]any{"comment_url": url, "body_excerpt": excerpt})
	})
}

// SetGitHubChecks records the combined check state of a commit on every pull request whose head
// it is, with an event for each pull request whose state changed.
func (s *Store) SetGitHubChecks(ctx context.Context, repositoryID int64, headSHA, state string) ([]GitHubPullRequestChange, error) {
	switch state {
	case ChecksNone, ChecksPending, ChecksSuccess, ChecksFailure:
	default:
		return nil, invalid("checks_state", "must be none, pending, success, or failure")
	}

	var changes []GitHubPullRequestChange
	err := s.write(ctx, Caller{ActorID: SystemActorID}, func(w *writeTx) error {
		rows, err := w.tx.QueryContext(ctx, `SELECT id, checks_state FROM github_pull_requests WHERE repository_id = ? AND head_sha = ?`, repositoryID, headSHA)
		if err != nil {
			return fmt.Errorf("find pull requests for commit: %w", err)
		}
		previous := map[int64]string{}
		for rows.Next() {
			var id int64
			var checks string
			if err := rows.Scan(&id, &checks); err != nil {
				rows.Close()
				return fmt.Errorf("read pull request checks: %w", err)
			}
			previous[id] = checks
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("read pull request checks: %w", err)
		}

		ids := slices.Sorted(maps.Keys(previous))
		for _, id := range ids {
			before := previous[id]
			if before == state {
				continue
			}
			if _, err := w.exec(`UPDATE github_pull_requests SET checks_state = ? WHERE id = ?`, state, id); err != nil {
				return fmt.Errorf("save pull request checks: %w", err)
			}
			pr, err := getPullRequest(ctx, w.tx, id)
			if err != nil {
				return err
			}
			if err := pullRequestEvent(w, pr, "github.checks.changed", "", map[string]any{"checks_state": state, "previous_checks_state": before}); err != nil {
				return err
			}
			changes = append(changes, GitHubPullRequestChange{PullRequest: pr, Kind: "github.checks.changed"})
		}
		return nil
	})
	return changes, err
}

// SyncGitHubPullRequests saves pull requests and reviews loaded from the GitHub API, such as
// when a repository is linked, and records one event for the whole load. It returns how many
// pull requests it saved.
func (s *Store) SyncGitHubPullRequests(ctx context.Context, by Caller, repositoryID int64, pulls []GitHubPullRequestInput, reviews map[int64][]GitHubReviewInput) (int, error) {
	if len(pulls) > maxSyncedPulls {
		pulls = pulls[:maxSyncedPulls]
	}
	trimmed := make(map[int64][]GitHubReviewInput, len(pulls))
	for i := range pulls {
		if err := validatePullRequest(&pulls[i]); err != nil {
			return 0, err
		}
		if pulls[i].Repository.ID != repositoryID {
			return 0, invalid("repository", "every pull request must belong to repository %d", repositoryID)
		}
		list := reviews[pulls[i].ID]
		if len(list) > maxReviewsPerPR {
			list = list[len(list)-maxReviewsPerPR:]
		}
		for j := range list {
			if err := validateReview(&list[j]); err != nil {
				return 0, err
			}
		}
		trimmed[pulls[i].ID] = list
	}

	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireOwner("load pull requests from GitHub"); err != nil {
			return err
		}
		if _, err := getRepository(ctx, w.tx, repositoryID); err != nil {
			return err
		}
		for _, pull := range pulls {
			if _, _, err := savePullRequest(w, pull); err != nil {
				return err
			}
			for _, review := range trimmed[pull.ID] {
				if err := saveReview(w, pull.ID, review); err != nil {
					return err
				}
			}
			if err := summarizeReviews(w, pull.ID); err != nil {
				return err
			}
		}
		return repositoryEvent(w, repositoryID, "github.repository.synced", map[string]any{"count": len(pulls)})
	})
	return len(pulls), err
}

// closeIssueForMergedPullRequest marks done the issue a merged pull request is linked to, and
// records the pull request's URL on it when it has none.
func closeIssueForMergedPullRequest(w *writeTx, pr GitHubPullRequest) error {
	issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, pr.IssueID)
	if err != nil {
		return err
	}
	return closeMergedIssue(w, issue, pr.URL)
}

// ListGitHubAccounts returns every teammate's GitHub login.
func (s *Store) ListGitHubAccounts(ctx context.Context) ([]GitHubAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT person_id, login FROM github_accounts ORDER BY lower(login)`)
	if err != nil {
		return nil, fmt.Errorf("list GitHub accounts: %w", err)
	}
	defer rows.Close()

	accounts := []GitHubAccount{}
	for rows.Next() {
		var account GitHubAccount
		if err := rows.Scan(&account.PersonID, &account.Login); err != nil {
			return nil, fmt.Errorf("read GitHub account: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// SetGitHubLogin sets or clears a teammate's GitHub login. People set their own and owners set
// anyone's; a Discord agent sets only the login of the person it works for. Sparrow does not
// check that the person owns the GitHub account.
func (s *Store) SetGitHubLogin(ctx context.Context, by Caller, personID, login string) (*GitHubAccount, error) {
	login = strings.TrimPrefix(strings.TrimSpace(login), "@")
	if login != "" && (len(login) > gitHubLoginLimit || !gitHubLoginPattern.MatchString(login) || strings.Contains(login, "--")) {
		return nil, invalid("login", "must be a GitHub username")
	}

	var account *GitHubAccount
	err := s.write(ctx, by, func(w *writeTx) error {
		switch {
		case w.caller.Kind == ActorHuman && (w.caller.ID == personID || w.caller.IsOwner()):
		case w.caller.Kind == ActorAgent && w.client == ClientDiscord && w.caller.OwnerActorID == personID:
		default:
			return forbidden("you can set only your own GitHub username")
		}
		person, err := getActor(ctx, w.tx, personID)
		if err != nil {
			return err
		}
		if person.Kind != ActorHuman || !person.Active() {
			return invalid("person", "must be an active teammate")
		}

		var previous string
		err = w.tx.QueryRowContext(ctx, `SELECT login FROM github_accounts WHERE person_id = ?`, personID).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read GitHub account: %w", err)
		}
		if login == "" {
			if previous == "" {
				return nil
			}
			if _, err := w.exec(`DELETE FROM github_accounts WHERE person_id = ?`, personID); err != nil {
				return fmt.Errorf("remove GitHub account: %w", err)
			}
			return w.event("", TargetActor, personID, "github.account.linked", map[string]any{"login": nil, "previous_login": previous})
		}

		account = &GitHubAccount{PersonID: personID, Login: login}
		if previous == login {
			return nil
		}
		_, err = w.exec(`INSERT INTO github_accounts (person_id, login) VALUES (?, ?)
			ON CONFLICT (person_id) DO UPDATE SET login = excluded.login`, personID, login)
		if err != nil {
			return mapUniqueViolation(err, "another teammate already uses this GitHub username", "save GitHub account")
		}
		payload := map[string]any{"login": login}
		if previous != "" {
			payload["previous_login"] = previous
		}
		return w.event("", TargetActor, personID, "github.account.linked", payload)
	})
	return account, err
}

// SetDiscordProjectChannel makes a Discord channel the place a project's pull request news is
// posted, on behalf of a verified member of an approved server.
func (s *Store) SetDiscordProjectChannel(ctx context.Context, source DiscordIdentity, projectKey, channelID string) (Project, error) {
	if err := validateDiscordGuildID(channelID); err != nil {
		return Project{}, invalid("channel", "must be a Discord channel ID")
	}

	by := Caller{ActorID: SystemActorID}
	if agentID, err := discordAgentID(ctx, s.db, source); err == nil {
		by = Caller{ActorID: agentID, Client: ClientDiscord, Discord: source}
	} else if !errors.Is(err, ErrForbidden) {
		return Project{}, err
	}

	var project Project
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := requireDiscordGuild(ctx, w.tx, source); err != nil {
			return err
		}
		var err error
		project, err = getProject(ctx, w.tx, "key = ?", strings.ToUpper(strings.TrimSpace(projectKey)))
		if err != nil {
			return err
		}
		_, err = w.exec(`INSERT INTO discord_project_channels (project_id, guild_id, channel_id) VALUES (?, ?, ?)
			ON CONFLICT (project_id) DO UPDATE SET guild_id = excluded.guild_id, channel_id = excluded.channel_id`,
			project.ID, source.GuildID, channelID)
		if err != nil {
			return fmt.Errorf("save Discord channel for %s: %w", project.Key, err)
		}
		return w.event(project.ID, TargetProject, project.ID, "discord.channel.set", map[string]any{
			"guild_id": source.GuildID, "channel_id": channelID, "discord_user_id": source.UserID,
		})
	})
	return project, err
}

const discordChannelQuery = `SELECT c.project_id, p.key, c.guild_id, COALESCE(g.name, ''), c.channel_id
	FROM discord_project_channels c JOIN projects p ON p.id = c.project_id
	LEFT JOIN discord_guild_approvals g ON g.guild_id = c.guild_id`

// ListDiscordProjectChannels returns the Discord channel of every project that has one.
func (s *Store) ListDiscordProjectChannels(ctx context.Context) ([]DiscordProjectChannel, error) {
	rows, err := s.db.QueryContext(ctx, discordChannelQuery+` ORDER BY p.key`)
	if err != nil {
		return nil, fmt.Errorf("list Discord project channels: %w", err)
	}
	defer rows.Close()

	channels := []DiscordProjectChannel{}
	for rows.Next() {
		var channel DiscordProjectChannel
		if err := rows.Scan(&channel.ProjectID, &channel.ProjectKey, &channel.GuildID, &channel.GuildName, &channel.ChannelID); err != nil {
			return nil, fmt.Errorf("read Discord project channel: %w", err)
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

// DiscordChannelForProject returns the channel a project posts to, or ErrNotFound. The caller
// checks that the channel's server is still approved.
func (s *Store) DiscordChannelForProject(ctx context.Context, projectID string) (DiscordProjectChannel, error) {
	var channel DiscordProjectChannel
	err := s.db.QueryRowContext(ctx, discordChannelQuery+` WHERE c.project_id = ?`, projectID).
		Scan(&channel.ProjectID, &channel.ProjectKey, &channel.GuildID, &channel.GuildName, &channel.ChannelID)
	if errors.Is(err, sql.ErrNoRows) {
		return DiscordProjectChannel{}, notFound("Discord channel")
	}
	if err != nil {
		return DiscordProjectChannel{}, fmt.Errorf("read Discord channel: %w", err)
	}
	return channel, nil
}

// RemoveDiscordProjectChannel stops posting a project's pull request news to Discord. Only an
// owner in the desktop app can do this.
func (s *Store) RemoveDiscordProjectChannel(ctx context.Context, by Caller, projectKey string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		if !w.caller.IsOwner() || w.client != ClientBrowser {
			return forbidden("only an owner in the desktop app can remove Discord channels")
		}
		project, err := getProject(ctx, w.tx, "key = ?", strings.ToUpper(strings.TrimSpace(projectKey)))
		if err != nil {
			return err
		}
		result, err := w.exec(`DELETE FROM discord_project_channels WHERE project_id = ?`, project.ID)
		if err != nil {
			return fmt.Errorf("remove Discord channel for %s: %w", project.Key, err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return notFound("Discord channel")
		}
		return w.event(project.ID, TargetProject, project.ID, "discord.channel.removed", map[string]any{})
	})
}

// DiscordUsersForGitHubLogins maps GitHub logins to the Discord user IDs of the teammates who use
// them, through each teammate's active Discord link in the guild. Logins with no linked teammate
// are left out.
func (s *Store) DiscordUsersForGitHubLogins(ctx context.Context, guildID string, logins []string) (map[string]string, error) {
	users := map[string]string{}
	for _, login := range logins {
		if login == "" {
			continue
		}
		var userID string
		err := s.db.QueryRowContext(ctx, `SELECT d.user_id FROM github_accounts ga
			JOIN actors p ON p.id = ga.person_id AND p.kind = 'human' AND p.disabled_at IS NULL
			JOIN actors a ON a.owner_actor_id = p.id AND a.kind = 'agent' AND a.agent_name = ? AND a.disabled_at IS NULL
			JOIN discord_links d ON d.agent_id = a.id AND d.guild_id = ?
			WHERE ga.login = ?`, discordAgentName, guildID, login).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("find Discord user for GitHub login: %w", err)
		}
		users[strings.ToLower(login)] = userID
	}
	return users, nil
}
