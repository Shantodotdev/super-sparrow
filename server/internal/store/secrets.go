package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxSecretValue is the largest secret value in bytes.
	MaxSecretValue = 64 << 10
	// maxSecretChanges is the most keys one change may set or delete, such as a pasted .env.
	maxSecretChanges = 500
)

var secretNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

// unsafeNamePattern matches variables that change how programs load or start, such as
// LD_PRELOAD or NODE_OPTIONS. sparrow run puts dev values in the environment of whatever it
// starts, so one of these in the team defaults would run code on every teammate's machine.
var unsafeNamePattern = regexp.MustCompile(`^(PATH|ENV|BASH_ENV|SHELLOPTS|PS4|IFS|HOME|SHELL|LD_.*|DYLD_.*|NODE_OPTIONS|NODE_PATH|PYTHONPATH|PYTHONSTARTUP|PYTHONHOME|PERL5OPT|PERL5LIB|RUBYOPT|RUBYLIB|JAVA_TOOL_OPTIONS|_JAVA_OPTIONS|GIT_.*|SPARROW_.*)$`)

// SecretBox encrypts secret values. Context names the record a value belongs to; a value only
// opens with the context it was sealed with.
type SecretBox interface {
	KeyID() string
	Seal(plaintext, context []byte) ([]byte, error)
	Open(ciphertext []byte, keyID string, context []byte) ([]byte, error)
}

// SecretConfig names a config as people see it.
type SecretConfig string

const (
	// ConfigDev is a person's own dev: the team dev defaults plus that person's overrides.
	ConfigDev         SecretConfig = "dev"
	ConfigDevDefaults SecretConfig = "dev-defaults"
	ConfigStaging     SecretConfig = "staging"
	ConfigProduction  SecretConfig = "production"
)

// SecretConfigs are every config, in the order they are shown.
var SecretConfigs = []SecretConfig{ConfigDev, ConfigDevDefaults, ConfigStaging, ConfigProduction}

// ParseSecretConfig checks that value names a config.
func ParseSecretConfig(field, value string) (SecretConfig, error) {
	config := SecretConfig(value)
	if !slices.Contains(SecretConfigs, config) {
		return "", invalid(field, "must be dev, dev-defaults, staging, or production")
	}
	return config, nil
}

// team reports whether the config belongs to the team rather than one person.
func (c SecretConfig) team() bool { return c != ConfigDev }

// sensitive reports whether reading the config's values needs the staging and production
// permission and a browser.
func (c SecretConfig) sensitive() bool { return c == ConfigStaging || c == ConfigProduction }

// stored is the config's name in the secrets table. A person's dev is stored as their overrides.
func (c SecretConfig) stored() string {
	switch c {
	case ConfigDev:
		return "dev_personal"
	case ConfigDevDefaults:
		return "dev_defaults"
	}
	return string(c)
}

// requiredList is the config whose required keys apply: the team dev defaults share dev's list.
func (c SecretConfig) requiredList() string {
	if c == ConfigDevDefaults {
		return string(ConfigDev)
	}
	return string(c)
}

// SecretSource says where a key of a person's dev gets its value.
type SecretSource string

const (
	SourceDefault  SecretSource = "default"
	SourceOverride SecretSource = "override"
)

// SecretEntry is a key of a config without its value.
type SecretEntry struct {
	Name string
	// Source is set only for a person's dev.
	Source    SecretSource
	Version   int
	UpdatedAt time.Time
	UpdatedBy string
}

// SecretListing is a config's keys, its required keys, and the required keys without a value.
type SecretListing struct {
	Secrets  []SecretEntry
	Required []string
	Missing  []string
}

// SecretsOverview is everything the secrets page shows a caller about a project, without values.
type SecretsOverview struct {
	Enabled   bool
	CanManage bool
	IsOwner   bool
	// Configs is nil until secrets are turned on for the project.
	Configs map[SecretConfig]SecretListing
}

// SecretValue is one key's current value.
type SecretValue struct {
	Name    string
	Value   string
	Version int
	Source  SecretSource
}

// SecretVersion is one value a key has had, without the value.
type SecretVersion struct {
	Version   int
	Deleted   bool
	ActorID   string
	CreatedAt time.Time
}

// SecretChange sets and deletes keys of one config together. In a person's dev, delete removes
// the person's override, so the key goes back to the team default.
type SecretChange struct {
	Set    map[string]string
	Delete []string
}

// SecretValues is a config's values, read together for a download. Names is in name order.
type SecretValues struct {
	Names  []string
	Values map[string]string
}

// AccessAction is what an access log entry records.
type AccessAction string

const (
	AccessReveal   AccessAction = "reveal"
	AccessDownload AccessAction = "download"
)

// AccessLogEntry records who read secret values. Exactly one of ActorID and ServiceTokenID is set.
type AccessLogEntry struct {
	Sequence       int64
	ID             string
	Config         SecretConfig
	Action         AccessAction
	Names          []string
	ActorID        string
	ServiceTokenID string
	CreatedAt      time.Time
}

// secretViewer is a caller with the person whose dev they use: themselves, or an agent's owner.
type secretViewer struct {
	actor  Actor
	person Actor
	client Client
}

// canManage reports whether the viewer may read and edit the team configs' values. It needs a
// person with the permission in a browser, never the CLI or a token, since agents run where
// those are.
func (v secretViewer) canManage() bool {
	return v.actor.Kind == ActorHuman && v.client == ClientBrowser && v.actor.CanManage()
}

// canReadValues reports whether the viewer may read the values of config. Everyone signed in
// reads their own dev, which includes the team dev defaults.
func (v secretViewer) canReadValues(config SecretConfig) bool {
	if config.sensitive() {
		return v.canManage()
	}
	return v.client != "" && (v.actor.Kind == ActorHuman || v.actor.Kind == ActorAgent)
}

// checkWrite allows a person to change their own dev from a browser or the CLI, and someone who
// can manage to change the team configs.
func (v secretViewer) checkWrite(config SecretConfig) error {
	if v.actor.Kind != ActorHuman {
		return forbidden("only a person can change secrets, not an agent")
	}
	if config.team() && !v.canManage() {
		return forbidden("changing team secrets needs the staging and production permission, in the browser")
	}
	if !config.team() && v.client != ClientBrowser && v.client != ClientCLI {
		return forbidden("sign in to change your dev secrets")
	}
	return nil
}

// newSecretViewer loads the caller and, for an agent, its owner. Both must be active.
func newSecretViewer(ctx context.Context, q querier, by Caller) (secretViewer, error) {
	actor, err := getActor(ctx, q, by.ActorID)
	if errors.Is(err, ErrNotFound) {
		return secretViewer{}, forbidden("unknown caller")
	}
	if err != nil {
		return secretViewer{}, err
	}
	if !actor.Active() {
		return secretViewer{}, forbidden("caller is disabled")
	}

	viewer := secretViewer{actor: actor, person: actor, client: by.Client}
	switch actor.Kind {
	case ActorHuman:
	case ActorAgent:
		owner, err := getActor(ctx, q, actor.OwnerActorID)
		if err != nil {
			return secretViewer{}, err
		}
		if !owner.Active() {
			return secretViewer{}, forbidden("the agent's owner is disabled")
		}
		viewer.person = owner
	default:
		return secretViewer{}, forbidden("only people and their agents can use secrets")
	}
	return viewer, nil
}

// secretsProject loads a project whose secrets are turned on.
func secretsProject(ctx context.Context, q querier, projectID string) (Project, error) {
	project, err := getProject(ctx, q, `id = ?`, projectID)
	if err != nil {
		return Project{}, err
	}
	if !project.SecretsEnabled {
		return Project{}, Refuse(ErrConflict, "secrets are not turned on for %s; an owner turns them on", project.Key)
	}
	return project, nil
}

// EnableSecrets turns on secrets for a project. Only an owner can.
func (s *Store) EnableSecrets(ctx context.Context, by Caller, projectID string) (Project, error) {
	var project Project
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("turn on secrets"); err != nil {
			return err
		}
		if err := w.requireOwner("turn on secrets"); err != nil {
			return err
		}

		var err error
		project, err = getProject(w.ctx, w.tx, `id = ?`, projectID)
		if err != nil || project.SecretsEnabled {
			return err
		}

		if _, err := w.exec(`UPDATE projects SET secrets_enabled_at = ?1, updated_at = ?1 WHERE id = ?2`, w.timestamp(), project.ID); err != nil {
			return fmt.Errorf("turn on secrets: %w", err)
		}
		project.SecretsEnabled, project.UpdatedAt = true, w.now

		return w.event(project.ID, TargetProject, project.ID, "secrets.enabled", map[string]any{"key": project.Key})
	})
	return project, err
}

// SetCanManageEnvironments gives a member the staging and production permission or takes it
// away. Only an owner can, and only for people.
func (s *Store) SetCanManageEnvironments(ctx context.Context, by Caller, personID string, allowed bool) (Actor, error) {
	var person Actor
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("change who manages staging and production"); err != nil {
			return err
		}
		if err := w.requireOwner("change who manages staging and production"); err != nil {
			return err
		}

		var err error
		person, err = getActor(w.ctx, w.tx, personID)
		if err != nil {
			return err
		}
		if person.Kind != ActorHuman {
			return invalid("person_id", "only people can manage staging and production")
		}
		if person.CanManageEnvironments == allowed {
			return nil
		}

		_, err = w.exec(`UPDATE actors SET can_manage_environments = ?, updated_at = ? WHERE id = ?`, allowed, w.timestamp(), person.ID)
		if err != nil {
			return fmt.Errorf("change environments permission: %w", err)
		}
		person.CanManageEnvironments, person.UpdatedAt = allowed, w.now

		// CI tokens a member made stop working when they lose the permission that let them.
		if !allowed && !person.IsOwner() {
			if err := revokeServiceTokensBy(w, person.ID); err != nil {
				return err
			}
		}

		return w.event("", TargetActor, person.ID, "actor.environments_permission_changed",
			map[string]any{"can_manage_environments": allowed})
	})
	return person, err
}

// SecretsOverview lists a project's configs for the caller: every key name and required key, with
// the caller's own dev. People and agents see the names of every config, but no values.
func (s *Store) SecretsOverview(ctx context.Context, by Caller, projectID string) (SecretsOverview, error) {
	viewer, err := newSecretViewer(ctx, s.db, by)
	if err != nil {
		return SecretsOverview{}, err
	}
	project, err := getProject(ctx, s.db, `id = ?`, projectID)
	if err != nil {
		return SecretsOverview{}, err
	}

	overview := SecretsOverview{
		Enabled: project.SecretsEnabled, CanManage: viewer.canManage(),
		IsOwner: viewer.actor.IsOwner(),
	}
	if !project.SecretsEnabled {
		return overview, nil
	}

	overview.Configs = make(map[SecretConfig]SecretListing, len(SecretConfigs))
	for _, config := range SecretConfigs {
		listing, err := secretListing(ctx, s.db, project.ID, config, viewer.person.ID)
		if err != nil {
			return SecretsOverview{}, err
		}
		overview.Configs[config] = listing
	}
	return overview, nil
}

// secretListing returns config's keys for person, with its required keys and those without a
// value. A person's dev lists the team defaults with the person's overrides in their place.
func secretListing(ctx context.Context, q querier, projectID string, config SecretConfig, personID string) (SecretListing, error) {
	var entries []SecretEntry
	var err error
	if config == ConfigDev {
		entries, err = devEntries(ctx, q, projectID, personID)
	} else {
		entries, err = secretEntries(ctx, q, projectID, config.stored(), "")
	}
	if err != nil {
		return SecretListing{}, err
	}

	required, err := requiredKeys(ctx, q, projectID, config.requiredList())
	if err != nil {
		return SecretListing{}, err
	}

	present := make(map[string]bool, len(entries))
	for _, entry := range entries {
		present[entry.Name] = true
	}
	missing := []string{}
	for _, name := range required {
		if !present[name] {
			missing = append(missing, name)
		}
	}

	return SecretListing{Secrets: entries, Required: required, Missing: missing}, nil
}

// devEntries merges the team dev defaults with a person's overrides, in name order.
func devEntries(ctx context.Context, q querier, projectID, personID string) ([]SecretEntry, error) {
	defaults, err := secretEntries(ctx, q, projectID, ConfigDevDefaults.stored(), "")
	if err != nil {
		return nil, err
	}
	overrides, err := secretEntries(ctx, q, projectID, ConfigDev.stored(), personID)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]SecretEntry, len(defaults)+len(overrides))
	for _, entry := range defaults {
		entry.Source = SourceDefault
		merged[entry.Name] = entry
	}
	for _, entry := range overrides {
		entry.Source = SourceOverride
		merged[entry.Name] = entry
	}

	entries := make([]SecretEntry, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b SecretEntry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

// secretEntries lists the keys of one stored config that have a value, in name order.
func secretEntries(ctx context.Context, q querier, projectID, stored, ownerID string) ([]SecretEntry, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT s.name, s.current_version, v.actor_id, v.created_at
		FROM secrets s JOIN secret_versions v ON v.secret_id = s.id AND v.version = s.current_version
		WHERE s.project_id = ? AND s.config = ? AND ifnull(s.owner_actor_id, '') = ? AND s.deleted_at IS NULL
		ORDER BY s.name`, projectID, stored, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()

	entries := []SecretEntry{}
	for rows.Next() {
		var entry SecretEntry
		var updated string
		if err := rows.Scan(&entry.Name, &entry.Version, &entry.UpdatedBy, &updated); err != nil {
			return nil, fmt.Errorf("read secret: %w", err)
		}
		if entry.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// requiredKeys lists the required keys of dev, staging, or production in name order.
func requiredKeys(ctx context.Context, q querier, projectID, list string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT name FROM secret_required_keys WHERE project_id = ? AND config = ? ORDER BY name`, projectID, list)
	if err != nil {
		return nil, fmt.Errorf("list required keys: %w", err)
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read required key: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// storedSecret is a key's row with its current version.
type storedSecret struct {
	id      string
	version int
	deleted bool
}

// findSecret returns the row of a key, or ErrNotFound when the key never existed.
func findSecret(ctx context.Context, q querier, projectID, stored, ownerID, name string) (storedSecret, error) {
	var secret storedSecret
	var deleted sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT id, current_version, deleted_at FROM secrets
		WHERE project_id = ? AND config = ? AND ifnull(owner_actor_id, '') = ? AND name = ?`,
		projectID, stored, ownerID, name).Scan(&secret.id, &secret.version, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return storedSecret{}, notFound("secret " + name)
	}
	if err != nil {
		return storedSecret{}, fmt.Errorf("find secret %s: %w", name, err)
	}
	secret.deleted = deleted.Valid
	return secret, nil
}

// secretRecord names where an encrypted value belongs.
type secretRecord struct {
	projectID, stored, owner, name, secretID string
	version                                  int
}

// sealContext binds an encrypted value to its project, config, owner, name, key row, and version,
// so a value moved to another key, config, or version, or a row renamed in the database, does
// not decrypt.
func (r secretRecord) sealContext() []byte {
	return fmt.Appendf(nil, "sparrow-secret-v1\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d",
		r.projectID, r.stored, r.owner, r.name, r.secretID, r.version)
}

// box returns the encryption secrets are stored with, failing when the server set none.
func (s *Store) box() (SecretBox, error) {
	if s.secrets == nil {
		return nil, errors.New("secret encryption is not configured")
	}
	return s.secrets, nil
}

// openVersion decrypts one version of a key. It fails when the version was a delete.
func (s *Store) openVersion(ctx context.Context, q querier, secretID string, version int) (string, error) {
	box, err := s.box()
	if err != nil {
		return "", err
	}

	record := secretRecord{secretID: secretID, version: version}
	var ciphertext []byte
	var keyID sql.NullString
	err = q.QueryRowContext(ctx, `
		SELECT s.project_id, s.config, ifnull(s.owner_actor_id, ''), s.name, v.ciphertext, v.key_id
		FROM secret_versions v JOIN secrets s ON s.id = v.secret_id
		WHERE v.secret_id = ? AND v.version = ?`, secretID, version).
		Scan(&record.projectID, &record.stored, &record.owner, &record.name, &ciphertext, &keyID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", notFound(fmt.Sprintf("version %d", version))
	}
	if err != nil {
		return "", fmt.Errorf("read secret version: %w", err)
	}
	if !keyID.Valid {
		return "", invalid("version", "version %d deleted the key; it has no value", version)
	}

	plaintext, err := box.Open(ciphertext, keyID.String, record.sealContext())
	if err != nil {
		return "", fmt.Errorf("decrypt secret version %d: %w", version, err)
	}
	return string(plaintext), nil
}

// ownerFor returns who owns config's keys for the viewer: the person for their dev, nobody for
// team configs.
func ownerFor(config SecretConfig, viewer secretViewer) string {
	if config.team() {
		return ""
	}
	return viewer.person.ID
}

// RevealSecret returns one key's current value and records that the caller read it. A person's
// dev falls back to the team default when the person has no override.
func (s *Store) RevealSecret(ctx context.Context, by Caller, projectID string, config SecretConfig, name string) (SecretValue, error) {
	var value SecretValue
	err := s.write(ctx, by, func(w *writeTx) error {
		viewer, project, err := w.secretAccess(by, projectID)
		if err != nil {
			return err
		}
		if !viewer.canReadValues(config) {
			return forbidden("reading " + string(config) + " values needs the staging and production permission, in the browser")
		}

		stored, owner, source := config.stored(), ownerFor(config, viewer), SecretSource("")
		if config == ConfigDev {
			source = SourceOverride
		}
		secret, err := findSecret(w.ctx, w.tx, project.ID, stored, owner, name)
		if config == ConfigDev && (errors.Is(err, ErrNotFound) || (err == nil && secret.deleted)) {
			stored, owner, source = ConfigDevDefaults.stored(), "", SourceDefault
			secret, err = findSecret(w.ctx, w.tx, project.ID, stored, owner, name)
		}
		if err != nil {
			return err
		}
		if secret.deleted {
			return notFound("secret " + name)
		}

		plaintext, err := s.openVersion(w.ctx, w.tx, secret.id, secret.version)
		if err != nil {
			return err
		}
		value = SecretValue{Name: name, Value: plaintext, Version: secret.version, Source: source}

		return w.logAccess(project.ID, config, viewer, AccessReveal, []string{name})
	})
	return value, err
}

// DownloadSecrets returns every value of config for the caller and records the download.
func (s *Store) DownloadSecrets(ctx context.Context, by Caller, projectID string, config SecretConfig) (SecretValues, error) {
	var values SecretValues
	err := s.write(ctx, by, func(w *writeTx) error {
		viewer, project, err := w.secretAccess(by, projectID)
		if err != nil {
			return err
		}
		if !viewer.canReadValues(config) {
			return forbidden("downloading " + string(config) + " needs the staging and production permission, in the browser")
		}

		values, err = s.readValues(w.ctx, w.tx, project.ID, config, viewer.person.ID)
		if err != nil {
			return err
		}
		return w.logAccess(project.ID, config, viewer, AccessDownload, values.Names)
	})
	return values, err
}

// readValues decrypts every key of config, merging a person's dev over the team defaults.
func (s *Store) readValues(ctx context.Context, q querier, projectID string, config SecretConfig, personID string) (SecretValues, error) {
	values := SecretValues{Values: map[string]string{}}
	layers := []struct{ stored, owner string }{{config.stored(), ""}}
	if config == ConfigDev {
		layers = []struct{ stored, owner string }{{ConfigDevDefaults.stored(), ""}, {ConfigDev.stored(), personID}}
	}

	for _, layer := range layers {
		rows, err := q.QueryContext(ctx, `
			SELECT id, name, current_version FROM secrets
			WHERE project_id = ? AND config = ? AND ifnull(owner_actor_id, '') = ? AND deleted_at IS NULL`,
			projectID, layer.stored, layer.owner)
		if err != nil {
			return SecretValues{}, fmt.Errorf("list secrets: %w", err)
		}
		var found []storedSecret
		var names []string
		for rows.Next() {
			var row storedSecret
			var name string
			if err := rows.Scan(&row.id, &name, &row.version); err != nil {
				rows.Close()
				return SecretValues{}, fmt.Errorf("read secret: %w", err)
			}
			found, names = append(found, row), append(names, name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return SecretValues{}, fmt.Errorf("list secrets: %w", err)
		}

		for i, row := range found {
			plaintext, err := s.openVersion(ctx, q, row.id, row.version)
			if err != nil {
				return SecretValues{}, fmt.Errorf("%s: %w", names[i], err)
			}
			values.Values[names[i]] = plaintext
		}
	}

	for name := range values.Values {
		values.Names = append(values.Names, name)
	}
	slices.Sort(values.Names)
	return values, nil
}

// secretAccess loads the viewer and the project for a read, checking secrets are turned on.
func (s *Store) secretAccess(ctx context.Context, by Caller, projectID string) (secretViewer, Project, error) {
	viewer, err := newSecretViewer(ctx, s.db, by)
	if err != nil {
		return secretViewer{}, Project{}, err
	}
	project, err := secretsProject(ctx, s.db, projectID)
	if err != nil {
		return secretViewer{}, Project{}, err
	}
	return viewer, project, nil
}

// secretAccess loads the viewer and the project inside a write, checking secrets are turned on.
func (w *writeTx) secretAccess(by Caller, projectID string) (secretViewer, Project, error) {
	viewer, err := newSecretViewer(w.ctx, w.tx, by)
	if err != nil {
		return secretViewer{}, Project{}, err
	}
	project, err := secretsProject(w.ctx, w.tx, projectID)
	if err != nil {
		return secretViewer{}, Project{}, err
	}
	return viewer, project, nil
}

// logAccess records that the viewer read names from config.
func (w *writeTx) logAccess(projectID string, config SecretConfig, viewer secretViewer, action AccessAction, names []string) error {
	return insertAccessLog(w.ctx, w.tx, projectID, config, ownerFor(config, viewer), viewer.actor.ID, "", action, names, w.timestamp())
}

// insertAccessLog records that a person, agent, or CI token read names from config.
func insertAccessLog(ctx context.Context, tx *sql.Tx, projectID string, config SecretConfig, ownerID, actorID, tokenID string,
	action AccessAction, names []string, stamp string) error {
	encoded, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("encode accessed names: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO secret_access_log (id, project_id, config, owner_actor_id, actor_id, service_token_id, action, names, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), projectID, config.stored(), nullString(ownerID), nullString(actorID), nullString(tokenID), action, string(encoded), stamp)
	if err != nil {
		return fmt.Errorf("record secret access: %w", err)
	}
	return nil
}

// validateSecretChange checks names and values, and that no key is both set and deleted.
func validateSecretChange(change SecretChange) error {
	if len(change.Set)+len(change.Delete) == 0 {
		return invalid("set", "name at least one key to set or delete")
	}
	if len(change.Set)+len(change.Delete) > maxSecretChanges {
		return invalid("set", "change at most %d keys at once", maxSecretChanges)
	}

	for name, value := range change.Set {
		if !secretNamePattern.MatchString(name) {
			return invalid("set", "%q is not a valid name: use capital letters, digits, and underscores, starting with a letter or underscore", name)
		}
		if unsafeNamePattern.MatchString(name) {
			return invalid("set", "%s changes how programs start, so it cannot be a secret", name)
		}
		if len(value) > MaxSecretValue {
			return invalid("set", "%s is longer than %d KiB", name, MaxSecretValue>>10)
		}
		if !utf8.ValidString(value) {
			return invalid("set", "%s is not valid UTF-8 text", name)
		}
	}
	for _, name := range change.Delete {
		if !secretNamePattern.MatchString(name) {
			return invalid("delete", "%q is not a valid name", name)
		}
		if _, set := change.Set[name]; set {
			return invalid("delete", "%s is both set and deleted", name)
		}
	}
	return nil
}

// ChangeSecrets sets and deletes keys of one config together and returns its new listing. Each
// change adds a version, so earlier values can be restored. A person's dev changes only their own
// overrides.
func (s *Store) ChangeSecrets(ctx context.Context, by Caller, projectID string, config SecretConfig, change SecretChange) (SecretListing, error) {
	if err := validateSecretChange(change); err != nil {
		return SecretListing{}, err
	}
	box, err := s.box()
	if err != nil {
		return SecretListing{}, err
	}

	var listing SecretListing
	err = s.write(ctx, by, func(w *writeTx) error {
		viewer, project, err := w.secretAccess(by, projectID)
		if err != nil {
			return err
		}
		if err := viewer.checkWrite(config); err != nil {
			return err
		}
		owner := ownerFor(config, viewer)

		// Set every key, keeping names in order so events read the same each time.
		var added, changed, removed []string
		names := make([]string, 0, len(change.Set))
		for name := range change.Set {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			isNew, err := w.putSecretVersion(box, project.ID, config.stored(), owner, name, change.Set[name])
			if err != nil {
				return err
			}
			if isNew {
				added = append(added, name)
			} else {
				changed = append(changed, name)
			}
		}

		// Deleting a key that has no value is refused, so a typo is not silently ignored. In a
		// person's dev, deleting means going back to the team default, so a key that already uses
		// the default is left as it is.
		deletes := slices.Clone(change.Delete)
		slices.Sort(deletes)
		for _, name := range slices.Compact(deletes) {
			err := w.deleteSecret(project.ID, config.stored(), owner, name)
			if errors.Is(err, ErrNotFound) && config == ConfigDev && w.hasDefault(project.ID, name) {
				continue
			}
			if err != nil {
				return err
			}
			removed = append(removed, name)
		}

		if err := w.secretEvent(project, config, viewer, "changed", map[string]any{
			"added": nonNil(added), "changed": nonNil(changed), "removed": nonNil(removed),
		}); err != nil {
			return err
		}

		listing, err = secretListing(w.ctx, w.tx, project.ID, config, viewer.person.ID)
		return err
	})
	return listing, err
}

// nonNil returns names, or an empty list for nil, so events record [] rather than null.
func nonNil(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// hasDefault reports whether the team dev defaults have a value for name.
func (w *writeTx) hasDefault(projectID, name string) bool {
	secret, err := findSecret(w.ctx, w.tx, projectID, ConfigDevDefaults.stored(), "", name)
	return err == nil && !secret.deleted
}

// putSecretVersion stores value as a key's next version, adding the key the first time. It
// reports whether the key had no value before.
func (w *writeTx) putSecretVersion(box SecretBox, projectID, stored, owner, name, value string) (bool, error) {
	secret, err := findSecret(w.ctx, w.tx, projectID, stored, owner, name)
	isNew := errors.Is(err, ErrNotFound) || (err == nil && secret.deleted)
	switch {
	case errors.Is(err, ErrNotFound):
		secret = storedSecret{id: newID(), version: 1}
		_, err = w.exec(`
			INSERT INTO secrets (id, project_id, config, owner_actor_id, name, current_version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
			secret.id, projectID, stored, nullString(owner), name, w.timestamp(), w.timestamp())
		if err != nil {
			return false, fmt.Errorf("add secret %s: %w", name, err)
		}
	case err != nil:
		return false, err
	default:
		secret.version++
		_, err = w.exec(`UPDATE secrets SET current_version = ?, deleted_at = NULL, updated_at = ? WHERE id = ?`,
			secret.version, w.timestamp(), secret.id)
		if err != nil {
			return false, fmt.Errorf("update secret %s: %w", name, err)
		}
	}
	record := secretRecord{projectID: projectID, stored: stored, owner: owner, name: name, secretID: secret.id, version: secret.version}
	ciphertext, err := box.Seal([]byte(value), record.sealContext())
	if err != nil {
		return false, fmt.Errorf("encrypt %s: %w", name, err)
	}
	_, err = w.exec(`
		INSERT INTO secret_versions (secret_id, version, ciphertext, key_id, actor_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, secret.id, secret.version, ciphertext, box.KeyID(), w.caller.ID, w.timestamp())
	if err != nil {
		return false, fmt.Errorf("store secret %s: %w", name, err)
	}
	return isNew, nil
}

// deleteSecret records a key's deletion as its next version, keeping its history.
func (w *writeTx) deleteSecret(projectID, stored, owner, name string) error {
	secret, err := findSecret(w.ctx, w.tx, projectID, stored, owner, name)
	if err != nil {
		return err
	}
	if secret.deleted {
		return notFound("secret " + name)
	}

	secret.version++
	_, err = w.exec(`UPDATE secrets SET current_version = ?1, deleted_at = ?2, updated_at = ?2 WHERE id = ?3`,
		secret.version, w.timestamp(), secret.id)
	if err != nil {
		return fmt.Errorf("delete secret %s: %w", name, err)
	}
	_, err = w.exec(`INSERT INTO secret_versions (secret_id, version, actor_id, created_at) VALUES (?, ?, ?, ?)`,
		secret.id, secret.version, w.caller.ID, w.timestamp())
	if err != nil {
		return fmt.Errorf("record deletion of %s: %w", name, err)
	}
	return nil
}

// secretEvent records a change to config without any value. A team config's change goes in the
// project's history. A person's dev change is recorded on the person without a project, and
// only that person sees it, since nobody else may see their overrides.
func (w *writeTx) secretEvent(project Project, config SecretConfig, viewer secretViewer, action string, payload map[string]any) error {
	payload["config"] = config
	if config.team() {
		return w.event(project.ID, TargetProject, project.ID, "secret."+action, payload)
	}
	payload["project_id"], payload["project_key"] = project.ID, project.Key
	return w.event("", TargetActor, viewer.person.ID, DevSecretEventPrefix+action, payload)
}

// DevSecretEventPrefix starts the kinds of events about a person's dev secrets, which only that
// person sees.
const DevSecretEventPrefix = "dev_secret."

// SecretHistory lists the versions of a key, newest first. For a person's dev it lists the
// person's own overrides.
func (s *Store) SecretHistory(ctx context.Context, by Caller, projectID string, config SecretConfig, name string) ([]SecretVersion, error) {
	viewer, project, err := s.secretAccess(ctx, by, projectID)
	if err != nil {
		return nil, err
	}
	if config.sensitive() && !viewer.canManage() {
		return nil, forbidden("the history of " + string(config) + " needs the staging and production permission, in the browser")
	}

	secret, err := findSecret(ctx, s.db, project.ID, config.stored(), ownerFor(config, viewer), name)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT version, ciphertext IS NULL, actor_id, created_at FROM secret_versions
		WHERE secret_id = ? ORDER BY version DESC`, secret.id)
	if err != nil {
		return nil, fmt.Errorf("list secret versions: %w", err)
	}
	defer rows.Close()

	versions := []SecretVersion{}
	for rows.Next() {
		var version SecretVersion
		var created string
		if err := rows.Scan(&version.Version, &version.Deleted, &version.ActorID, &created); err != nil {
			return nil, fmt.Errorf("read secret version: %w", err)
		}
		if version.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

// RestoreSecret makes an earlier version of a key current again, as a new version. Restoring a
// version that deleted the key deletes it again.
func (s *Store) RestoreSecret(ctx context.Context, by Caller, projectID string, config SecretConfig, name string, version int) (SecretListing, error) {
	box, err := s.box()
	if err != nil {
		return SecretListing{}, err
	}

	var listing SecretListing
	err = s.write(ctx, by, func(w *writeTx) error {
		viewer, project, err := w.secretAccess(by, projectID)
		if err != nil {
			return err
		}
		if err := viewer.checkWrite(config); err != nil {
			return err
		}
		owner := ownerFor(config, viewer)

		secret, err := findSecret(w.ctx, w.tx, project.ID, config.stored(), owner, name)
		if err != nil {
			return err
		}
		if version < 1 || version >= secret.version {
			return invalid("version", "must be an earlier version, from 1 to %d", secret.version-1)
		}

		var deleted bool
		if err := w.tx.QueryRowContext(w.ctx, `SELECT ciphertext IS NULL FROM secret_versions WHERE secret_id = ? AND version = ?`,
			secret.id, version).Scan(&deleted); err != nil {
			return fmt.Errorf("read secret version: %w", err)
		}
		switch {
		case deleted && secret.deleted:
			return invalid("version", "%s is already deleted", name)
		case deleted:
			err = w.deleteSecret(project.ID, config.stored(), owner, name)
		default:
			var value string
			value, err = s.openVersion(w.ctx, w.tx, secret.id, version)
			if err == nil {
				_, err = w.putSecretVersion(box, project.ID, config.stored(), owner, name, value)
			}
		}
		if err != nil {
			return err
		}

		if err := w.secretEvent(project, config, viewer, "restored", map[string]any{"name": name, "version": version}); err != nil {
			return err
		}
		listing, err = secretListing(w.ctx, w.tx, project.ID, config, viewer.person.ID)
		return err
	})
	return listing, err
}

// SetRequiredKeys replaces the required keys of dev, staging, or production and returns that
// config's listing. The team dev defaults share dev's list.
func (s *Store) SetRequiredKeys(ctx context.Context, by Caller, projectID string, config SecretConfig, names []string) (SecretListing, error) {
	if config == ConfigDevDefaults {
		return SecretListing{}, invalid("config", "the team dev defaults use dev's required keys")
	}
	unique := slices.Clone(names)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	if len(unique) > maxSecretChanges {
		return SecretListing{}, invalid("names", "list at most %d keys", maxSecretChanges)
	}
	for _, name := range unique {
		if !secretNamePattern.MatchString(name) {
			return SecretListing{}, invalid("names", "%q is not a valid name", name)
		}
	}

	var listing SecretListing
	err := s.write(ctx, by, func(w *writeTx) error {
		viewer, project, err := w.secretAccess(by, projectID)
		if err != nil {
			return err
		}
		if !viewer.canManage() {
			return forbidden("changing required keys needs the staging and production permission, in the browser")
		}

		if _, err := w.exec(`DELETE FROM secret_required_keys WHERE project_id = ? AND config = ?`, project.ID, string(config)); err != nil {
			return fmt.Errorf("replace required keys: %w", err)
		}
		for _, name := range unique {
			_, err := w.exec(`INSERT INTO secret_required_keys (project_id, config, name, created_at) VALUES (?, ?, ?, ?)`,
				project.ID, string(config), name, w.timestamp())
			if err != nil {
				return fmt.Errorf("add required key %s: %w", name, err)
			}
		}

		err = w.event(project.ID, TargetProject, project.ID, "secret.required_keys_changed",
			map[string]any{"config": config, "names": nonNil(unique)})
		if err != nil {
			return err
		}
		listing, err = secretListing(w.ctx, w.tx, project.ID, config, viewer.person.ID)
		return err
	})
	return listing, err
}

// AccessLog lists who read values of config, newest first. Pass the last Sequence of a page as
// beforeSequence for the next, or 0 for the newest. Owners see the team configs' log; each person
// sees the log of their own dev.
func (s *Store) AccessLog(ctx context.Context, by Caller, projectID string, config SecretConfig, beforeSequence int64, limit int) ([]AccessLogEntry, error) {
	limit, err := pageLimit(limit)
	if err != nil {
		return nil, err
	}
	viewer, project, err := s.secretAccess(ctx, by, projectID)
	if err != nil {
		return nil, err
	}
	if viewer.actor.Kind != ActorHuman || (config.team() && !viewer.actor.IsOwner()) {
		return nil, forbidden("only owners see who read team secrets; everyone sees their own dev's log")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, id, action, names, ifnull(actor_id, ''), ifnull(service_token_id, ''), created_at
		FROM secret_access_log
		WHERE project_id = ? AND config = ? AND ifnull(owner_actor_id, '') = ? AND (?4 = 0 OR sequence < ?4)
		ORDER BY sequence DESC LIMIT ?`,
		project.ID, config.stored(), ownerFor(config, viewer), beforeSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list access log: %w", err)
	}
	defer rows.Close()

	entries := []AccessLogEntry{}
	for rows.Next() {
		entry := AccessLogEntry{Config: config}
		var names, created string
		if err := rows.Scan(&entry.Sequence, &entry.ID, &entry.Action, &names, &entry.ActorID, &entry.ServiceTokenID, &created); err != nil {
			return nil, fmt.Errorf("read access log: %w", err)
		}
		if err := json.Unmarshal([]byte(names), &entry.Names); err != nil {
			return nil, fmt.Errorf("read accessed names: %w", err)
		}
		if entry.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
