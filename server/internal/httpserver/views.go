package httpserver

import (
	"encoding/json"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/checklist"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// The views are the API's JSON shapes. They never hold a hash or a secret, except the new token
// secret that createToken returns once.

type actorView struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email,omitempty"`
	Role         string     `json:"role,omitempty"`
	AgentName    string     `json:"agent_name,omitempty"`
	OwnerActorID string     `json:"owner_actor_id,omitempty"`
	DisabledAt   *time.Time `json:"disabled_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func newActorView(actor store.Actor) actorView {
	return actorView{
		ID: actor.ID, Kind: string(actor.Kind), DisplayName: actor.DisplayName, Email: actor.Email,
		Role: string(actor.Role), AgentName: actor.AgentName, OwnerActorID: actor.OwnerActorID,
		DisabledAt: actor.DisabledAt, CreatedAt: actor.CreatedAt,
	}
}

type sessionView struct {
	ID          string    `json:"id"`
	DeviceLabel string    `json:"device_label"`
	Current     bool      `json:"current"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func newSessionView(session store.Session, currentID string) sessionView {
	return sessionView{
		ID: session.ID, DeviceLabel: session.DeviceLabel, Current: session.ID == currentID,
		LastSeenAt: session.LastSeenAt, ExpiresAt: session.ExpiresAt, CreatedAt: session.CreatedAt,
	}
}

type tokenView struct {
	ID           string     `json:"id"`
	AgentActorID string     `json:"agent_actor_id"`
	OwnerActorID string     `json:"owner_actor_id"`
	AgentName    string     `json:"agent_name"`
	Label        string     `json:"label"`
	DisplayHint  string     `json:"display_hint"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func newTokenView(token store.AgentToken) tokenView {
	return tokenView{
		ID: token.ID, AgentActorID: token.AgentActorID, OwnerActorID: token.OwnerActorID, AgentName: token.AgentName,
		Label: token.Label, DisplayHint: token.DisplayHint, LastUsedAt: token.LastUsedAt,
		ExpiresAt: token.ExpiresAt, CreatedAt: token.CreatedAt,
	}
}

type projectView struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newProjectView(project store.Project) projectView {
	return projectView{ID: project.ID, Key: project.Key, Name: project.Name, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt}
}

type milestoneView struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newMilestoneView(milestone store.Milestone) milestoneView {
	return milestoneView{
		ID: milestone.ID, ProjectID: milestone.ProjectID, Name: milestone.Name, Position: milestone.Position,
		Status: string(milestone.Status), CreatedAt: milestone.CreatedAt, UpdatedAt: milestone.UpdatedAt,
	}
}

type labelView struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func newLabelView(label store.Label) labelView {
	return labelView{ID: label.ID, ProjectID: label.ProjectID, Name: label.Name, CreatedAt: label.CreatedAt}
}

type progressView struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// issueView is an issue. Plan and Checklist are filled only when one issue is fetched, so lists
// stay small; ChecklistProgress is always there.
type issueView struct {
	ID                string           `json:"id"`
	Key               string           `json:"key"`
	ProjectID         string           `json:"project_id"`
	ProjectKey        string           `json:"project_key"`
	Number            int              `json:"number"`
	Title             string           `json:"title"`
	Plan              *string          `json:"plan,omitempty"`
	Checklist         []checklist.Item `json:"checklist,omitempty"`
	ChecklistProgress progressView     `json:"checklist_progress"`
	Status            string           `json:"status"`
	ParentIssueID     string           `json:"parent_issue_id"`
	MilestoneID       string           `json:"milestone_id"`
	AssigneeActorID   string           `json:"assignee_actor_id"`
	BranchName        string           `json:"branch_name"`
	PullRequestURL    string           `json:"pull_request_url"`
	LabelIDs          []string         `json:"label_ids"`
	CreatedByActorID  string           `json:"created_by_actor_id"`
	CompletedAt       *time.Time       `json:"completed_at"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

func newIssueSummaryView(issue store.Issue) issueView {
	items := checklist.Items(issue.Plan)
	progress := progressView{Total: len(items)}
	for _, item := range items {
		if item.Checked {
			progress.Done++
		}
	}

	return issueView{
		ID: issue.ID, Key: issue.Key(), ProjectID: issue.ProjectID, ProjectKey: issue.ProjectKey, Number: issue.Number,
		Title: issue.Title, ChecklistProgress: progress, Status: string(issue.Status), ParentIssueID: issue.ParentIssueID,
		MilestoneID: issue.MilestoneID, AssigneeActorID: issue.AssigneeActorID, BranchName: issue.BranchName,
		PullRequestURL: issue.PullRequestURL, LabelIDs: issue.LabelIDs, CreatedByActorID: issue.CreatedByID,
		CompletedAt: issue.CompletedAt, CreatedAt: issue.CreatedAt, UpdatedAt: issue.UpdatedAt,
	}
}

func newIssueView(issue store.Issue) issueView {
	view := newIssueSummaryView(issue)
	view.Plan, view.Checklist = &issue.Plan, checklist.Items(issue.Plan)
	return view
}

type roundView struct {
	ID                  string     `json:"id"`
	IssueID             string     `json:"issue_id"`
	Number              int        `json:"number"`
	FromCommit          string     `json:"from_commit"`
	ToCommit            string     `json:"to_commit"`
	IncludesUncommitted bool       `json:"includes_uncommitted"`
	PlanCoverage        string     `json:"plan_coverage"`
	Summary             string     `json:"summary"`
	AuthorActorID       string     `json:"author_actor_id"`
	FinishedAt          *time.Time `json:"finished_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func newRoundView(round store.ReviewRound) roundView {
	return roundView{
		ID: round.ID, IssueID: round.IssueID, Number: round.Number, FromCommit: round.FromCommit, ToCommit: round.ToCommit,
		IncludesUncommitted: round.IncludesUncommitted, PlanCoverage: round.PlanCoverage, Summary: round.Summary,
		AuthorActorID: round.AuthorActorID, FinishedAt: round.FinishedAt, CreatedAt: round.CreatedAt, UpdatedAt: round.UpdatedAt,
	}
}

type findingView struct {
	ID                   string     `json:"id"`
	ReviewRoundID        string     `json:"review_round_id"`
	IssueID              string     `json:"issue_id"`
	Round                int        `json:"round"`
	Number               int        `json:"number"`
	Label                string     `json:"label"`
	Severity             string     `json:"severity"`
	Title                string     `json:"title"`
	Problem              string     `json:"problem"`
	Fix                  string     `json:"fix"`
	Evidence             string     `json:"evidence"`
	FilePath             string     `json:"file_path"`
	Line                 int        `json:"line"`
	CommitSHA            string     `json:"commit_sha"`
	Status               string     `json:"status"`
	DecisionReason       string     `json:"decision_reason"`
	DecidedByActorID     string     `json:"decided_by_actor_id"`
	DecidedAt            *time.Time `json:"decided_at"`
	FoundByActorID       string     `json:"found_by_actor_id"`
	FixedByActorID       string     `json:"fixed_by_actor_id"`
	FixedInCommit        string     `json:"fixed_in_commit"`
	FixedAt              *time.Time `json:"fixed_at"`
	VerifiedByActorID    string     `json:"verified_by_actor_id"`
	VerifiedAt           *time.Time `json:"verified_at"`
	DuplicateOfFindingID string     `json:"duplicate_of_finding_id"`
	FollowUpIssueID      string     `json:"follow_up_issue_id"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func newFindingView(f store.Finding) findingView {
	return findingView{
		ID: f.ID, ReviewRoundID: f.ReviewRoundID, IssueID: f.IssueID, Round: f.RoundNumber, Number: f.Number,
		Label: f.Label(), Severity: string(f.Severity), Title: f.Title, Problem: f.Problem, Fix: f.Fix,
		Evidence: f.Evidence, FilePath: f.FilePath, Line: f.Line, CommitSHA: f.CommitSHA, Status: string(f.Status),
		DecisionReason: f.DecisionReason, DecidedByActorID: f.DecidedByActorID, DecidedAt: f.DecidedAt,
		FoundByActorID: f.FoundByActorID, FixedByActorID: f.FixedByActorID, FixedInCommit: f.FixedInCommit,
		FixedAt: f.FixedAt, VerifiedByActorID: f.VerifiedByActorID, VerifiedAt: f.VerifiedAt,
		DuplicateOfFindingID: f.DuplicateOfFindingID, FollowUpIssueID: f.FollowUpIssueID,
		CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

func newFindingViews(findings []store.Finding) []findingView {
	views := make([]findingView, 0, len(findings))
	for _, finding := range findings {
		views = append(views, newFindingView(finding))
	}
	return views
}

type commentView struct {
	ID              string     `json:"id"`
	IssueID         string     `json:"issue_id"`
	FindingID       string     `json:"finding_id"`
	ParentCommentID string     `json:"parent_comment_id"`
	Body            string     `json:"body"`
	AuthorActorID   string     `json:"author_actor_id"`
	EditedAt        *time.Time `json:"edited_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func newCommentView(comment store.Comment) commentView {
	return commentView{
		ID: comment.ID, IssueID: comment.IssueID, FindingID: comment.FindingID, ParentCommentID: comment.ParentCommentID,
		Body: comment.Body, AuthorActorID: comment.AuthorActorID, EditedAt: comment.EditedAt,
		CreatedAt: comment.CreatedAt, UpdatedAt: comment.UpdatedAt,
	}
}

type eventView struct {
	Sequence      int64           `json:"sequence"`
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	TargetType    string          `json:"target_type"`
	TargetID      string          `json:"target_id"`
	Kind          string          `json:"kind"`
	ActorID       string          `json:"actor_id"`
	ReportedModel string          `json:"reported_model"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"created_at"`
}

func newEventView(event store.Event) eventView {
	return eventView{
		Sequence: event.Sequence, ID: event.ID, ProjectID: event.ProjectID, TargetType: string(event.TargetType),
		TargetID: event.TargetID, Kind: event.Kind, ActorID: event.ActorID, ReportedModel: event.ReportedModel,
		Payload: json.RawMessage(event.Payload), CreatedAt: event.CreatedAt,
	}
}

type backupRunView struct {
	ObjectKey  string     `json:"object_key"`
	Result     string     `json:"result"`
	SizeBytes  int64      `json:"size_bytes"`
	Error      string     `json:"error"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

func newBackupRunView(run *store.BackupRun) *backupRunView {
	if run == nil {
		return nil
	}
	return &backupRunView{
		ObjectKey: run.ObjectKey, Result: string(run.Result), SizeBytes: run.SizeBytes, Error: run.Error,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
}
