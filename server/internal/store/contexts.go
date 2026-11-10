package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// contextKeyPattern matches a context key such as WP-C3, without regard to case.
var contextKeyPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{1,9})-[Cc]([1-9][0-9]{0,8})$`)

// Context is a note kept outside the issue tracker: inspiration, background, and screenshots a
// person or agent wants to keep and hand to others by its key, such as WP-C3.
type Context struct {
	ID          string
	ProjectID   string
	ProjectKey  string
	ProjectSlug string
	Number      int
	Title       string
	Body        string
	// BodyRevision goes up with every change to Body. An update that edits the body names the
	// revision it started from, so no edit silently undoes another.
	BodyRevision int
	// IssueID is the issue the context informs, or "" for none. IssueKey is read with it.
	IssueID     string
	IssueKey    string
	CreatedByID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Key returns the context's key, such as WP-C3.
func (c Context) Key() string { return fmt.Sprintf("%s-C%d", c.ProjectKey, c.Number) }

// NewContext holds the fields of a new context.
type NewContext struct {
	ProjectID string
	Title     string
	Body      string
	IssueID   string
}

// ContextUpdate changes only the supplied fields. Body edits require their starting revision, and
// an empty IssueID unlinks the issue.
type ContextUpdate struct {
	Title        Field[string]
	Body         Field[string]
	BodyRevision int
	IssueID      Field[string]
}

// ContextFilter selects a project's contexts, newest first. IssueID narrows them to the contexts
// linked to one issue, and Search matches the title or key without regard to case.
type ContextFilter struct {
	ProjectID string
	IssueID   string
	Search    string
	Limit     int
	Offset    int
}

const contextSelect = `SELECT c.id, c.project_id, p.key, p.slug, c.number, c.title, c.body, c.body_revision,
	c.issue_id, CASE WHEN i.id IS NULL THEN NULL ELSE p.key || '-' || i.number END,
	c.created_by_actor_id, c.created_at, c.updated_at
	FROM contexts c JOIN projects p ON p.id = c.project_id LEFT JOIN issues i ON i.id = c.issue_id`

// CreateContext adds a context to a project under the next context number.
func (s *Store) CreateContext(ctx context.Context, by Caller, input NewContext) (Context, error) {
	title, err := requiredText("title", input.Title, maxTitleLength)
	if err != nil {
		return Context{}, err
	}
	body, err := optionalText("body", input.Body, maxTextLength)
	if err != nil {
		return Context{}, err
	}

	var created Context
	err = s.write(ctx, by, func(w *writeTx) error {
		project, err := getProject(w.ctx, w.tx, `id = ?`, input.ProjectID)
		if err != nil {
			return err
		}
		if err := checkContextIssue(w, project.ID, input.IssueID); err != nil {
			return err
		}

		var number int
		if err := w.tx.QueryRowContext(w.ctx, `UPDATE projects SET next_context_number = next_context_number + 1
			WHERE id = ? RETURNING next_context_number - 1`, project.ID).Scan(&number); err != nil {
			return fmt.Errorf("take context number: %w", err)
		}

		id := newID()
		_, err = w.exec(`INSERT INTO contexts (id, project_id, number, title, body, body_revision, issue_id,
			created_by_actor_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
			id, project.ID, number, title, body, nullString(input.IssueID), w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("create context: %w", err)
		}
		if created, err = getContext(w.ctx, w.tx, `c.id = ?`, id); err != nil {
			return err
		}

		return w.event(project.ID, TargetProject, project.ID, "context.created", map[string]any{
			"context_id": id, "number": number, "title": title, "issue_id": input.IssueID,
		})
	})
	return created, err
}

// UpdateContext edits a context, refusing a body edit that started from an older revision.
func (s *Store) UpdateContext(ctx context.Context, by Caller, contextID string, update ContextUpdate) (Context, error) {
	var err error
	if update.Title.Set {
		if update.Title.Value, err = requiredText("title", update.Title.Value, maxTitleLength); err != nil {
			return Context{}, err
		}
	}
	if update.Body.Set {
		if update.Body.Value, err = optionalText("body", update.Body.Value, maxTextLength); err != nil {
			return Context{}, err
		}
		if update.BodyRevision < 1 {
			return Context{}, invalid("body_revision", "is required with body")
		}
	}

	var current Context
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err = getContext(w.ctx, w.tx, `c.id = ?`, contextID)
		if err != nil {
			return err
		}

		changes := map[string]any{}
		applyChange(update.Title, &current.Title, "title", changes)
		if update.Body.Set && update.Body.Value != current.Body {
			if update.BodyRevision != current.BodyRevision {
				return Refuse(ErrConflict, "context %s changed since revision %d; reload it before saving", current.Key(), update.BodyRevision)
			}
			current.Body = update.Body.Value
			current.BodyRevision++
			changes["body"] = map[string]any{"revision": current.BodyRevision}
		}
		if update.IssueID.Set && update.IssueID.Value != current.IssueID {
			if err := checkContextIssue(w, current.ProjectID, update.IssueID.Value); err != nil {
				return err
			}
			changes["issue_id"] = map[string]any{"from": current.IssueID, "to": update.IssueID.Value}
			current.IssueID = update.IssueID.Value
		}
		if len(changes) == 0 {
			return nil
		}

		_, err = w.exec(`UPDATE contexts SET title = ?, body = ?, body_revision = ?, issue_id = ?, updated_at = ? WHERE id = ?`,
			current.Title, current.Body, current.BodyRevision, nullString(current.IssueID), w.timestamp(), current.ID)
		if err != nil {
			return fmt.Errorf("update context %s: %w", current.Key(), err)
		}
		if current, err = getContext(w.ctx, w.tx, `c.id = ?`, current.ID); err != nil {
			return err
		}

		return w.event(current.ProjectID, TargetProject, current.ProjectID, "context.updated", map[string]any{
			"context_id": current.ID, "number": current.Number, "changes": changes,
		})
	})
	return current, err
}

// DeleteContext removes a context. Only a person can delete one. Images it showed stay, since
// other markdown may refer to them.
func (s *Store) DeleteContext(ctx context.Context, by Caller, contextID string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("delete contexts"); err != nil {
			return err
		}
		current, err := getContext(w.ctx, w.tx, `c.id = ?`, contextID)
		if err != nil {
			return err
		}

		if _, err := w.exec(`DELETE FROM contexts WHERE id = ?`, current.ID); err != nil {
			return fmt.Errorf("delete context %s: %w", current.Key(), err)
		}
		return w.event(current.ProjectID, TargetProject, current.ProjectID, "context.deleted", map[string]any{
			"context_id": current.ID, "number": current.Number, "title": current.Title,
		})
	})
}

// GetContextByKey returns the context with a key such as WP-C3, matched without regard to case.
func (s *Store) GetContextByKey(ctx context.Context, key string) (Context, error) {
	match := contextKeyPattern.FindStringSubmatch(strings.TrimSpace(key))
	if match == nil {
		return Context{}, invalid("key", "must look like WP-C3")
	}

	number, _ := strconv.Atoi(match[2])
	return getContext(ctx, s.db, `p.key = ? AND c.number = ?`, strings.ToUpper(match[1]), number)
}

// ListContexts returns a page of a project's contexts, newest first.
func (s *Store) ListContexts(ctx context.Context, filter ContextFilter) ([]Context, error) {
	limit, err := pageLimit(filter.Limit)
	if err != nil {
		return nil, err
	}
	if filter.Offset < 0 {
		return nil, invalid("offset", "must not be negative")
	}

	where := []string{`c.project_id = ?`}
	args := []any{filter.ProjectID}
	if filter.IssueID != "" {
		where = append(where, `c.issue_id = ?`)
		args = append(args, filter.IssueID)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		where = append(where, `(instr(lower(c.title), lower(?)) > 0 OR lower(p.key || '-c' || c.number) = lower(?))`)
		args = append(args, search, search)
	}
	args = append(args, limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, contextSelect+` WHERE `+strings.Join(where, " AND ")+
		` ORDER BY c.number DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list contexts: %w", err)
	}
	defer rows.Close()

	contexts := []Context{}
	for rows.Next() {
		context, err := scanContext(rows)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, context)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list contexts: %w", err)
	}
	return contexts, nil
}

// checkContextIssue refuses an issue outside the context's project. An empty ID links none.
func checkContextIssue(w *writeTx, projectID, issueID string) error {
	if issueID == "" {
		return nil
	}
	issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
	if errors.Is(err, ErrNotFound) || (err == nil && issue.ProjectID != projectID) {
		return invalid("issue_id", "must be an issue in the context's project")
	}
	return err
}

func getContext(ctx context.Context, q querier, where string, args ...any) (Context, error) {
	return scanContext(q.QueryRowContext(ctx, contextSelect+` WHERE `+where, args...))
}

func scanContext(row scanner) (Context, error) {
	var c Context
	var issueID, issueKey sql.NullString
	var created, updated string
	err := row.Scan(&c.ID, &c.ProjectID, &c.ProjectKey, &c.ProjectSlug, &c.Number, &c.Title, &c.Body, &c.BodyRevision,
		&issueID, &issueKey, &c.CreatedByID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Context{}, notFound("context")
	}
	if err != nil {
		return Context{}, fmt.Errorf("read context: %w", err)
	}

	c.IssueID, c.IssueKey = issueID.String, issueKey.String
	c.CreatedAt, c.UpdatedAt, err = timestamps(created, updated)
	return c, err
}
