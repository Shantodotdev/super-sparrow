import { issueStatuses } from "./issue-status";
import type { IssueStatus } from "./types";

/** The issues page's view and filters; a missing value means the list view or no filter. */
export type IssuesSearch = {
  view?: "board";
  q?: string;
  status?: IssueStatus;
  assignee?: string;
  label?: string;
  milestone?: string;
};

export type IssueTab = "plan" | "findings" | "rounds" | "activity" | "comments";

export type IssueSearch = {
  tab?: Exclude<IssueTab, "plan">;
  round?: number;
  finding?: string;
  findingStatus?: "all";
};

/** The finding selected on a review round page, by its label such as R2. */
export type ReviewSearch = { finding?: string };

function text(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function oneOf<T extends string>(value: unknown, values: readonly T[]): T | undefined {
  return values.find((item) => item === value);
}

/** Keeps only known views and statuses and non-empty filter values; other values are dropped. */
export function validateIssuesSearch(search: Record<string, unknown>): IssuesSearch {
  return {
    view: search.view === "board" ? "board" : undefined,
    q: text(search.q),
    status: oneOf(search.status, issueStatuses),
    assignee: text(search.assignee),
    label: text(search.label),
    milestone: text(search.milestone),
  };
}

/** Validates issue tabs and limits drawer selection and filters to review-related tabs. */
export function validateIssueSearch(search: Record<string, unknown>): IssueSearch {
  const reviewTab = search.tab === "rounds" || search.tab === "findings";
  const round =
    typeof search.round === "string" || typeof search.round === "number"
      ? Number(search.round)
      : NaN;

  return {
    tab:
      search.tab === "findings" ||
      search.tab === "rounds" ||
      search.tab === "activity" ||
      search.tab === "comments"
        ? search.tab
        : undefined,
    round: reviewTab && Number.isSafeInteger(round) && round > 0 ? round : undefined,
    finding: reviewTab ? text(search.finding) : undefined,
    findingStatus: search.tab === "findings" && search.findingStatus === "all" ? "all" : undefined,
  };
}

export function validateReviewSearch(search: Record<string, unknown>): ReviewSearch {
  const finding = text(search.finding);
  return { finding: finding && /^R\d+$/.test(finding) ? finding : undefined };
}
