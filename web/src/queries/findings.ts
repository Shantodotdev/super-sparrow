import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import type { Finding, FindingStatus, Issue } from "../lib/types";
import { issuesQuery } from "./issues";
import { findingPath, getFindings } from "./resources";
import { projectsQuery } from "./project-resources";

export const findingsQuery = queryOptions({
  queryKey: ["findings"],
  queryFn: async ({ client }) => getFindings(await client.fetchQuery(projectsQuery)),
});
export type Decision = Extract<FindingStatus, "fix" | "wontfix" | "later">;

/** Publishes a committed lifecycle change and refreshes counts without delaying the drawer. */
export function useChangeFindingStatus() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      finding,
      status,
      reason,
    }: {
      finding: Finding;
      status: FindingStatus;
      reason?: string;
    }) =>
      apiWrite<{ finding: Finding; follow_up_issue: Issue | null }>(
        `${findingPath(finding)}/status`,
        "POST",
        { status, reason },
      ),
    onSuccess: ({ finding, follow_up_issue }) => {
      void client.cancelQueries({ queryKey: findingsQuery.queryKey });
      client.setQueryData(findingsQuery.queryKey, (current) =>
        current?.map((item) => (item.id === finding.id ? finding : item)),
      );
      if (follow_up_issue) {
        void client.cancelQueries({ queryKey: issuesQuery.queryKey });
        client.setQueryData(
          issuesQuery.queryKey,
          (current) =>
            current && [
              follow_up_issue,
              ...current.filter((item) => item.id !== follow_up_issue.id),
            ],
        );
      }
      for (const key of ["findings", "issues", "issue", "activity"])
        void client.invalidateQueries({ queryKey: [key] });
    },
    onError: () => {
      void client.invalidateQueries({ queryKey: findingsQuery.queryKey });
    },
  });
}

export function useAddComment() {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({
      issueId,
      findingId,
      body,
    }: {
      issueId: string;
      findingId: string;
      body: string;
    }) => {
      const issue = client.getQueryData(issuesQuery.queryKey)?.find((i) => i.id === issueId);
      const finding = client.getQueryData(findingsQuery.queryKey)?.find((f) => f.id === findingId);
      if (!issue || (findingId && !finding)) throw new Error("Comment target is not loaded");
      const path = finding ? findingPath(finding) : `/api/issues/${issue.key}`;
      return apiWrite(`${path}/comments`, "POST", { body });
    },
    onSuccess: () => client.invalidateQueries(),
  });
  return (issueId: string, findingId: string, body: string) =>
    mutation.mutateAsync({ issueId, findingId, body });
}
