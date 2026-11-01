import type { Actor, ReviewRound } from "../lib/types";
import { at } from "./project";
import { agents } from "./team";

type Row = [number, number, string, Actor, string];

// Issue number, round number, the day it finished, its reviewer, and its summary.
const rows: Row[] = [
  [154, 2, "2024-11-08", agents.shantoCodex, "Reviewed concurrent decision handling and feedback for conflicting updates."],
  [154, 1, "2024-11-07", agents.shantoClaude, "Reviewed the initial decision flow and its audit history."],
  [142, 2, "2024-11-06", agents.shantoCodex, "Reviewed sign-in expiry, branch validation, and related interface updates."],
  [142, 1, "2024-11-04", agents.shantoClaude, "Reviewed code verification and the sign-in form."],
  [144, 1, "2024-11-05", agents.shantoClaude, "Reviewed error messages for unsuccessful sign-in."],
  [153, 1, "2024-11-07", agents.shantoCodex, "Reviewed the issue list filters and view switching."],
];

export const seedReviewRounds: ReviewRound[] = rows.map(
  ([issueNumber, number, day, author, summary]) => ({
    id: `round-${issueNumber}-${number}`,
    issue_id: `issue-${issueNumber}`,
    number,
    plan_coverage: "",
    summary,
    author_actor_id: author.id,
    finished_at: at(day, "11:40"),
    created_at: at(day, "11:28"),
    updated_at: at(day, "11:40"),
  }),
);
