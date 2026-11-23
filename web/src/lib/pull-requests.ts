import type { ChecksState, PullRequest, PullRequestFilter } from "./types";

export const pullRequestFilters: { value: PullRequestFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "merged", label: "Merged" },
  { value: "closed", label: "Closed" },
  { value: "all", label: "All" },
];

export type PullRequestsSearch = { state: PullRequestFilter };

/** Reads the state filter from the URL, showing open pull requests when it is missing or unknown. */
export function validatePullRequestsSearch(search: Record<string, unknown>): PullRequestsSearch {
  const match = pullRequestFilters.find((filter) => filter.value === search.state);
  return { state: match?.value ?? "open" };
}

export const reviewLabel: Record<Exclude<PullRequest["review_state"], "none">, string> = {
  approved: "Approved",
  changes_requested: "Changes requested",
  commented: "Reviewed",
};

export const checksLabel: Record<Exclude<ChecksState, "none">, string> = {
  pending: "Checks running",
  success: "Checks passed",
  failure: "Checks failed",
};

/** Names a pull request's state for screen readers and tooltips. */
export function pullRequestStateLabel(pull: Pick<PullRequest, "state" | "draft">): string {
  if (pull.state === "open" && pull.draft) return "Draft";
  return pull.state[0]!.toUpperCase() + pull.state.slice(1);
}
