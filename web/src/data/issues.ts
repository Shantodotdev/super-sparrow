import type { Actor, Issue, IssuePriority, IssueStatus, Milestone } from "../lib/types";
import { checklistItems } from "../lib/checklist";
import { at, milestones, project } from "./project";
import { agents, people } from "./team";

const signInPlan = `## Goal
Allow people to sign in with a one-time code sent to their email address, with clear messages and graceful handling of the edge cases.

## Approach
1. Add backend endpoints to request and verify email codes.
2. Build the screens for requesting a code and entering it.
3. Connect the sign-in flow to session management.
4. Add loading, error, and success states.

## Edge cases
- An expired code shows a clear error and offers a new code.
- Too many requests are rate limited with a helpful message.
- Network errors offer a retry.

## Checklist
- [x] Backend endpoints (request and verify code)
- [x] Email code screens (request, input, states)
- [ ] Connect to session management
- [ ] Write tests
- [ ] Update documentation`;

const decisionsPlan = `## Goal
Two reviewers deciding on the same finding at once must not overwrite each other.

## Checklist
- [x] Record who made each decision
- [ ] Refuse a decision made on a stale finding
- [ ] Show the latest decision when one is refused`;

/** Plans, branches, and pull requests of the issues that have them. */
const details: Record<number, Partial<Pick<Issue, "plan" | "branch_name" | "pull_request_url">>> = {
  142: {
    plan: signInPlan,
    branch_name: "feat/sp-142-email-code",
    pull_request_url: "https://github.com/shantodotdev/super-sparrow/pull/324",
  },
  154: {
    plan: decisionsPlan,
    branch_name: "fix/sp-154-concurrent-decisions",
    pull_request_url: "https://github.com/shantodotdev/super-sparrow/pull/331",
  },
  153: { plan: "Filter the issue list by status, assignee, label, and milestone.", branch_name: "feat/sp-153-issue-filters" },
};

type Row = [number, string, IssueStatus, IssuePriority, string[], Actor, Milestone, string];

// Number, title, status, priority, labels, assignee, milestone, and the day it last changed.
const rows: Row[] = [
  [142, "Add email code sign-in", "in_review", "high", ["auth", "frontend", "security"], people.shanto, milestones.launch, "2024-11-06"],
  [144, "Clarify error message for invalid login", "in_review", "medium", ["ux", "frontend"], agents.shantoClaude, milestones.launch, "2024-11-05"],
  [148, "Add telemetry for sign-in failures", "in_review", "medium", ["observability"], people.shanto, milestones.launch, "2024-11-04"],
  [136, "Validate branch names", "in_progress", "high", ["backend", "security"], agents.shantoClaude, milestones.launch, "2024-11-06"],
  [139, "Improve empty state copy", "in_progress", "low", ["ux"], agents.shantoCodex, milestones.launch, "2024-11-06"],
  [141, "Add keyboard shortcuts", "in_progress", "medium", ["frontend"], people.priya, milestones.launch, "2024-11-05"],
  [140, "Add branch creation flow", "todo", "medium", ["feature"], people.shanto, milestones.launch, "2024-11-04"],
  [146, "Improve search performance", "todo", "high", ["performance"], agents.shantoClaude, milestones.launch, "2024-11-04"],
  [150, "Dark mode polish", "todo", "low", ["ui"], people.priya, milestones.launch, "2024-11-03"],
  [152, "Export findings to CSV", "backlog", "medium", ["feature"], agents.shantoCodex, milestones.later, "2024-10-31"],
  [153, "Add issue filters to the project list", "in_review", "medium", ["frontend"], people.priya, milestones.launch, "2024-11-07"],
  [154, "Handle concurrent review decisions", "in_review", "urgent", ["bug"], agents.shantoCodex, milestones.launch, "2024-11-07"],
  [155, "Stream project activity updates", "in_progress", "medium", ["realtime"], agents.shantoClaude, milestones.launch, "2024-11-07"],
  [156, "Improve review finding keyboard actions", "in_progress", "high", ["accessibility"], people.shanto, milestones.launch, "2024-11-07"],
  [157, "Add project milestone settings", "todo", "medium", ["feature"], agents.shantoCodex, milestones.launch, "2024-11-07"],
  [158, "Document agent token rotation", "todo", "low", ["documentation"], people.priya, milestones.launch, "2024-11-07"],
  [159, "Add finding discussion threads", "backlog", "medium", ["frontend"], agents.shantoClaude, milestones.later, "2024-11-07"],
  [160, "Support duplicate finding links", "backlog", "low", ["backend"], people.shanto, milestones.later, "2024-11-07"],
  [161, "Add email code expiration checks", "done", "high", ["auth"], agents.shantoCodex, milestones.launch, "2024-11-07"],
  [162, "Create project issue counters", "done", "medium", ["database"], agents.shantoClaude, milestones.launch, "2024-11-07"],
  [163, "Add issue status transitions", "done", "high", ["backend"], people.priya, milestones.launch, "2024-11-07"],
  [164, "Explore alternate sign-in flow", "canceled", "low", ["research"], people.shanto, milestones.later, "2024-11-07"],
  [165, "Review backup failure messaging", "todo", "high", ["security"], agents.shantoCodex, milestones.later, "2024-11-07"],
  [166, "Add responsive layout for review cards", "in_progress", "medium", ["ui"], people.priya, milestones.launch, "2024-11-07"],
  [167, "Add session device management", "done", "medium", ["security"], people.shanto, milestones.launch, "2024-11-08"],
  [168, "Add review round commit ranges", "done", "medium", ["backend"], agents.shantoClaude, milestones.launch, "2024-11-08"],
  [169, "Explore alternative review layouts", "canceled", "low", ["research"], people.priya, milestones.later, "2024-11-08"],
  [170, "Investigate external notification support", "canceled", "low", ["research"], agents.shantoCodex, milestones.later, "2024-11-08"],
  [171, "Add finding search across review rounds", "backlog", "medium", ["feature"], people.shanto, milestones.later, "2024-11-08"],
  [172, "Improve empty milestone states", "todo", "low", ["ux"], people.priya, milestones.launch, "2024-11-08"],
];

/** Builds the issue as the API returns it. Finding and comment counts are filled in by the queries. */
export function newIssue(fields: {
  number: number;
  title: string;
  status: IssueStatus;
  priority: IssuePriority;
  labelIds: string[];
  assigneeId: string;
  milestoneId: string;
  plan: string;
  branchName?: string;
  pullRequestUrl?: string;
  createdById: string;
  createdAt: string;
  updatedAt: string;
}): Issue {
  const checklist = checklistItems(fields.plan);
  const finished = fields.status === "done" || fields.status === "canceled";

  return {
    id: `issue-${fields.number}`,
    key: `${project.key}-${fields.number}`,
    project_id: project.id,
    project_key: project.key,
    number: fields.number,
    title: fields.title,
    plan: fields.plan,
    plan_revision: 1,
    checklist,
    checklist_progress: { done: checklist.filter((item) => item.checked).length, total: checklist.length },
    status: fields.status,
    priority: fields.priority,
    parent_issue_id: "",
    milestone_id: fields.milestoneId,
    assignee_actor_id: fields.assigneeId,
    branch_name: fields.branchName ?? "",
    pull_request_url: fields.pullRequestUrl ?? "",
    label_ids: fields.labelIds,
    created_by_actor_id: fields.createdById,
    unresolved_findings: 0,
    comment_count: 0,
    completed_at: finished ? fields.updatedAt : null,
    created_at: fields.createdAt,
    updated_at: fields.updatedAt,
  };
}

export const seedIssues: Issue[] = rows.map(
  ([number, title, status, priority, labels, assignee, milestone, day]) =>
    newIssue({
      number,
      title,
      status,
      priority,
      labelIds: labels.map((name) => `label-${name}`),
      assigneeId: assignee.id,
      milestoneId: milestone.id,
      plan: details[number]?.plan ?? "",
      branchName: details[number]?.branch_name,
      pullRequestUrl: details[number]?.pull_request_url,
      createdById: people.shanto.id,
      createdAt: at("2024-10-30", "16:18"),
      updatedAt: at(day),
    }),
);
