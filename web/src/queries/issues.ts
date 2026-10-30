import { useMutation, useQueryClient, queryOptions } from "@tanstack/react-query";
import type { NewIssueValues } from "../components/new-issue-modal";
import { useToast } from "../components/toast";
import { apiGet, apiWrite } from "../lib/api";
import type { ChecklistItem, Issue } from "../lib/types";
import { getIssues } from "./resources";
import { resourceQuery } from "./options";

export const issuesQuery = resourceQuery("issues", getIssues);
export const issueQuery = (key: string) =>
  queryOptions({
    queryKey: ["issue", key],
    queryFn: async () =>
      (await apiGet<{ issue: Issue }>(`/api/issues/${encodeURIComponent(key)}`)).issue,
  });

/** Persists issue edits; the server owns revisions and actor attribution. */
export function useUpdateIssue() {
  const client = useQueryClient();
  const { notify } = useToast();
  const mutation = useMutation({
    mutationFn: async ({
      id,
      change,
    }: {
      id: string;
      change: (issue: Issue) => Partial<Issue>;
    }) => {
      const issue = client.getQueryData(issuesQuery.queryKey)?.find((i) => i.id === id);
      if (!issue) throw new Error("Issue is not loaded");
      return apiWrite(`/api/issues/${issue.key}`, "PATCH", change(issue));
    },
    onSuccess: () => client.invalidateQueries(),
    onError: (error) => notify(error.message),
  });
  return (id: string, change: (issue: Issue) => Partial<Issue>) => mutation.mutate({ id, change });
}

export function useSetChecklistItem() {
  const client = useQueryClient();
  const { notify } = useToast();
  const mutation = useMutation({
    mutationFn: ({
      issue,
      item,
      checked,
    }: {
      issue: Issue;
      item: ChecklistItem;
      checked: boolean;
    }) =>
      apiWrite(`/api/issues/${issue.key}/checklist`, "PATCH", {
        items: [{ index: item.index, text: item.text, checked }],
      }),
    onSuccess: () => client.invalidateQueries(),
    onError: (error) => {
      notify(error.message);
      void client.invalidateQueries();
    },
  });
  return (issue: Issue, item: ChecklistItem, checked: boolean) =>
    mutation.mutate({ issue, item, checked });
}

export function useCreateIssue() {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: (v: NewIssueValues) =>
      apiWrite<{ issue: Issue }>(
        `/api/projects/${encodeURIComponent(v.projectKey)}/issues`,
        "POST",
        {
          title: v.title,
          plan: v.plan,
          status: v.status,
          assignee_actor_id: v.assigneeId,
          milestone_id: v.milestoneId,
          label_ids: v.labelIds,
        },
      ),
    onSuccess: () => client.invalidateQueries(),
  });
  return mutation.mutateAsync;
}

/** Patches a loaded issue and updates its detail cache with the committed server response. */
export function usePatchIssue(key: string) {
  const client = useQueryClient();
  const { notify } = useToast();
  const mutation = useMutation({
    scope: { id: `issue:${key}` },
    mutationFn: (change: Partial<Issue>) =>
      apiWrite<{ issue: Issue }>(`/api/issues/${encodeURIComponent(key)}`, "PATCH", change),
    onSuccess: ({ issue }) => {
      client.setQueryData(issueQuery(key).queryKey, (current) =>
        current && current.plan_revision > issue.plan_revision ? current : issue,
      );
      void client.invalidateQueries({ queryKey: issuesQuery.queryKey });
      void client.invalidateQueries({ queryKey: issueQuery(key).queryKey });
      void client.invalidateQueries({ queryKey: ["activity"] });
    },
    onError: (error) => notify(error.message),
  });
  return mutation;
}
