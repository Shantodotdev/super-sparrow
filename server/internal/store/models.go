package store

import (
	"fmt"
	"time"
)

// SystemActorID is the ID of the built-in system actor, which records actions that no person or
// agent took, such as creating the first owner.
const SystemActorID = "00000000-0000-7000-8000-000000000000"

// ActorKind says whether an actor is a person, an agent working for a person, or the system.
type ActorKind string

const (
	ActorHuman  ActorKind = "human"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

// Role is a person's permission level.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

func (r Role) valid() bool { return r == RoleOwner || r == RoleMember }

// Actor is anyone who can change data: a person, an agent, or the system.
type Actor struct {
	ID           string
	Kind         ActorKind
	DisplayName  string
	Email        string
	Role         Role
	AgentName    string
	OwnerActorID string
	DisabledAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Active reports whether the actor can still make changes.
func (a Actor) Active() bool { return a.DisabledAt == nil }

// IsOwner reports whether the actor is an active owner.
func (a Actor) IsOwner() bool { return a.Kind == ActorHuman && a.Role == RoleOwner && a.Active() }

// Caller identifies who is making a change. The server builds it from the authenticated session
// or agent token, never from request input.
type Caller struct {
	ActorID string
	// ReportedModel is the model an agent says it runs. It is recorded on events as reported,
	// because the server cannot verify it.
	ReportedModel string
}

// Project groups issues under a key such as WP.
type Project struct {
	ID              string
	Key             string
	Name            string
	NextIssueNumber int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MilestoneStatus is where a milestone stands.
type MilestoneStatus string

const (
	MilestonePlanned MilestoneStatus = "planned"
	MilestoneActive  MilestoneStatus = "active"
	MilestoneDone    MilestoneStatus = "done"
)

func (s MilestoneStatus) valid() bool {
	return s == MilestonePlanned || s == MilestoneActive || s == MilestoneDone
}

// Milestone is a phase of a project.
type Milestone struct {
	ID        string
	ProjectID string
	Name      string
	Position  int
	Status    MilestoneStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Label tags issues within one project.
type Label struct {
	ID        string
	ProjectID string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IssueStatus is where an issue stands.
type IssueStatus string

const (
	IssueBacklog    IssueStatus = "backlog"
	IssueTodo       IssueStatus = "todo"
	IssueInProgress IssueStatus = "in_progress"
	IssueInReview   IssueStatus = "in_review"
	IssueDone       IssueStatus = "done"
	IssueCanceled   IssueStatus = "canceled"
)

func (s IssueStatus) valid() bool {
	switch s {
	case IssueBacklog, IssueTodo, IssueInProgress, IssueInReview, IssueDone, IssueCanceled:
		return true
	}
	return false
}

func (s IssueStatus) closed() bool { return s == IssueDone || s == IssueCanceled }

// Issue is a task whose description is its plan.
type Issue struct {
	ID              string
	ProjectID       string
	ProjectKey      string
	Number          int
	Title           string
	Plan            string
	Status          IssueStatus
	ParentIssueID   string
	MilestoneID     string
	AssigneeActorID string
	BranchName      string
	PullRequestURL  string
	LabelIDs        []string
	CreatedByID     string
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Key returns the issue's key, such as WP-142.
func (i Issue) Key() string { return fmt.Sprintf("%s-%d", i.ProjectKey, i.Number) }

// ReviewRound is one review of an issue's code, covering a range of commits.
type ReviewRound struct {
	ID                  string
	IssueID             string
	Number              int
	FromCommit          string
	ToCommit            string
	IncludesUncommitted bool
	PlanCoverage        string
	Summary             string
	AuthorActorID       string
	FinishedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Severity is how much a finding matters.
type Severity string

const (
	SeverityBlocking  Severity = "blocking"
	SeverityShouldFix Severity = "should_fix"
	SeverityNit       Severity = "nit"
)

func (s Severity) valid() bool {
	return s == SeverityBlocking || s == SeverityShouldFix || s == SeverityNit
}

// FindingStatus is where a finding stands. Open findings wait for a decision; fix, wontfix, and
// later are the developer's decisions; fixed and verified follow a fix.
type FindingStatus string

const (
	FindingOpen     FindingStatus = "open"
	FindingFix      FindingStatus = "fix"
	FindingWontfix  FindingStatus = "wontfix"
	FindingLater    FindingStatus = "later"
	FindingFixed    FindingStatus = "fixed"
	FindingVerified FindingStatus = "verified"
)

func (s FindingStatus) valid() bool {
	switch s {
	case FindingOpen, FindingFix, FindingWontfix, FindingLater, FindingFixed, FindingVerified:
		return true
	}
	return false
}

// Finding is one problem a review found.
type Finding struct {
	ID                   string
	ReviewRoundID        string
	IssueID              string
	RoundNumber          int
	Number               int
	Severity             Severity
	Title                string
	Problem              string
	Fix                  string
	Evidence             string
	FilePath             string
	Line                 int
	CommitSHA            string
	Status               FindingStatus
	DecisionReason       string
	DecidedByActorID     string
	DecidedAt            *time.Time
	FoundByActorID       string
	FixedByActorID       string
	FixedInCommit        string
	FixedAt              *time.Time
	VerifiedByActorID    string
	VerifiedAt           *time.Time
	DuplicateOfFindingID string
	FollowUpIssueID      string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Label returns the finding's short name within its round, such as R3.
func (f Finding) Label() string { return fmt.Sprintf("R%d", f.Number) }

// Comment is a message on an issue or a finding, optionally replying to another comment.
type Comment struct {
	ID              string
	IssueID         string
	FindingID       string
	ParentCommentID string
	Body            string
	AuthorActorID   string
	EditedAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TargetType names the kind of record an event is about.
type TargetType string

const (
	TargetActor       TargetType = "actor"
	TargetProject     TargetType = "project"
	TargetMilestone   TargetType = "milestone"
	TargetLabel       TargetType = "label"
	TargetIssue       TargetType = "issue"
	TargetReviewRound TargetType = "review_round"
	TargetFinding     TargetType = "finding"
	TargetComment     TargetType = "comment"
)

func (t TargetType) valid() bool {
	switch t {
	case TargetActor, TargetProject, TargetMilestone, TargetLabel, TargetIssue, TargetReviewRound, TargetFinding, TargetComment:
		return true
	}
	return false
}

// Event is one recorded change. Events are never changed or deleted.
type Event struct {
	Sequence      int64
	ID            string
	ProjectID     string
	TargetType    TargetType
	TargetID      string
	Kind          string
	ActorID       string
	ReportedModel string
	Payload       []byte
	CreatedAt     time.Time
}

// Field is an optional update: a field is changed only when Set is true. For a field that can
// be empty, such as an issue's milestone, setting it to the zero value clears it.
type Field[T any] struct {
	Set   bool
	Value T
}

// Change returns a Field that sets the value.
func Change[T any](value T) Field[T] { return Field[T]{Set: true, Value: value} }

// Session is one signed-in device of a person.
type Session struct {
	ID          string
	ActorID     string
	DeviceLabel string
	LastSeenAt  time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AgentToken lets an agent act through the API. Only a hash of its secret is stored.
type AgentToken struct {
	ID           string
	AgentActorID string
	OwnerActorID string
	AgentName    string
	Label        string
	// DisplayHint shows enough of the secret, such as ssp_…x7Qe, for a person to tell tokens apart.
	DisplayHint string
	LastUsedAt  *time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
