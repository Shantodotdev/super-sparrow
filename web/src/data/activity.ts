import type { ActivityEvent, Actor, EventTarget } from "../lib/types";
import { at, project } from "./project";
import { agents, people } from "./team";

let sequence = 0;

function event(
  actor: Actor,
  kind: string,
  target: [EventTarget, string],
  payload: Record<string, unknown>,
  date: string,
  time: string,
  reportedModel = "",
): ActivityEvent {
  sequence += 1;
  return {
    sequence,
    id: `event-${sequence}`,
    project_id: project.id,
    target_type: target[0],
    target_id: target[1],
    kind,
    actor_id: actor.id,
    reported_model: reportedModel,
    payload,
    created_at: at(date, time),
  };
}

/** Events oldest first, as the API pages them. */
export const seedEvents: ActivityEvent[] = [
  event(people.shanto, "issue.created", ["issue", "issue-142"], { key: "SP-142", title: "Add email code sign-in" }, "2024-11-05", "16:18"),
  event(agents.shantoCodex, "review_round.started", ["review_round", "round-142-2"], { issue_id: "issue-142", number: 2 }, "2024-11-06", "11:28", "gpt-5-codex"),
  event(agents.shantoCodex, "finding.added", ["finding", "finding-142-2-1"], { issue_id: "issue-142", round: 2, number: 1, title: "Session can expire during sign-in" }, "2024-11-06", "11:31", "gpt-5-codex"),
  event(agents.shantoClaude, "finding.added", ["finding", "finding-142-2-2"], { issue_id: "issue-142", round: 2, number: 2, title: "Missing validation for branch name" }, "2024-11-06", "11:34", "claude-opus"),
  event(people.shanto, "finding.status_changed", ["finding", "finding-142-1-2"], { issue_id: "issue-142", round: 1, number: 2, from: "open", to: "fix" }, "2024-11-06", "11:37"),
  event(agents.shantoCodex, "finding.status_changed", ["finding", "finding-142-1-2"], { issue_id: "issue-142", round: 1, number: 2, from: "fix", to: "fixed", fixed_in_commit: "7b4e1aa" }, "2024-11-06", "11:52", "gpt-5-codex"),
  event(people.priya, "comment.added", ["comment", "comment-3"], { issue_id: "issue-142", finding_id: "" }, "2024-11-06", "12:10"),
  event(agents.shantoClaude, "issue.checklist_changed", ["issue", "issue-142"], { items: [0, 1], plan_revision: 2 }, "2024-11-06", "12:40", "claude-opus"),
];
