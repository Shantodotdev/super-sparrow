import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { seedComments } from "../data/comments";
import { seedFindings } from "../data/findings";
import { newIssue, seedIssues } from "../data/issues";
import { currentPerson } from "../data/team";
import type { NewIssueValues } from "../components/new-issue-modal";
import { useToast } from "../components/toast";
import { checklistItems, setChecked } from "../lib/checklist";
import { isUnresolved } from "../lib/finding-status";
import type { ChecklistItem, Issue } from "../lib/types";
import { seededQuery } from "./seeded";

// The server counts unresolved findings and issue comments when it reads an issue; the preview
// counts them once from its sample data, and its changes keep them current.
const issuesWithCounts = seedIssues.map((issue) => ({
  ...issue,
  unresolved_findings: seedFindings.filter((f) => f.issue_id === issue.id && isUnresolved(f))
    .length,
  comment_count: seedComments.filter((c) => c.issue_id === issue.id && c.finding_id === "").length,
}));

export const issuesQuery = seededQuery("issues", issuesWithCounts);

/** Returns a function that changes one issue's fields in the preview. */
export function useUpdateIssue() {
  const queryClient = useQueryClient();

  return useCallback(
    (id: string, change: (issue: Issue) => Partial<Issue>) => {
      queryClient.setQueryData(issuesQuery.queryKey, (issues) =>
        issues?.map((issue) =>
          issue.id === id
            ? { ...issue, ...change(issue), updated_at: new Date().toISOString() }
            : issue,
        ),
      );
    },
    [queryClient],
  );
}

/**
 * Returns a function that ticks or unticks one checklist item, as PATCH
 * /api/issues/{key}/checklist does: it rewrites that line of the plan and moves the plan to its
 * next revision.
 */
export function useSetChecklistItem() {
  const updateIssue = useUpdateIssue();

  return useCallback(
    (issue: Issue, item: ChecklistItem, checked: boolean) =>
      updateIssue(issue.id, (current) => {
        const plan = setChecked(current.plan ?? "", item.index, checked);
        const checklist = checklistItems(plan);
        return {
          plan,
          checklist,
          plan_revision: current.plan_revision + 1,
          checklist_progress: {
            done: checklist.filter((i) => i.checked).length,
            total: checklist.length,
          },
        };
      }),
    [updateIssue],
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

      const now = new Date().toISOString();
      const issue = newIssue({
        number: Math.max(...issues.map((item) => item.number)) + 1,
        title: values.title,
        status: values.status,
        priority: values.priority,
        labelIds: values.labelIds,
        assigneeId: values.assigneeId,
        milestoneId: values.milestoneId,
        plan: values.plan,
        createdById: currentPerson.id,
        createdAt: now,
        updatedAt: now,
      });

      queryClient.setQueryData(issuesQuery.queryKey, [issue, ...issues]);
      notify(`${issue.key} created in this preview`);
    },
    [queryClient, notify],
  );
}
