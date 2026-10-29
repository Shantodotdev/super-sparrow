export type IssuesSearch = { view?: "board" };

export type IssueTab = "plan" | "rounds" | "activity";

export type IssueSearch = { tab?: Exclude<IssueTab, "plan"> };

/** Accepts only the board view; the list view is the absence of the parameter. */
export function validateIssuesSearch(search: Record<string, unknown>): IssuesSearch {
  return { view: search.view === "board" ? "board" : undefined };
}

/** Accepts only the non-default issue tabs; the plan tab is the absence of the parameter. */
export function validateIssueSearch(search: Record<string, unknown>): IssueSearch {
  return { tab: search.tab === "rounds" || search.tab === "activity" ? search.tab : undefined };
}
