import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../components/toast";
import { apiWrite } from "../lib/api";
import type { Finding, FindingStatus } from "../lib/types";
import { issuesQuery } from "./issues";
import { findingPath, getFindings } from "./resources";
import { resourceQuery } from "./options";

export const findingsQuery = resourceQuery("findings", getFindings);
export type Decision = Extract<FindingStatus, "fix" | "wontfix" | "later">;

export function useDecideFinding() {
  const client = useQueryClient();
  const { notify } = useToast();
  const mutation = useMutation({
    mutationFn: ({
      finding,
      status,
      reason,
    }: {
      finding: Finding;
      status: Decision;
      reason: string;
    }) => apiWrite(`${findingPath(finding)}/status`, "POST", { status, reason }),
    onSuccess: () => client.invalidateQueries(),
    onError: (error) => notify(error.message),
  });
  return (finding: Finding, status: Decision, reason = "") =>
    mutation.mutate({ finding, status, reason });
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
