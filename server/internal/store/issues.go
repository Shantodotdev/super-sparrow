package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/checklist"
)

const issueColumns = `i.id, i.project_id, p.key, i.number, i.title, i.plan, i.plan_revision, i.status, i.priority, i.parent_issue_id,
	i.milestone_id, i.assignee_actor_id, i.branch_name, i.pull_request_url,
	(SELECT group_concat(label_id) FROM issue_labels WHERE issue_id = i.id),
	i.created_by_actor_id, i.completed_at, i.created_at, i.updated_at,
	(SELECT count(*) FROM findings f JOIN review_rounds r ON r.id = f.review_round_id
		WHERE r.issue_id = i.id AND f.status IN ('open', 'fix', 'fixed') AND f.duplicate_of_finding_id IS NULL),
	(SELECT count(*) FROM comments c WHERE c.issue_id = i.id)`

const issueFrom = ` FROM issues i JOIN projects p ON p.id = i.project_id `

var (
	issueKeyPattern       = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{1,9})-([1-9][0-9]{0,8})$`)
	branchIssueKeyPattern = regexp.MustCompile(`(?i)(?:^|[/_.-])([a-z][a-z0-9]{1,9})-([0-9]{1,9})(?:$|[/_.-])`)
)

// NewIssue describes an issue to create.
type NewIssue struct {
	ProjectID       string
	Title           string
	Plan            string
	Status          IssueStatus
	Priority        string
	ParentIssueID   string
	MilestoneID     string
	AssigneeActorID string
	BranchName      string
	PullRequestURL  string
	LabelIDs        []string
}

// IssueUpdate lists the issue fields to change. Setting an ID or text field to "" clears it, and
// setting LabelIDs replaces the whole set.
type IssueUpdate struct {
	Title Field[string]
	Plan  Field[string]
	// PlanRevision is the plan revision an edited Plan started from. It is required with Plan, and
	// the update is refused with ErrConflict when the plan has changed since.
	PlanRevision    int
	ParentIssueID   Field[string]
	MilestoneID     Field[string]
	AssigneeActorID Field[string]
	BranchName      Field[string]
	PullRequestURL  Field[string]
	Status          Field[IssueStatus]
	Priority        Field[string]
	LabelIDs        Field[[]string]
}

// IssueFilter selects issues within one project. Results come newest first, a page at a time:
// pass the smallest number of one page as BeforeNumber to get the next.
type IssueFilter struct {
	ProjectID    string
	Statuses     []IssueStatus
	MilestoneID  string
	AssigneeID   string
	BeforeNumber int
	Limit        int
}

// issueFields holds validated issue values, shared by creation and follow-up issues.
type issueFields struct {
	title, plan                     string
	priority                        string
	status                          IssueStatus
	parentID, milestoneID           string
	assigneeID, branch, pullRequest string
	labelIDs                        []string
}

// CreateIssue creates an issue with the project's next number. People and agents can create
// issues.
func (s *Store) CreateIssue(ctx context.Context, by Caller, input NewIssue) (Issue, error) {
	fields, err := validateNewIssue(input)
	if err != nil {
		return Issue{}, err
	}

	var issue Issue
	err = s.write(ctx, by, func(w *writeTx) error {
		project, err := getProject(w.ctx, w.tx, `id = ?`, input.ProjectID)
		if err != nil {
			return err
		}

		issue, err = insertIssue(w, project, fields)
		return err
	})

	return issue, err
}

func validateNewIssue(input NewIssue) (issueFields, error) {
	var fields issueFields
	var err error

	if fields.title, err = requiredText("title", input.Title, maxTitleLength); err != nil {
		return fields, err
	}
	if fields.plan, err = optionalText("plan", input.Plan, maxTextLength); err != nil {
		return fields, err
	}
	if !input.Status.valid() {
		return fields, invalid("status", "must be backlog, todo, in_progress, in_review, done, or canceled")
	}
	if fields.branch, err = optionalBranch(input.BranchName); err != nil {
		return fields, err
	}
	if fields.pullRequest, err = optionalHTTPURL("pull_request_url", input.PullRequestURL); err != nil {
		return fields, err
	}
	if len(input.LabelIDs) > maxLabelsPerIssue {
		return fields, invalid("label_ids", "must have at most %d labels", maxLabelsPerIssue)
	}

	if err := validatePriority(input.Priority); err != nil {
		return fields, err
	}
	fields.priority = input.Priority
	fields.status = input.Status
	fields.parentID, fields.milestoneID, fields.assigneeID = input.ParentIssueID, input.MilestoneID, input.AssigneeActorID
	fields.labelIDs = uniqueSorted(input.LabelIDs)
	return fields, nil
}

// insertIssue takes the project's next issue number, inserts the issue and its labels, and
// records the event.
func insertIssue(w *writeTx, project Project, fields issueFields) (Issue, error) {
	if err := checkIssueReferences(w, project.ID, "", fields.parentID, fields.milestoneID, fields.assigneeID); err != nil {
		return Issue{}, err
	}
	if err := checkLabels(w, project.ID, fields.labelIDs); err != nil {
		return Issue{}, err
	}

	// The transaction holds the write lock, so no other writer can take the same number.
	var number int
	err := w.tx.QueryRowContext(w.ctx, `
		UPDATE projects SET next_issue_number = next_issue_number + 1, updated_at = ?
		WHERE id = ? RETURNING next_issue_number - 1`, w.timestamp(), project.ID).Scan(&number)
	if err != nil {
		return Issue{}, fmt.Errorf("take issue number: %w", err)
	}

	issue := Issue{
		ID: newID(), ProjectID: project.ID, ProjectKey: project.Key, Number: number,
		Title: fields.title, Plan: fields.plan, PlanRevision: 1, Status: fields.status, Priority: fields.priority,
		ParentIssueID: fields.parentID, MilestoneID: fields.milestoneID, AssigneeActorID: fields.assigneeID,
		BranchName: fields.branch, PullRequestURL: fields.pullRequest, LabelIDs: fields.labelIDs,
		CreatedByID: w.caller.ID, CreatedAt: w.now, UpdatedAt: w.now,
	}
	var completedAt sql.NullString
	if issue.Status.closed() {
		completedAt = sql.NullString{String: w.timestamp(), Valid: true}
		issue.CompletedAt = &w.now
	}

	_, err = w.exec(`
		INSERT INTO issues (id, project_id, number, title, plan, status, priority, parent_issue_id, milestone_id,
			assignee_actor_id, branch_name, pull_request_url, created_by_actor_id, completed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.ID, project.ID, number, issue.Title, issue.Plan, issue.Status, issue.Priority, nullString(issue.ParentIssueID),
		nullString(issue.MilestoneID), nullString(issue.AssigneeActorID), nullString(issue.BranchName),
		nullString(issue.PullRequestURL), w.caller.ID, completedAt, w.timestamp(), w.timestamp())
	if err != nil {
		return Issue{}, mapUniqueViolation(err, "another issue in this project already uses branch "+issue.BranchName, "create issue")
	}

	if err := insertIssueLabels(w, issue.ID, issue.LabelIDs); err != nil {
		return Issue{}, err
	}

	err = w.event(project.ID, TargetIssue, issue.ID, "issue.created", map[string]any{
		"key": issue.Key(), "title": issue.Title, "status": issue.Status, "priority": issue.Priority, "parent_issue_id": issue.ParentIssueID,
		"milestone_id": issue.MilestoneID, "assignee_actor_id": issue.AssigneeActorID,
		"branch_name": issue.BranchName, "label_ids": issue.LabelIDs,
	})
	return issue, err
}

// UpdateIssue changes the given fields, status, and labels of an issue in one transaction, with
// one event for each kind of change. People and agents can update issues.
func (s *Store) UpdateIssue(ctx context.Context, by Caller, issueID string, update IssueUpdate) (Issue, error) {
	if err := validateIssueUpdate(&update); err != nil {
		return Issue{}, err
	}

	var issue Issue
	err := s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}

		if err := updateIssueFields(w, current, update); err != nil {
			return err
		}
		if update.Status.Set {
			if err := setIssueStatus(w, current, update.Status.Value); err != nil {
				return err
			}
		}
		if update.LabelIDs.Set {
			if err := setIssueLabels(w, current, update.LabelIDs.Value); err != nil {
				return err
			}
		}

		issue, err = getIssue(w.ctx, w.tx, `i.id = ?`, current.ID)
		return err
	})

	return issue, err
}

// updateIssueFields writes the changed text and reference fields and records them in one event.
func updateIssueFields(w *writeTx, current Issue, update IssueUpdate) error {
	next := current
	changes := map[string]any{}
	applyChange(update.Title, &next.Title, "title", changes)
	applyChange(update.Priority, &next.Priority, "priority", changes)
	applyChange(update.ParentIssueID, &next.ParentIssueID, "parent_issue_id", changes)
	applyChange(update.MilestoneID, &next.MilestoneID, "milestone_id", changes)
	applyChange(update.AssigneeActorID, &next.AssigneeActorID, "assignee_actor_id", changes)
	applyChange(update.BranchName, &next.BranchName, "branch_name", changes)
	applyChange(update.PullRequestURL, &next.PullRequestURL, "pull_request_url", changes)
	// A plan can be long, so its event records only its new revision; the issue holds the text.
	if update.Plan.Set && update.Plan.Value != current.Plan {
		if update.PlanRevision != current.PlanRevision {
			return Refuse(ErrConflict, "%s's plan changed after revision %d and is now at revision %d; load it again, "+
				"reapply your edit, and save it with plan_revision %d", current.Key(), update.PlanRevision, current.PlanRevision, current.PlanRevision)
		}
		next.Plan, next.PlanRevision = update.Plan.Value, current.PlanRevision+1
		changes["plan"] = map[string]any{"revision": next.PlanRevision}
	}
	if len(changes) == 0 {
		return nil
	}

	err := checkIssueReferences(w, current.ProjectID, current.ID, changedOrEmpty(update.ParentIssueID, next.ParentIssueID),
		changedOrEmpty(update.MilestoneID, next.MilestoneID), changedOrEmpty(update.AssigneeActorID, next.AssigneeActorID))
	if err != nil {
		return err
	}

	_, err = w.exec(`
		UPDATE issues SET title = ?, priority = ?, plan = ?, plan_revision = ?, parent_issue_id = ?, milestone_id = ?,
			assignee_actor_id = ?, branch_name = ?, pull_request_url = ?, updated_at = ?
		WHERE id = ?`,
		next.Title, next.Priority, next.Plan, next.PlanRevision, nullString(next.ParentIssueID), nullString(next.MilestoneID),
		nullString(next.AssigneeActorID), nullString(next.BranchName), nullString(next.PullRequestURL),
		w.timestamp(), current.ID)
	if err != nil {
		return mapUniqueViolation(err, "another issue in this project already uses branch "+next.BranchName, "update issue")
	}

	return w.event(current.ProjectID, TargetIssue, current.ID, "issue.updated", changes)
}

func validateIssueUpdate(update *IssueUpdate) error {
	var err error
	if update.Priority.Set {
		if err := validatePriority(update.Priority.Value); err != nil {
			return err
		}
	}
	if update.Title.Set {
		if update.Title.Value, err = requiredText("title", update.Title.Value, maxTitleLength); err != nil {
			return err
		}
	}
	if update.Plan.Set {
		if update.Plan.Value, err = optionalText("plan", update.Plan.Value, maxTextLength); err != nil {
			return err
		}
	}
	if update.BranchName.Set {
		if update.BranchName.Value, err = optionalBranch(update.BranchName.Value); err != nil {
			return err
		}
	}
	if update.PullRequestURL.Set {
		if update.PullRequestURL.Value, err = optionalHTTPURL("pull_request_url", update.PullRequestURL.Value); err != nil {
			return err
		}
	}
	if update.Plan.Set && update.PlanRevision < 1 {
		return invalid("plan_revision", "is required with plan: send the plan_revision the edited plan started from")
	}
	if update.Status.Set && !update.Status.Value.valid() {
		return invalid("status", "must be backlog, todo, in_progress, in_review, done, or canceled")
	}
	if update.LabelIDs.Set {
		if len(update.LabelIDs.Value) > maxLabelsPerIssue {
			return invalid("label_ids", "must have at most %d labels", maxLabelsPerIssue)
		}
		update.LabelIDs.Value = uniqueSorted(update.LabelIDs.Value)
	}
	return nil
}

// applyChange sets *target and records the change when the field is set to a new value.
func applyChange(field Field[string], target *string, name string, changes map[string]any) {
	if field.Set && field.Value != *target {
		changes[name] = map[string]any{"from": *target, "to": field.Value}
		*target = field.Value
	}
}

// changedOrEmpty returns value only when the field is being set, so unchanged references are
// not checked again.
func changedOrEmpty(field Field[string], value string) string {
	if field.Set {
		return value
	}
	return ""
}

// SetIssueStatus moves an issue to a status. Closing it as done or canceled records when; moving
// it back to an open status clears that. People and agents can change an issue's status.
func (s *Store) SetIssueStatus(ctx context.Context, by Caller, issueID string, status IssueStatus) error {
	if !status.valid() {
		return invalid("status", "must be backlog, todo, in_progress, in_review, done, or canceled")
	}

	return s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}
		return setIssueStatus(w, current, status)
	})
}

func setIssueStatus(w *writeTx, current Issue, status IssueStatus) error {
	return setIssueStatusWith(w, current, status, map[string]any{})
}

// setIssueStatusWith moves the issue to status, adding extra to the event's payload, such as why
// it moved.
func setIssueStatusWith(w *writeTx, current Issue, status IssueStatus, extra map[string]any) error {
	if current.Status == status {
		return nil
	}

	completedAt := sql.NullString{}
	if status.closed() {
		completedAt = sql.NullString{String: w.timestamp(), Valid: true}
	}

	_, err := w.exec(`UPDATE issues SET status = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		status, completedAt, w.timestamp(), current.ID)
	if err != nil {
		return fmt.Errorf("change issue status: %w", err)
	}

	extra["from"], extra["to"] = current.Status, status
	return w.event(current.ProjectID, TargetIssue, current.ID, "issue.status_changed", extra)
}

// SetIssueLabels replaces an issue's labels. People and agents can label issues.
func (s *Store) SetIssueLabels(ctx context.Context, by Caller, issueID string, labelIDs []string) error {
	if len(labelIDs) > maxLabelsPerIssue {
		return invalid("label_ids", "must have at most %d labels", maxLabelsPerIssue)
	}

	return s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}
		return setIssueLabels(w, current, uniqueSorted(labelIDs))
	})
}

// setIssueLabels replaces the issue's labels with labelIDs, which must already be unique and sorted.
func setIssueLabels(w *writeTx, current Issue, labelIDs []string) error {
	if slices.Equal(current.LabelIDs, labelIDs) {
		return nil
	}
	if err := checkLabels(w, current.ProjectID, labelIDs); err != nil {
		return err
	}

	if _, err := w.exec(`DELETE FROM issue_labels WHERE issue_id = ?`, current.ID); err != nil {
		return fmt.Errorf("clear issue labels: %w", err)
	}
	if err := insertIssueLabels(w, current.ID, labelIDs); err != nil {
		return err
	}
	if _, err := w.exec(`UPDATE issues SET updated_at = ? WHERE id = ?`, w.timestamp(), current.ID); err != nil {
		return fmt.Errorf("touch issue: %w", err)
	}

	return w.event(current.ProjectID, TargetIssue, current.ID, "issue.labels_changed",
		map[string]any{"from": current.LabelIDs, "to": labelIDs})
}

// UpdatePlanChecklist ticks or unticks checklist items in an issue's plan, keyed by their index
// as checklist.Items numbers them. Each change names the item's text as the caller read it, and
// is refused with ErrConflict when the item at that index has other text because the plan was
// edited since. It changes only those lines, so it never overwrites an edit someone else made to
// the rest of the plan. People and agents can tick items.
func (s *Store) UpdatePlanChecklist(ctx context.Context, by Caller, issueID string, changes map[int]checklist.Change) (Issue, error) {
	if len(changes) == 0 {
		return Issue{}, invalid("items", "must name at least one item")
	}
	for index, change := range changes {
		if index < 0 {
			return Issue{}, invalid("items", "indexes must not be negative")
		}
		if strings.TrimSpace(change.Text) == "" {
			return Issue{}, invalid("items", "each item needs the text it had when you read the plan")
		}
	}

	var issue Issue
	err := s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}

		plan, err := checklist.Set(current.Plan, changes)
		if changed, ok := errors.AsType[*checklist.ItemChangedError](err); ok {
			return Refuse(ErrConflict, "%s's plan changed: %s", current.Key(), changed.Message)
		}
		if err != nil {
			return invalid("items", "%s", err.Error())
		}
		issue = current
		if plan == current.Plan {
			return nil
		}

		_, err = w.exec(`UPDATE issues SET plan = ?, plan_revision = plan_revision + 1, updated_at = ? WHERE id = ?`,
			plan, w.timestamp(), current.ID)
		if err != nil {
			return fmt.Errorf("update plan checklist: %w", err)
		}
		issue.Plan, issue.PlanRevision, issue.UpdatedAt = plan, current.PlanRevision+1, w.now

		items := make([]map[string]any, 0, len(changes))
		for _, item := range checklist.Items(plan) {
			if _, ok := changes[item.Index]; ok {
				items = append(items, map[string]any{"index": item.Index, "text": item.Text, "checked": item.Checked})
			}
		}
		return w.event(current.ProjectID, TargetIssue, current.ID, "issue.checklist_changed",
			map[string]any{"items": items, "plan_revision": issue.PlanRevision})
	})

	return issue, err
}

// checkIssueReferences checks that a parent issue, milestone, and assignee exist and fit the
// issue. An empty ID is not checked. issueID is empty for a new issue.
func checkIssueReferences(w *writeTx, projectID, issueID, parentID, milestoneID, assigneeID string) error {
	if parentID != "" {
		if err := checkParent(w, projectID, issueID, parentID); err != nil {
			return err
		}
	}

	if milestoneID != "" {
		var milestoneProject string
		err := w.tx.QueryRowContext(w.ctx, `SELECT project_id FROM milestones WHERE id = ?`, milestoneID).Scan(&milestoneProject)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && milestoneProject != projectID) {
			return invalid("milestone_id", "must be a milestone of this project")
		}
		if err != nil {
			return fmt.Errorf("check milestone: %w", err)
		}
	}

	if assigneeID != "" {
		assignee, err := getActor(w.ctx, w.tx, assigneeID)
		if errors.Is(err, ErrNotFound) || (err == nil && (assignee.Kind == ActorSystem || !assignee.Active())) {
			return invalid("assignee_actor_id", "must be an active person or agent")
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// checkParent checks that parentID is an issue of the same project and that making it the
// parent would not create a cycle.
func checkParent(w *writeTx, projectID, issueID, parentID string) error {
	if parentID == issueID {
		return invalid("parent_issue_id", "an issue cannot be its own parent")
	}

	// Walk up from the new parent; reaching the issue itself means the link would form a cycle.
	current := parentID
	for depth := 0; current != ""; depth++ {
		if depth > 100 {
			return invalid("parent_issue_id", "the parent chain is too deep")
		}

		var parentProject string
		var next sql.NullString
		err := w.tx.QueryRowContext(w.ctx, `SELECT project_id, parent_issue_id FROM issues WHERE id = ?`, current).Scan(&parentProject, &next)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && parentProject != projectID) {
			return invalid("parent_issue_id", "must be an issue of this project")
		}
		if err != nil {
			return fmt.Errorf("check parent issue: %w", err)
		}
		if issueID != "" && next.String == issueID {
			return invalid("parent_issue_id", "would make the issues each other's parent")
		}

		current = next.String
	}

	return nil
}

func checkLabels(w *writeTx, projectID string, labelIDs []string) error {
	for _, labelID := range labelIDs {
		var labelProject string
		err := w.tx.QueryRowContext(w.ctx, `SELECT project_id FROM labels WHERE id = ?`, labelID).Scan(&labelProject)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && labelProject != projectID) {
			return invalid("label_ids", "must all be labels of this project")
		}
		if err != nil {
			return fmt.Errorf("check label: %w", err)
		}
	}
	return nil
}

func insertIssueLabels(w *writeTx, issueID string, labelIDs []string) error {
	for _, labelID := range labelIDs {
		if _, err := w.exec(`INSERT INTO issue_labels (issue_id, label_id) VALUES (?, ?)`, issueID, labelID); err != nil {
			return fmt.Errorf("label issue: %w", err)
		}
	}
	return nil
}

// GetIssue returns the issue with the given ID.
func (s *Store) GetIssue(ctx context.Context, id string) (Issue, error) {
	return getIssue(ctx, s.db, `i.id = ?`, id)
}

// GetIssueByKey returns the issue with a key such as WP-142, matched without regard to case.
func (s *Store) GetIssueByKey(ctx context.Context, key string) (Issue, error) {
	match := issueKeyPattern.FindStringSubmatch(strings.TrimSpace(key))
	if match == nil {
		return Issue{}, invalid("key", "must look like WP-142")
	}

	number, _ := strconv.Atoi(match[2])
	return getIssueByProjectNumber(ctx, s.db, match[1], number)
}

// FindIssueForBranch returns the issue a branch belongs to: the issue that records this exact
// branch name, or else the issue whose key the name contains, as in feat/wp-142-meta-comments.
func (s *Store) FindIssueForBranch(ctx context.Context, branch string) (Issue, error) {
	return findIssueForBranch(ctx, s.db, branch)
}

func findIssueForBranch(ctx context.Context, q querier, branch string) (Issue, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return Issue{}, invalid("branch", "is required")
	}

	issue, err := getIssue(ctx, q, `i.branch_name = ? ORDER BY i.created_at LIMIT 1`, branch)
	if !errors.Is(err, ErrNotFound) {
		return issue, err
	}

	// Separators are shared between neighbouring keys, so scan from each match's own start.
	for start := 0; start < len(branch); {
		location := branchIssueKeyPattern.FindStringSubmatchIndex(branch[start:])
		if location == nil {
			break
		}

		key, numberText := branch[start+location[2]:start+location[3]], branch[start+location[4]:start+location[5]]
		number, _ := strconv.Atoi(numberText)
		issue, err := getIssueByProjectNumber(ctx, q, key, number)
		if !errors.Is(err, ErrNotFound) {
			return issue, err
		}

		start += location[3]
	}

	return Issue{}, notFound("issue for branch " + branch)
}

// ListIssues returns a page of a project's issues, newest first.
func (s *Store) ListIssues(ctx context.Context, filter IssueFilter) ([]Issue, error) {
	if filter.ProjectID == "" {
		return nil, invalid("project_id", "is required")
	}
	limit, err := pageLimit(filter.Limit)
	if err != nil {
		return nil, err
	}

	where := []string{`i.project_id = ?`}
	args := []any{filter.ProjectID}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for index, status := range filter.Statuses {
			if !status.valid() {
				return nil, invalid("statuses", "contains an unknown status %q", status)
			}
			placeholders[index] = "?"
			args = append(args, status)
		}
		where = append(where, `i.status IN (`+strings.Join(placeholders, ", ")+`)`)
	}
	if filter.MilestoneID != "" {
		where = append(where, `i.milestone_id = ?`)
		args = append(args, filter.MilestoneID)
	}
	if filter.AssigneeID != "" {
		where = append(where, `i.assignee_actor_id = ?`)
		args = append(args, filter.AssigneeID)
	}
	if filter.BeforeNumber > 0 {
		where = append(where, `i.number < ?`)
		args = append(args, filter.BeforeNumber)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+issueColumns+issueFrom+`WHERE `+strings.Join(where, " AND ")+` ORDER BY i.number DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	var issues []Issue
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	return issues, nil
}

func getIssue(ctx context.Context, q querier, where string, args ...any) (Issue, error) {
	return scanIssue(q.QueryRowContext(ctx, `SELECT `+issueColumns+issueFrom+`WHERE `+where, args...))
}

func getIssueByProjectNumber(ctx context.Context, q querier, projectKey string, number int) (Issue, error) {
	return getIssue(ctx, q, `p.key = ? AND i.number = ?`, strings.ToUpper(projectKey), number)
}

func scanIssue(row scanner) (Issue, error) {
	var issue Issue
	var parent, milestone, assignee, branch, pullRequest, labels, completed sql.NullString
	var created, updated string

	err := row.Scan(&issue.ID, &issue.ProjectID, &issue.ProjectKey, &issue.Number, &issue.Title, &issue.Plan,
		&issue.PlanRevision, &issue.Status, &issue.Priority, &parent, &milestone, &assignee, &branch, &pullRequest, &labels,
		&issue.CreatedByID, &completed, &created, &updated, &issue.UnresolvedFindings, &issue.CommentCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, notFound("issue")
	}
	if err != nil {
		return Issue{}, fmt.Errorf("read issue: %w", err)
	}

	issue.ParentIssueID, issue.MilestoneID, issue.AssigneeActorID = parent.String, milestone.String, assignee.String
	issue.BranchName, issue.PullRequestURL = branch.String, pullRequest.String
	issue.LabelIDs = []string{}
	if labels.Valid {
		issue.LabelIDs = uniqueSorted(strings.Split(labels.String, ","))
	}
	if issue.CompletedAt, err = parseOptionalTime(completed); err != nil {
		return Issue{}, err
	}
	if issue.CreatedAt, issue.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Issue{}, err
	}

	return issue, nil
}

// uniqueSorted returns the non-empty values sorted, without duplicates, never nil.
func uniqueSorted(values []string) []string {
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}

	slices.Sort(result)
	return slices.Compact(result)
}

func validatePriority(priority string) error {
	switch priority {
	case "", "urgent", "high", "medium", "low":
		return nil
	default:
		return invalid("priority", "must be urgent, high, medium, low, or empty")
	}
}
