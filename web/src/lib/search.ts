import { findingSeverities, findingStatuses } from "./finding-status";
import { issueStatuses } from "./issue-status";
import type { FindingSeverity, FindingStatus, IssueStatus } from "./types";

/** The issues page's view and filters; a missing value means the list view or no filter. */
export type IssuesSearch = {
  view?: "board";
  q?: string;
  status?: IssueStatus;
  assignee?: string;
  label?: string;
  milestone?: string;
};

export type IssueTab = "plan" | "rounds" | "activity";

export type IssueSearch = { tab?: Exclude<IssueTab, "plan"> };

/**
 * The findings page's filters and the selected finding, by ID. A missing status shows the
 * unresolved findings, as the page's title says.
 */
export type FindingsSearch = {
  q?: string;
  severity?: FindingSeverity;
  status?: FindingStatus | "all";
  issue?: string;
  finding?: string;
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

/** Accepts only the non-default issue tabs; the plan tab is the absence of the parameter. */
export function validateIssueSearch(search: Record<string, unknown>): IssueSearch {
  return { tab: search.tab === "rounds" || search.tab === "activity" ? search.tab : undefined };
}

export function validateFindingsSearch(search: Record<string, unknown>): FindingsSearch {
  return {
    q: text(search.q),
    severity: oneOf(search.severity, findingSeverities),
    status: search.status === "all" ? "all" : oneOf(search.status, findingStatuses),
    issue: text(search.issue),
    finding: text(search.finding),
  };
}

export function validateReviewSearch(search: Record<string, unknown>): ReviewSearch {
  const finding = text(search.finding);
  return { finding: finding && /^R\d+$/.test(finding) ? finding : undefined };
}
