// The Go API's JSON shapes, as server/internal/httpserver/views.go writes them. Timestamps are
// RFC 3339 strings, and an empty string means "none" for optional IDs, as the API sends them.

export type NavSection = "issues" | "activity" | "milestones" | "findings" | "settings";

export type ActorKind = "human" | "agent" | "system";

export type Role = "owner" | "member";

export type Actor = {
  id: string;
  kind: ActorKind;
  display_name: string;
  email?: string;
  role?: Role;
  agent_name?: string;
  owner_actor_id?: string;
  disabled_at: string | null;
  created_at: string;
};

export type MilestoneStatus = "planned" | "active" | "done";

export type Milestone = {
  id: string;
  project_id: string;
  name: string;
  position: number;
  status: MilestoneStatus;
  created_at: string;
  updated_at: string;
};

export type Label = {
  id: string;
  project_id: string;
  name: string;
  created_at: string;
};

export type IssueStatus = "backlog" | "todo" | "in_progress" | "in_review" | "done" | "canceled";

/** Issue priority. The API has no priority yet; it exists only in the frontend preview. */
export type IssuePriority = "urgent" | "high" | "medium" | "low";

export type ChecklistItem = { index: number; text: string; checked: boolean };

export type Issue = {
  id: string;
  key: string;
  project_id: string;
  project_key: string;
  number: number;
  title: string;
  /** The plan in markdown. The API sends it, and `checklist`, only when one issue is fetched. */
  plan?: string;
  plan_revision: number;
  checklist?: ChecklistItem[];
  checklist_progress: { done: number; total: number };
  status: IssueStatus;
  /** Not in the API yet; see IssuePriority. */
  priority: IssuePriority;
  parent_issue_id: string;
  milestone_id: string;
  assignee_actor_id: string;
  branch_name: string;
  pull_request_url: string;
  label_ids: string[];
  created_by_actor_id: string;
  unresolved_findings: number;
  comment_count: number;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
};

export type ReviewRound = {
  id: string;
  issue_id: string;
  number: number;
  from_commit: string;
  to_commit: string;
  includes_uncommitted: boolean;
  plan_coverage: string;
  summary: string;
  author_actor_id: string;
  finished_at: string | null;
  created_at: string;
  updated_at: string;
};

export type FindingSeverity = "blocking" | "should_fix" | "nit";

export type FindingStatus = "open" | "fix" | "wontfix" | "later" | "fixed" | "verified";

export type Finding = {
  id: string;
  review_round_id: string;
  issue_id: string;
  issue_key: string;
  round: number;
  number: number;
  /** The finding's label within its round, such as R3. */
  label: string;
  severity: FindingSeverity;
  title: string;
  problem: string;
  fix: string;
  evidence: string;
  file_path: string;
  line: number;
  commit_sha: string;
  status: FindingStatus;
  decision_reason: string;
  decided_by_actor_id: string;
  decided_at: string | null;
  found_by_actor_id: string;
  fixed_by_actor_id: string;
  fixed_in_commit: string;
  fixed_at: string | null;
  verified_by_actor_id: string;
  verified_at: string | null;
  duplicate_of_finding_id: string;
  follow_up_issue_id: string;
  comment_count: number;
  created_at: string;
  updated_at: string;
};

export type Comment = {
  id: string;
  issue_id: string;
  finding_id: string;
  parent_comment_id: string;
  body: string;
  author_actor_id: string;
  edited_at: string | null;
  created_at: string;
  updated_at: string;
};

export type EventTarget =
  "actor" | "project" | "milestone" | "label" | "issue" | "review_round" | "finding" | "comment";

export type ActivityEvent = {
  sequence: number;
  id: string;
  project_id: string;
  target_type: EventTarget;
  target_id: string;
  /** What happened, such as finding.added. */
  kind: string;
  actor_id: string;
  reported_model: string;
  payload: Record<string, unknown>;
  created_at: string;
};

export type AgentToken = {
  id: string;
  agent_actor_id: string;
  owner_actor_id: string;
  agent_name: string;
  label: string;
  display_hint: string;
  last_used_at: string | null;
  expires_at: string;
  created_at: string;
};

/** What creating a token returns: the token and its secret, which the API never sends again. */
export type NewAgentToken = { token: AgentToken; secret: string };
