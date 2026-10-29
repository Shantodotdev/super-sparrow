package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const projectColumns = `id, key, name, next_issue_number, created_at, updated_at`

// NewProject describes a project to create.
type NewProject struct {
	// Key prefixes the project's issue keys, such as WP in WP-142: 2 to 10 capital letters and
	// digits, starting with a letter.
	Key  string
	Name string
}

// CreateProject creates a project. Only a person can create projects.
func (s *Store) CreateProject(ctx context.Context, by Caller, input NewProject) (Project, error) {
	key := strings.ToUpper(strings.TrimSpace(input.Key))
	if !projectKeyPattern.MatchString(key) {
		return Project{}, invalid("key", "must be 2 to 10 capital letters and digits, starting with a letter")
	}
	name, err := requiredText("name", input.Name, maxNameLength)
	if err != nil {
		return Project{}, err
	}

	project := Project{ID: newID(), Key: key, Name: name, NextIssueNumber: 1}
	err = s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("create projects"); err != nil {
			return err
		}

		project.CreatedAt, project.UpdatedAt = w.now, w.now
		_, err := w.exec(`INSERT INTO projects (id, key, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			project.ID, key, name, w.timestamp(), w.timestamp())
		if err != nil {
			return mapUniqueViolation(err, "a project with key "+key+" already exists", "create project")
		}

		return w.event(project.ID, TargetProject, project.ID, "project.created", map[string]any{"key": key, "name": name})
	})

	return project, err
}

// RenameProject changes a project's name. Its key never changes, so issue keys stay stable.
func (s *Store) RenameProject(ctx context.Context, by Caller, projectID, name string) error {
	name, err := requiredText("name", name, maxNameLength)
	if err != nil {
		return err
	}

	return s.write(ctx, by, func(w *writeTx) error {
		project, err := getProject(w.ctx, w.tx, `id = ?`, projectID)
		if err != nil {
			return err
		}
		if project.Name == name {
			return nil
		}

		if _, err := w.exec(`UPDATE projects SET name = ?, updated_at = ? WHERE id = ?`, name, w.timestamp(), project.ID); err != nil {
			return fmt.Errorf("rename project: %w", err)
		}

		return w.event(project.ID, TargetProject, project.ID, "project.renamed",
			map[string]any{"from": project.Name, "to": name})
	})
}

// DeleteProject deletes a project with all of its milestones, labels, issues, reviews, and
// comments. Its events stay as history. Only an owner can delete projects.
func (s *Store) DeleteProject(ctx context.Context, by Caller, projectID string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireOwner("delete projects"); err != nil {
			return err
		}

		project, err := getProject(w.ctx, w.tx, `id = ?`, projectID)
		if err != nil {
			return err
		}

		// Issues refer to each other through parent and follow-up links, so clear those first
		// and let the cascades remove the rest in any order.
		cleanup := []string{
			`UPDATE findings SET duplicate_of_finding_id = NULL, follow_up_issue_id = NULL
			 WHERE review_round_id IN (SELECT r.id FROM review_rounds r JOIN issues i ON i.id = r.issue_id WHERE i.project_id = ?)`,
			`UPDATE issues SET parent_issue_id = NULL WHERE project_id = ?`,
			`DELETE FROM projects WHERE id = ?`,
		}
		for _, statement := range cleanup {
			if _, err := w.exec(statement, project.ID); err != nil {
				return fmt.Errorf("delete project: %w", err)
			}
		}

		return w.event(project.ID, TargetProject, project.ID, "project.deleted",
			map[string]any{"key": project.Key, "name": project.Name})
	})
}

// GetProject returns the project with the given ID.
func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	return getProject(ctx, s.db, `id = ?`, id)
}

// GetProjectByKey returns the project with the given key, matched without regard to case.
func (s *Store) GetProjectByKey(ctx context.Context, key string) (Project, error) {
	return getProject(ctx, s.db, `key = ?`, strings.ToUpper(strings.TrimSpace(key)))
}

// ListProjects returns every project in key order.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func getProject(ctx context.Context, q querier, where string, arg any) (Project, error) {
	return scanProject(q.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE `+where, arg))
}

func scanProject(row scanner) (Project, error) {
	var project Project
	var created, updated string

	err := row.Scan(&project.ID, &project.Key, &project.Name, &project.NextIssueNumber, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, notFound("project")
	}
	if err != nil {
		return Project{}, fmt.Errorf("read project: %w", err)
	}

	if project.CreatedAt, project.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Project{}, err
	}
	return project, nil
}

// NewMilestone describes a milestone to add at the end of a project's milestones.
type NewMilestone struct {
	ProjectID string
	Name      string
	Status    MilestoneStatus
}

// MilestoneUpdate lists the milestone fields to change.
type MilestoneUpdate struct {
	Name     Field[string]
	Status   Field[MilestoneStatus]
	Position Field[int]
}

const milestoneColumns = `id, project_id, name, position, status, created_at, updated_at`

// CreateMilestone adds a milestone after the project's existing milestones. Only a person can
// create milestones.
func (s *Store) CreateMilestone(ctx context.Context, by Caller, input NewMilestone) (Milestone, error) {
	name, err := requiredText("name", input.Name, maxNameLength)
	if err != nil {
		return Milestone{}, err
	}
	if !input.Status.valid() {
		return Milestone{}, invalid("status", "must be planned, active, or done")
	}

	milestone := Milestone{ID: newID(), ProjectID: input.ProjectID, Name: name, Status: input.Status}
	err = s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("create milestones"); err != nil {
			return err
		}
		if _, err := getProject(w.ctx, w.tx, `id = ?`, input.ProjectID); err != nil {
			return err
		}

		err := w.tx.QueryRowContext(w.ctx,
			`SELECT coalesce(max(position) + 1, 0) FROM milestones WHERE project_id = ?`, input.ProjectID).Scan(&milestone.Position)
		if err != nil {
			return fmt.Errorf("find milestone position: %w", err)
		}

		milestone.CreatedAt, milestone.UpdatedAt = w.now, w.now
		_, err = w.exec(`
			INSERT INTO milestones (id, project_id, name, position, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			milestone.ID, input.ProjectID, name, milestone.Position, input.Status, w.timestamp(), w.timestamp())
		if err != nil {
			return mapUniqueViolation(err, "this project already has a milestone named "+name, "create milestone")
		}

		return w.event(input.ProjectID, TargetMilestone, milestone.ID, "milestone.created",
			map[string]any{"name": name, "status": input.Status, "position": milestone.Position})
	})

	return milestone, err
}

// UpdateMilestone changes a milestone's name, status, or position and returns it. Only a person
// can change milestones.
func (s *Store) UpdateMilestone(ctx context.Context, by Caller, milestoneID string, update MilestoneUpdate) (Milestone, error) {
	if update.Name.Set {
		name, err := requiredText("name", update.Name.Value, maxNameLength)
		if err != nil {
			return Milestone{}, err
		}
		update.Name.Value = name
	}
	if update.Status.Set && !update.Status.Value.valid() {
		return Milestone{}, invalid("status", "must be planned, active, or done")
	}
	if update.Position.Set && update.Position.Value < 0 {
		return Milestone{}, invalid("position", "must not be negative")
	}

	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("change milestones"); err != nil {
			return err
		}

		current, err := scanMilestone(w.tx.QueryRowContext(w.ctx, `SELECT `+milestoneColumns+` FROM milestones WHERE id = ?`, milestoneID))
		if err != nil {
			return err
		}

		next := current
		changes := map[string]any{}
		if update.Name.Set && update.Name.Value != current.Name {
			next.Name = update.Name.Value
			changes["name"] = map[string]any{"from": current.Name, "to": next.Name}
		}
		if update.Status.Set && update.Status.Value != current.Status {
			next.Status = update.Status.Value
			changes["status"] = map[string]any{"from": current.Status, "to": next.Status}
		}
		if update.Position.Set && update.Position.Value != current.Position {
			next.Position = update.Position.Value
			changes["position"] = map[string]any{"from": current.Position, "to": next.Position}
		}
		if len(changes) == 0 {
			return nil
		}

		_, err = w.exec(`UPDATE milestones SET name = ?, status = ?, position = ?, updated_at = ? WHERE id = ?`,
			next.Name, next.Status, next.Position, w.timestamp(), current.ID)
		if err != nil {
			return mapUniqueViolation(err, "this project already has a milestone named "+next.Name, "update milestone")
		}

		return w.event(current.ProjectID, TargetMilestone, current.ID, "milestone.updated", changes)
	})
	if err != nil {
		return Milestone{}, err
	}
	return scanMilestone(s.db.QueryRowContext(ctx, `SELECT `+milestoneColumns+` FROM milestones WHERE id = ?`, milestoneID))
}

// ListMilestones returns a project's milestones in position order.
func (s *Store) ListMilestones(ctx context.Context, projectID string) ([]Milestone, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+milestoneColumns+` FROM milestones WHERE project_id = ? ORDER BY position, name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list milestones: %w", err)
	}
	defer rows.Close()

	var milestones []Milestone
	for rows.Next() {
		milestone, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		milestones = append(milestones, milestone)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list milestones: %w", err)
	}
	return milestones, nil
}

func scanMilestone(row scanner) (Milestone, error) {
	var milestone Milestone
	var created, updated string

	err := row.Scan(&milestone.ID, &milestone.ProjectID, &milestone.Name, &milestone.Position, &milestone.Status, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Milestone{}, notFound("milestone")
	}
	if err != nil {
		return Milestone{}, fmt.Errorf("read milestone: %w", err)
	}

	if milestone.CreatedAt, milestone.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Milestone{}, err
	}
	return milestone, nil
}

// CreateLabel adds a label to a project. People and agents can create labels.
func (s *Store) CreateLabel(ctx context.Context, by Caller, projectID, name string) (Label, error) {
	name, err := requiredText("name", name, maxLabelLength)
	if err != nil {
		return Label{}, err
	}

	label := Label{ID: newID(), ProjectID: projectID, Name: name}
	err = s.write(ctx, by, func(w *writeTx) error {
		if _, err := getProject(w.ctx, w.tx, `id = ?`, projectID); err != nil {
			return err
		}

		label.CreatedAt, label.UpdatedAt = w.now, w.now
		_, err := w.exec(`INSERT INTO labels (id, project_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			label.ID, projectID, name, w.timestamp(), w.timestamp())
		if err != nil {
			return mapUniqueViolation(err, "this project already has a label named "+name, "create label")
		}

		return w.event(projectID, TargetLabel, label.ID, "label.created", map[string]any{"name": name})
	})

	return label, err
}

// ListLabels returns a project's labels in name order.
func (s *Store) ListLabels(ctx context.Context, projectID string) ([]Label, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, created_at, updated_at FROM labels WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()

	var labels []Label
	for rows.Next() {
		var label Label
		var created, updated string
		if err := rows.Scan(&label.ID, &label.ProjectID, &label.Name, &created, &updated); err != nil {
			return nil, fmt.Errorf("read label: %w", err)
		}
		if label.CreatedAt, label.UpdatedAt, err = timestamps(created, updated); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	return labels, nil
}
