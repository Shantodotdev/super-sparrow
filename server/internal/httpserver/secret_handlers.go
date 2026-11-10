package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// secretEntryView is a key of a config in API responses, never with its value.
type secretEntryView struct {
	Name      string    `json:"name"`
	Source    string    `json:"source,omitempty"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// secretListingView is a config's keys, required keys, and required keys without a value.
type secretListingView struct {
	Secrets  []secretEntryView `json:"secrets"`
	Required []string          `json:"required"`
	Missing  []string          `json:"missing"`
}

func newSecretListingView(listing store.SecretListing) secretListingView {
	view := secretListingView{Secrets: make([]secretEntryView, 0, len(listing.Secrets)), Required: listing.Required, Missing: listing.Missing}
	for _, entry := range listing.Secrets {
		view.Secrets = append(view.Secrets, secretEntryView{
			Name: entry.Name, Source: string(entry.Source), Version: entry.Version, UpdatedAt: entry.UpdatedAt, UpdatedBy: entry.UpdatedBy,
		})
	}
	return view
}

// serviceTokenView is a CI token in API responses. Its secret is shown only when it is created.
type serviceTokenView struct {
	ID          string     `json:"id"`
	Config      string     `json:"config"`
	Label       string     `json:"label"`
	DisplayHint string     `json:"display_hint"`
	CreatedBy   string     `json:"created_by"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func newServiceTokenView(token store.ServiceToken) serviceTokenView {
	return serviceTokenView{
		ID: token.ID, Config: string(token.Config), Label: token.Label, DisplayHint: token.DisplayHint, CreatedBy: token.CreatedBy,
		LastUsedAt: token.LastUsedAt, ExpiresAt: token.ExpiresAt, RevokedAt: token.RevokedAt, CreatedAt: token.CreatedAt,
	}
}

// secretConfigFromPath reads the {config} path segment, such as staging.
func secretConfigFromPath(r *http.Request) (store.SecretConfig, error) {
	return store.ParseSecretConfig("config", r.PathValue("config"))
}

func (s *server) enableSecrets(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	project, err = s.store.EnableSecrets(r.Context(), identityFrom(r.Context()).caller(), project.ID)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"project": newProjectView(project)})
	return nil
}

func (s *server) secretsOverview(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	overview, err := s.store.SecretsOverview(r.Context(), identityFrom(r.Context()).caller(), project.ID)
	if err != nil {
		return err
	}

	var configs map[store.SecretConfig]secretListingView
	if overview.Configs != nil {
		configs = make(map[store.SecretConfig]secretListingView, len(overview.Configs))
		for config, listing := range overview.Configs {
			configs[config] = newSecretListingView(listing)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": overview.Enabled, "can_manage": overview.CanManage, "is_owner": overview.IsOwner, "configs": configs,
	})
	return nil
}

func (s *server) revealSecret(w http.ResponseWriter, r *http.Request) error {
	project, config, err := s.secretTarget(r)
	if err != nil {
		return err
	}

	value, err := s.store.RevealSecret(r.Context(), identityFrom(r.Context()).caller(), project.ID, config, r.PathValue("name"))
	if err != nil {
		return err
	}

	body := map[string]any{"name": value.Name, "value": value.Value, "version": value.Version}
	if value.Source != "" {
		body["source"] = value.Source
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, body)
	return nil
}

func (s *server) changeSecrets(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Set    map[string]string `json:"set"`
		Delete []string          `json:"delete"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	project, config, err := s.secretTarget(r)
	if err != nil {
		return err
	}

	listing, err := s.store.ChangeSecrets(r.Context(), identityFrom(r.Context()).caller(), project.ID, config,
		store.SecretChange{Set: body.Set, Delete: body.Delete})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, newSecretListingView(listing))
	return nil
}

func (s *server) secretHistory(w http.ResponseWriter, r *http.Request) error {
	project, config, err := s.secretTarget(r)
	if err != nil {
		return err
	}

	versions, err := s.store.SecretHistory(r.Context(), identityFrom(r.Context()).caller(), project.ID, config, r.PathValue("name"))
	if err != nil {
		return err
	}

	views := make([]map[string]any, 0, len(versions))
	for _, version := range versions {
		views = append(views, map[string]any{
			"version": version.Version, "deleted": version.Deleted, "actor_id": version.ActorID, "created_at": version.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": views})
	return nil
}

func (s *server) restoreSecret(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Version int `json:"version"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	project, config, err := s.secretTarget(r)
	if err != nil {
		return err
	}

	listing, err := s.store.RestoreSecret(r.Context(), identityFrom(r.Context()).caller(), project.ID, config, r.PathValue("name"), body.Version)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, newSecretListingView(listing))
	return nil
}

func (s *server) setRequiredKeys(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Names []string `json:"names"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	project, config, err := s.secretTarget(r)
	if err != nil {
		return err
	}

	listing, err := s.store.SetRequiredKeys(r.Context(), identityFrom(r.Context()).caller(), project.ID, config, body.Names)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, newSecretListingView(listing))
	return nil
}

func (s *server) downloadSecrets(w http.ResponseWriter, r *http.Request) error {
	format, err := downloadFormat(r)
	if err != nil {
		return err
	}
	project, config, err := s.secretTarget(r)
	if err != nil {
		return err
	}

	values, err := s.store.DownloadSecrets(r.Context(), identityFrom(r.Context()).caller(), project.ID, config)
	if err != nil {
		return err
	}
	return writeSecretValues(w, format, values)
}

// downloadWithServiceToken answers CI: the token in the Authorization header names the config.
// Requests are limited per client address, since the route is reached before any sign-in.
func (s *server) downloadWithServiceToken(w http.ResponseWriter, r *http.Request) error {
	if ok, retry := s.perDownloadIP.Allow(clientIP(r, s.trustedProxies)); !ok {
		return &auth.RateLimitError{RetryAfter: retry}
	}
	format, err := downloadFormat(r)
	if err != nil {
		return err
	}
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return store.ErrUnauthenticated
	}

	_, values, err := s.auth.DownloadWithServiceToken(r.Context(), strings.TrimSpace(token))
	if err != nil {
		return err
	}
	return writeSecretValues(w, format, values)
}

func (s *server) secretAccessLog(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	config, err := store.ParseSecretConfig("config", r.URL.Query().Get("config"))
	if err != nil {
		return err
	}
	limit, err := queryLimit(r)
	if err != nil {
		return err
	}
	var before int64
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		if before, err = strconv.ParseInt(cursor, 10, 64); err != nil || before <= 0 {
			return &store.ValidationError{Field: "cursor", Message: "must be a cursor from an earlier page"}
		}
	}

	entries, err := s.store.AccessLog(r.Context(), identityFrom(r.Context()).caller(), project.ID, config, before, limit)
	if err != nil {
		return err
	}

	views := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		views = append(views, map[string]any{
			"id": entry.ID, "config": entry.Config, "action": entry.Action, "names": entry.Names,
			"actor_id": nullable(entry.ActorID), "service_token_id": nullable(entry.ServiceTokenID), "created_at": entry.CreatedAt,
		})
	}
	next := ""
	if len(entries) == pageSize(limit) {
		next = strconv.FormatInt(entries[len(entries)-1].Sequence, 10)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": views, "next_cursor": next})
	return nil
}

func (s *server) listServiceTokens(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	tokens, err := s.store.ListServiceTokens(r.Context(), identityFrom(r.Context()).caller(), project.ID)
	if err != nil {
		return err
	}

	views := make([]serviceTokenView, 0, len(tokens))
	for _, token := range tokens {
		views = append(views, newServiceTokenView(token))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": views})
	return nil
}

func (s *server) createServiceToken(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Config string `json:"config"`
		Label  string `json:"label"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	config, err := store.ParseSecretConfig("config", body.Config)
	if err != nil {
		return err
	}

	created, err := s.auth.CreateServiceToken(r.Context(), identityFrom(r.Context()).caller(), project.ID, config, body.Label)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"token": newServiceTokenView(created.Token), "secret": created.Secret})
	return nil
}

func (s *server) renewServiceToken(w http.ResponseWriter, r *http.Request) error {
	token, err := s.store.RenewServiceToken(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"))
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"token": newServiceTokenView(token)})
	return nil
}

func (s *server) revokeServiceToken(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.RevokeServiceToken(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id")); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) setEnvironmentsPermission(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		CanManageEnvironments *bool `json:"can_manage_environments"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if body.CanManageEnvironments == nil {
		return &store.ValidationError{Field: "can_manage_environments", Message: "is required"}
	}

	person, err := s.store.SetCanManageEnvironments(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"), *body.CanManageEnvironments)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"person": newActorView(person)})
	return nil
}

// secretTarget loads the project and config named by the {key} and {config} path segments.
func (s *server) secretTarget(r *http.Request) (store.Project, store.SecretConfig, error) {
	config, err := secretConfigFromPath(r)
	if err != nil {
		return store.Project{}, "", err
	}
	project, err := s.projectFromPath(r)
	if err != nil {
		return store.Project{}, "", err
	}
	return project, config, nil
}

// secretFormat is how a download writes values.
type secretFormat string

const (
	formatJSON  secretFormat = "json"
	formatEnv   secretFormat = "env"
	formatShell secretFormat = "shell"
)

func downloadFormat(r *http.Request) (secretFormat, error) {
	format := secretFormat(r.URL.Query().Get("format"))
	switch format {
	case formatJSON, formatEnv, formatShell:
		return format, nil
	}
	return "", &store.ValidationError{Field: "format", Message: "must be json, env, or shell"}
}

// writeSecretValues writes values in format. A .env line is NAME='value'; a value with a single
// quote or a line break cannot be written that way and is refused rather than mangled. Shell
// lines are export statements that quote every value safely.
func writeSecretValues(w http.ResponseWriter, format secretFormat, values store.SecretValues) error {
	w.Header().Set("Cache-Control", "no-store")
	if format == formatJSON {
		writeJSON(w, http.StatusOK, values.Values)
		return nil
	}

	var body strings.Builder
	for _, name := range values.Names {
		value := values.Values[name]
		switch format {
		case formatEnv:
			if strings.ContainsAny(value, "'\n\r") {
				return &store.ValidationError{Field: "format", Message: fmt.Sprintf("%s has a single quote or a line break; download it as json or shell", name)}
			}
			fmt.Fprintf(&body, "%s='%s'\n", name, value)
		case formatShell:
			fmt.Fprintf(&body, "export %s='%s'\n", name, strings.ReplaceAll(value, "'", `'\''`))
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(body.String()))
	return err
}

// nullable writes an empty ID as JSON null.
func nullable(id string) json.RawMessage {
	if id == "" {
		return json.RawMessage("null")
	}
	encoded, _ := json.Marshal(id)
	return encoded
}
