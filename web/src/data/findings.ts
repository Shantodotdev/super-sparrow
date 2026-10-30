import type { Actor, Finding, FindingSeverity, FindingStatus } from "../lib/types";
import { at, project } from "./project";
import { agents, people } from "./team";

type Row = [number, number, number, FindingSeverity, FindingStatus, Actor, string, string, string, number];

// Issue number, round, number within the round, severity, status, who found it, title, problem,
// and the file and line it points at.
const rows: Row[] = [
  [154, 2, 1, "blocking", "open", agents.shantoCodex, "Concurrent decisions can overwrite one another", "Two reviewers can submit decisions at the same time, and the later update silently replaces the first decision.", "server/internal/store/findings.go", 118],
  [154, 2, 2, "should_fix", "open", agents.shantoClaude, "Show a useful message when a decision is stale", "A reviewer who submits an outdated decision needs to see the latest state before trying again.", "web/src/components/finding-actions.tsx", 64],
  [154, 1, 1, "should_fix", "verified", agents.shantoClaude, "Record the actor on each decision", "The decision history needs to identify the reviewer responsible for each change.", "server/internal/store/events.go", 82],
  [142, 1, 1, "blocking", "verified", agents.shantoClaude, "Limit failed code verification attempts", "Repeated invalid code attempts should be limited to protect sign-in sessions.", "server/internal/auth/codes.go", 74],
  [142, 1, 2, "should_fix", "fixed", agents.shantoCodex, "Keep the code field accessible", "The verification input needs an accessible label and clear error feedback.", "web/src/routes/sign-in.tsx", 37],
  [153, 1, 1, "should_fix", "open", agents.shantoCodex, "Preserve filters when switching views", "Switching between list and board should not clear the active issue filters.", "web/src/routes/issues.tsx", 51],
  [142, 2, 1, "blocking", "open", agents.shantoCodex, "Session can expire during sign-in", "The sign-in flow can take longer than expected, causing the session to expire before the user completes authentication. This results in an error and the user having to start over.", "server/internal/auth/session.go", 42],
  [142, 2, 2, "should_fix", "open", agents.shantoClaude, "Missing validation for branch name", "Branch names are not validated, allowing invalid characters that fail on push.", "server/internal/store/issue.go", 68],
  [142, 2, 3, "nit", "open", agents.shantoCodex, "Clarify empty state copy", "The empty state copy is a bit vague and could better explain next steps for new users.", "web/src/routes/index.tsx", 24],
  [144, 1, 1, "should_fix", "open", agents.shantoClaude, "Handle expired sign-in codes", "The error does not explain how to request a new code.", "web/src/routes/sign-in.tsx", 32],
];

const decided: FindingStatus[] = ["fix", "wontfix", "later", "fixed", "verified"];

export const seedFindings: Finding[] = rows.map(
  ([issueNumber, round, number, severity, status, foundBy, title, problem, filePath, line]) => {
    const created = at("2024-11-06", "11:31");
    const fixed = status === "fixed" || status === "verified";

    return {
      id: `finding-${issueNumber}-${round}-${number}`,
      review_round_id: `round-${issueNumber}-${round}`,
      issue_id: `issue-${issueNumber}`,
      issue_key: `${project.key}-${issueNumber}`,
      round,
      number,
      label: `R${number}`,
      severity,
      title,
      problem,
      fix: "",
      evidence: "",
      file_path: filePath,
      line,
      commit_sha: "",
      status,
      decision_reason: "",
      decided_by_actor_id: decided.includes(status) ? people.shanto.id : "",
      decided_at: decided.includes(status) ? at("2024-11-06", "11:37") : null,
      found_by_actor_id: foundBy.id,
      fixed_by_actor_id: fixed ? agents.shantoCodex.id : "",
      fixed_in_commit: fixed ? "7b4e1aa" : "",
      fixed_at: fixed ? at("2024-11-06", "11:52") : null,
      verified_by_actor_id: status === "verified" ? agents.shantoClaude.id : "",
      verified_at: status === "verified" ? at("2024-11-07", "10:05") : null,
      duplicate_of_finding_id: "",
      follow_up_issue_id: "",
      comment_count: 0,
      created_at: created,
      updated_at: created,
    };
  },
);
