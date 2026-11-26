import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import type { Comment, Finding, FindingStatus, Issue } from "../lib/types";
import { cacheCreatedIssue } from "./issue-cache";
import {
  findingCommentsQuery,
  issueCommentsQuery,
  issueEventsQuery,
  issueFindingsQuery,
} from "./issue-resources";
import { issueQuery } from "./issues";
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
      const scopedFindings = issueFindingsQuery(finding.issue_key).queryKey;
      void client.cancelQueries({ queryKey: scopedFindings });
      client.setQueryData<Finding[]>(scopedFindings, (current) =>
        current?.map((item) => (item.id === finding.id ? finding : item)),
      );
      void client.cancelQueries({ queryKey: findingsQuery.queryKey, exact: true });
      client.setQueryData(findingsQuery.queryKey, (current) =>
        current?.map((item) => (item.id === finding.id ? finding : item)),
      );
      if (follow_up_issue) cacheCreatedIssue(client, follow_up_issue);
      void client.invalidateQueries({ queryKey: scopedFindings });
      void client.invalidateQueries({ queryKey: findingsQuery.queryKey, exact: true });
      void client.invalidateQueries({ queryKey: issueQuery(finding.issue_key).queryKey });
      void client.invalidateQueries({ queryKey: issueEventsQuery(finding.issue_key).queryKey });
      void client.invalidateQueries({ queryKey: ["issues"] });
      void client.invalidateQueries({ queryKey: ["activity"], exact: true });
    },
    onError: (_error, { finding }) => {
      void client.invalidateQueries({ queryKey: issueFindingsQuery(finding.issue_key).queryKey });
      void client.invalidateQueries({ queryKey: findingsQuery.queryKey, exact: true });
    },
  });
}

export function useAddComment() {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({
      issueKey,
      finding,
      body,
    }: {
      issueKey: string;
      finding?: { round: number; number: number };
      body: string;
    }) => {
      const path = finding
        ? `/api/issues/${encodeURIComponent(issueKey)}/reviews/${finding.round}/findings/${finding.number}/comments`
        : `/api/issues/${encodeURIComponent(issueKey)}/comments`;
      return apiWrite<{ comment: Comment }>(path, "POST", { body });
    },
    onSuccess: ({ comment }, { issueKey, finding }) => {
      const scopedKey = finding
        ? findingCommentsQuery(issueKey, finding.round, finding.number).queryKey
        : issueCommentsQuery(issueKey).queryKey;
      client.setQueryData<Comment[]>(scopedKey, (current) =>
        current && !current.some((item) => item.id === comment.id)
          ? [...current, comment]
          : current,
      );
      void client.invalidateQueries({ queryKey: scopedKey });
      void client.invalidateQueries({ queryKey: issueQuery(issueKey).queryKey });
      void client.invalidateQueries({ queryKey: issueEventsQuery(issueKey).queryKey });
      if (finding)
        void client.invalidateQueries({ queryKey: issueFindingsQuery(issueKey).queryKey });
      void client.invalidateQueries({ queryKey: ["issues"] });
      void client.invalidateQueries({ queryKey: ["activity"], exact: true });
    },
  });
  return (issueKey: string, body: string, finding?: { round: number; number: number }) =>
    mutation.mutateAsync({ issueKey, finding, body });
}
