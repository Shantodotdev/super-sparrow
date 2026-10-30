import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { seedComments } from "../data/comments";
import { seedFindings } from "../data/findings";
import { currentPerson } from "../data/team";
import { useToast } from "../components/toast";
import { findingStatusLabel, isUnresolved } from "../lib/finding-status";
import type { Comment, Finding, FindingStatus } from "../lib/types";
import { commentsQuery } from "./project";
import { issuesQuery } from "./issues";
import { seededQuery } from "./seeded";

const findingsWithCounts = seedFindings.map((finding) => ({
  ...finding,
  comment_count: seedComments.filter((comment) => comment.finding_id === finding.id).length,
}));

export const findingsQuery = seededQuery("findings", findingsWithCounts);

/** The decisions only a person makes on a finding. */
export type Decision = Extract<FindingStatus, "fix" | "wontfix" | "later">;

/**
 * Returns a function that records the signed-in person's decision on one finding, as PATCH
 * /api/findings/{id} does, and keeps its issue's count of unresolved findings current.
 */
export function useDecideFinding() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (finding: Finding, status: Decision, reason = "") => {
      const now = new Date().toISOString();
      const decided: Finding = {
        ...finding,
        status,
        decision_reason: reason,
        decided_by_actor_id: currentPerson.id,
        decided_at: now,
        updated_at: now,
      };

      queryClient.setQueryData(findingsQuery.queryKey, (findings) =>
        findings?.map((item) => (item.id === finding.id ? decided : item)),
      );
      const change = Number(isUnresolved(decided)) - Number(isUnresolved(finding));
      queryClient.setQueryData(issuesQuery.queryKey, (issues) =>
        issues?.map((issue) =>
          issue.id === finding.issue_id
            ? { ...issue, unresolved_findings: issue.unresolved_findings + change }
            : issue,
        ),
      );
      notify(`${finding.label} marked ${findingStatusLabel[status].toLowerCase()} in this preview`);
    },
    [queryClient, notify],
  );
}

/** Returns a function that adds the signed-in person's comment to an issue or one of its findings. */
export function useAddComment() {
  const queryClient = useQueryClient();

  return useCallback(
    (issueId: string, findingId: string, body: string) => {
      const now = new Date().toISOString();
      const comment: Comment = {
        id: `comment-${Date.now()}`,
        issue_id: issueId,
        finding_id: findingId,
        parent_comment_id: "",
        body,
        author_actor_id: currentPerson.id,
        edited_at: null,
        created_at: now,
        updated_at: now,
      };

      queryClient.setQueryData(commentsQuery.queryKey, (comments) => [
        ...(comments ?? []),
        comment,
      ]);
      if (findingId) {
        queryClient.setQueryData(findingsQuery.queryKey, (findings) =>
          findings?.map((f) =>
            f.id === findingId ? { ...f, comment_count: f.comment_count + 1 } : f,
          ),
        );
      } else {
        queryClient.setQueryData(issuesQuery.queryKey, (issues) =>
          issues?.map((i) => (i.id === issueId ? { ...i, comment_count: i.comment_count + 1 } : i)),
        );
      }
    },
    [queryClient],
  );
}
