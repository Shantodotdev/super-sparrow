import type { Actor, ReviewRound } from "../lib/types";
import { at } from "./project";
import { agents } from "./team";

type Row = [number, number, string, string, string, Actor, string];

// Issue number, round number, the commit range it reviewed, the day it finished, its reviewer, and
// its summary.
const rows: Row[] = [
  [154, 2, "d35a2cf", "8b7e13a", "2024-11-08", agents.shantoCodex, "Reviewed concurrent decision handling and feedback for conflicting updates."],
  [154, 1, "a17c021", "d35a2cf", "2024-11-07", agents.shantoClaude, "Reviewed the initial decision flow and its audit history."],
  [142, 2, "a3f9c2d", "7b4e1aa", "2024-11-06", agents.shantoCodex, "Reviewed sign-in expiry, branch validation, and related interface updates."],
  [142, 1, "6fa81d0", "a3f9c2d", "2024-11-04", agents.shantoClaude, "Reviewed code verification and the sign-in form."],
  [144, 1, "2e89b11", "5c3bd14", "2024-11-05", agents.shantoClaude, "Reviewed error messages for unsuccessful sign-in."],
  [153, 1, "019ac41", "27d9a8c", "2024-11-07", agents.shantoCodex, "Reviewed the issue list filters and view switching."],
];

export const seedReviewRounds: ReviewRound[] = rows.map(
  ([issueNumber, number, from, to, day, author, summary]) => ({
    id: `round-${issueNumber}-${number}`,
    issue_id: `issue-${issueNumber}`,
    number,
    from_commit: from,
    to_commit: to,
    includes_uncommitted: false,
    plan_coverage: "",
    summary,
    author_actor_id: author.id,
    finished_at: at(day, "11:40"),
    created_at: at(day, "11:28"),
    updated_at: at(day, "11:40"),
  }),
);
