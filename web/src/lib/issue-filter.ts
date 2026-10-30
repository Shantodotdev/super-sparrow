import type { IssuesSearch } from "./search";

/** Filters issues by the issues page's search text and filters; a missing filter matches all. */
export function filterIssues<
  T extends {
    key: string;
    title: string;
    status: string;
    assignee_actor_id: string;
    label_ids: string[];
    milestone_id: string;
  },
>(issues: T[], filters: IssuesSearch): T[] {
  const query = (filters.q ?? "").trim().toLowerCase();
  return issues.filter(
    (issue) =>
      `${issue.key} ${issue.title}`.toLowerCase().includes(query) &&
      (!filters.status || issue.status === filters.status) &&
      (!filters.assignee || issue.assignee_actor_id === filters.assignee) &&
      (!filters.label || issue.label_ids.includes(filters.label)) &&
      (!filters.milestone || issue.milestone_id === filters.milestone),
  );
}
