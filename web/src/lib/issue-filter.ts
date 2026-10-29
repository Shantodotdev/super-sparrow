/** Filters issue rows using the selected project list controls. */
export function filterIssues<
  T extends {
    key: string;
    title: string;
    status: string;
    assignee: string;
    labels: string[];
    milestone: string;
  },
>(
  issues: T[],
  filters: { query: string; status: string; assignee: string; label: string; milestone: string },
): T[] {
  const query = filters.query.trim().toLowerCase();
  return issues.filter(
    (issue) =>
      `${issue.key} ${issue.title}`.toLowerCase().includes(query) &&
      (filters.status === "All" || issue.status === filters.status) &&
      (filters.assignee === "All" || issue.assignee === filters.assignee) &&
      (filters.label === "All" || issue.labels.includes(filters.label)) &&
      (filters.milestone === "All" || issue.milestone === filters.milestone),
  );
}
