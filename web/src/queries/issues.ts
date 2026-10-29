import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { seedIssues } from "../data/issues";
import type { NewIssueValues } from "../components/new-issue-modal";
import { useToast } from "../components/toast";
import type { Issue, IssueStatus } from "../lib/types";
import { seededQuery } from "./seeded";

export const issuesQuery = seededQuery("issues", seedIssues);

/** Lists every label an issue can carry, including those no issue uses at the moment. */
export function issueLabelOptions(issues: Issue[]): string[] {
  return [...new Set([...seedIssues, ...issues].flatMap((issue) => issue.labels))].sort();
}

/** Returns a function that changes the status or labels of one issue. */
export function useUpdateIssue() {
  const queryClient = useQueryClient();

  return useCallback(
    (key: string, changes: Partial<Pick<Issue, "status" | "labels">>) => {
      queryClient.setQueryData(issuesQuery.queryKey, (issues) =>
        issues?.map((issue) => (issue.key === key ? { ...issue, ...changes } : issue)),
      );
    },
    [queryClient],
  );
}

/** Returns a function that adds a new issue at the top of the list. */
export function useCreateIssue() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (values: NewIssueValues) => {
      const issues = queryClient.getQueryData(issuesQuery.queryKey);
      if (!issues) throw new Error("create issue: issues are not loaded");

      const key = `SP-${Math.max(...issues.map((issue) => Number(issue.key.split("-")[1]))) + 1}`;
      const issue: Issue = {
        key,
        title: values.title,
        status: values.status as IssueStatus,
        labels: values.labels,
        assignee: values.assignee,
        findings: 0,
        milestone: values.milestone,
        updated: "Just now",
        plan: values.plan,
      };

      queryClient.setQueryData(issuesQuery.queryKey, [issue, ...issues]);
      notify(`${key} created in this preview`);
    },
    [queryClient, notify],
  );
}
