import type { QueryClient } from "@tanstack/react-query";
import type { Issue } from "../lib/types";

/** Publishes a committed creation without waiting for background reads or seeding incomplete lists. */
export function cacheCreatedIssue(client: QueryClient, issue: Issue): void {
  const matches = (key: readonly unknown[]) =>
    key[0] === "issues" && (key.length === 1 || key[1] === issue.project_key);
  void client.cancelQueries({ predicate: (query) => matches(query.queryKey) });
  const keepNewest = (current: Issue | undefined) =>
    current &&
    (current.plan_revision > issue.plan_revision || current.updated_at > issue.updated_at)
      ? current
      : issue;
  client.setQueriesData<Issue[]>(
    { predicate: (query) => matches(query.queryKey) },
    (current) =>
      current && [
        keepNewest(current.find((item) => item.id === issue.id)),
        ...current.filter((item) => item.id !== issue.id),
      ],
  );
  client.setQueryData<Issue>(["issue", issue.key], keepNewest);
  void client.invalidateQueries({ predicate: (query) => matches(query.queryKey) });
}
