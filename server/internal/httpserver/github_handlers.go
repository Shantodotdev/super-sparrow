package httpserver

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// githubDisabled is the refusal for an action that needs the GitHub App when it is not set up.
func githubDisabled() error {
	return store.Refuse(store.ErrNotFound, "the GitHub App is not configured")
}

// requireGitHubApp returns the refusal for actions that read from GitHub when the App is off.
func (s *server) requireGitHubApp() error {
	if s.github == nil || !s.github.Configured() {
		return githubDisabled()
	}
	return nil
}

// repositoryFromPath reads the {id} path segment as a GitHub repository ID.
func repositoryFromPath(r *http.Request) (int64, error) {
	value := r.PathValue("id")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
		return 0, &store.ValidationError{Field: "id", Message: "must be a positive whole number"}
	}
	return id, nil
}

// listPullRequests returns a project's pull requests in one state.
func (s *server) listPullRequests(w http.ResponseWriter, r *http.Request) error {
	state := r.URL.Query().Get("state")
	if state == "" {
		return &store.ValidationError{Field: "state", Message: "is required: open, merged, closed, or all"}
	}

	pulls, err := s.store.ListGitHubPullRequests(r.Context(), r.PathValue("key"), state)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"pull_requests": pulls})
	return nil
}

// githubSettings writes the GitHub settings page's data: the App, its repositories, the
// teammates' GitHub logins, and the Discord channels of projects.
func (s *server) githubSettings(w http.ResponseWriter, r *http.Request) error {
	repositories, err := s.store.ListGitHubRepositories(r.Context())
	if err != nil {
		return err
	}
	accounts, err := s.store.ListGitHubAccounts(r.Context())
	if err != nil {
		return err
	}
	channels, err := s.store.ListDiscordProjectChannels(r.Context())
	if err != nil {
		return err
	}

	app := map[string]any{"configured": false, "install_url": nil}
	if s.github != nil && s.github.Configured() {
		app = map[string]any{"configured": true, "install_url": s.github.InstallURL()}
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": app, "repositories": repositories, "accounts": accounts, "channels": channels})
	return nil
}

// refreshGitHub asks GitHub which accounts installed the App and which repositories it sees.
func (s *server) refreshGitHub(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireGitHubApp(); err != nil {
		return err
	}
	if err := s.github.Refresh(r.Context(), identityFrom(r.Context()).caller()); err != nil {
		return err
	}
	return s.githubSettings(w, r)
}

// linkGitHubRepository puts a repository's pull requests in a project, or in none when the
// project key is null. A newly linked repository's pull requests are loaded straight away; a
// failure to load them is logged and does not undo the link.
func (s *server) linkGitHubRepository(w http.ResponseWriter, r *http.Request) error {
	id, err := repositoryFromPath(r)
	if err != nil {
		return err
	}

	var body struct {
		ProjectKey *string `json:"project_key"`
	}
	nulls, err := decodeUpdate(w, r, &body)
	if err != nil {
		return err
	}
	if body.ProjectKey == nil && !nulls["project_key"] {
		return &store.ValidationError{Field: "project_key", Message: "is required: a project key, or null to unlink"}
	}
	projectKey := ""
	if body.ProjectKey != nil {
		projectKey = *body.ProjectKey
	}

	caller := identityFrom(r.Context()).caller()
	repository, err := s.store.LinkGitHubRepository(r.Context(), caller, id, projectKey)
	if err != nil {
		return err
	}

	if repository.ProjectKey != "" && repository.Installed && s.github != nil && s.github.Configured() {
		if _, err := s.github.SyncRepository(r.Context(), caller, id); err != nil {
			s.logger.ErrorContext(r.Context(), "load pull requests of linked repository", slog.String("repository", repository.FullName), "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"repository": repository})
	return nil
}

// syncGitHubRepository reloads a repository's pull requests from GitHub.
func (s *server) syncGitHubRepository(w http.ResponseWriter, r *http.Request) error {
	id, err := repositoryFromPath(r)
	if err != nil {
		return err
	}
	if err := s.requireGitHubApp(); err != nil {
		return err
	}

	synced, err := s.github.SyncRepository(r.Context(), identityFrom(r.Context()).caller(), id)
	if err != nil {
		return err
	}
	repository, err := s.store.GetGitHubRepository(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"repository": repository, "synced": synced})
	return nil
}

// removeDiscordChannel stops posting a project's pull request news to Discord.
func (s *server) removeDiscordChannel(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.RemoveDiscordProjectChannel(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("key")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// listGitHubAccounts returns every teammate's GitHub login.
func (s *server) listGitHubAccounts(w http.ResponseWriter, r *http.Request) error {
	accounts, err := s.store.ListGitHubAccounts(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
	return nil
}

// setGitHubLogin sets or clears a teammate's GitHub login. The store decides who may: people
// change their own, and owners anyone's.
func (s *server) setGitHubLogin(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Login *string `json:"login"`
	}
	nulls, err := decodeUpdate(w, r, &body)
	if err != nil {
		return err
	}
	if body.Login == nil && !nulls["login"] {
		return &store.ValidationError{Field: "login", Message: "is required: a GitHub username, or null to remove it"}
	}
	login := ""
	if body.Login != nil {
		login = *body.Login
	}

	account, err := s.store.SetGitHubLogin(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"), login)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": account})
	return nil
}
