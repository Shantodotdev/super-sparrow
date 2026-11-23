package github

import (
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// user is the part of a GitHub user object Sparrow reads. GitHub sends null for a deleted user.
type user struct {
	Login string `json:"login"`
}

type repositoryPayload struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
}

type installationPayload struct {
	ID          int64      `json:"id"`
	SuspendedAt *time.Time `json:"suspended_at"`
	Account     *user      `json:"account"`
}

// pullPayload is a pull request as webhook deliveries and the pull requests API report it.
type pullPayload struct {
	ID                 int64      `json:"id"`
	Number             int        `json:"number"`
	Title              string     `json:"title"`
	HTMLURL            string     `json:"html_url"`
	State              string     `json:"state"`
	Draft              bool       `json:"draft"`
	Merged             bool       `json:"merged"`
	MergedAt           *time.Time `json:"merged_at"`
	ClosedAt           *time.Time `json:"closed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Additions          int        `json:"additions"`
	Deletions          int        `json:"deletions"`
	ChangedFiles       int        `json:"changed_files"`
	User               *user      `json:"user"`
	MergedBy           *user      `json:"merged_by"`
	RequestedReviewers []user     `json:"requested_reviewers"`
	Head               struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type reviewPayload struct {
	ID          int64     `json:"id"`
	User        *user     `json:"user"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type commentPayload struct {
	User    *user  `json:"user"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

// login returns the user's login, or "" for a deleted user.
func (u *user) login() string {
	if u == nil {
		return ""
	}
	return u.Login
}

// githubURL is the page of a repository on github.com.
func githubURL(fullName string) string { return "https://github.com/" + fullName }

// storeRepository converts a payload's repository. installationID is 0 when the delivery does not
// name an installation, which keeps the one already stored.
func (r repositoryPayload) storeRepository(installationID int64) store.GitHubRepository {
	return store.GitHubRepository{
		ID: r.ID, InstallationID: installationID, FullName: r.FullName, URL: githubURL(r.FullName), Private: r.Private,
	}
}

// state is the pull request's state as Sparrow stores it: GitHub reports a merged pull request
// as closed.
func (p pullPayload) state() string {
	switch {
	case p.State != "closed":
		return store.PullRequestOpen
	case p.Merged || p.MergedAt != nil:
		return store.PullRequestMerged
	default:
		return store.PullRequestClosed
	}
}

// input converts the pull request to what the store saves.
func (p pullPayload) input(repository store.GitHubRepository) store.GitHubPullRequestInput {
	reviewers := make([]string, 0, len(p.RequestedReviewers))
	for _, reviewer := range p.RequestedReviewers {
		reviewers = append(reviewers, reviewer.Login)
	}
	return store.GitHubPullRequestInput{
		Repository:         repository,
		ID:                 p.ID,
		Number:             p.Number,
		Title:              p.Title,
		URL:                p.HTMLURL,
		State:              p.state(),
		Draft:              p.Draft,
		AuthorLogin:        p.User.login(),
		HeadRef:            p.Head.Ref,
		BaseRef:            p.Base.Ref,
		HeadSHA:            p.Head.SHA,
		Additions:          p.Additions,
		Deletions:          p.Deletions,
		ChangedFiles:       p.ChangedFiles,
		RequestedReviewers: reviewers,
		MergedByLogin:      p.MergedBy.login(),
		OpenedAt:           p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
		MergedAt:           p.MergedAt,
		ClosedAt:           p.ClosedAt,
	}
}

// input converts a review to what the store saves. The state is lowercased, as GitHub sends
// APPROVED where a delivery sends approved; ok is false for a review still pending.
func (r reviewPayload) input() (review store.GitHubReviewInput, ok bool) {
	state := strings.ToLower(r.State)
	if state == "pending" {
		return store.GitHubReviewInput{}, false
	}

	login := r.User.login()
	if login == "" {
		login = "ghost"
	}
	return store.GitHubReviewInput{ID: r.ID, ReviewerLogin: login, State: state, URL: r.HTMLURL, SubmittedAt: r.SubmittedAt}, true
}
