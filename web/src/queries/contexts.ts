import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import type { ContextNote } from "../lib/types";

/** A project's contexts, newest first. */
export const projectContextsQuery = (projectKey: string) =>
  queryOptions({
    queryKey: ["contexts", projectKey],
    queryFn: async () =>
      (
        await apiGet<{ contexts: ContextNote[] }>(
          `/api/projects/${encodeURIComponent(projectKey)}/contexts?limit=200`,
        )
      ).contexts,
  });

/** One context by its key, such as WP-C3. */
export const contextQuery = (key: string) =>
  queryOptions({
    queryKey: ["context", key.toUpperCase()],
    queryFn: async () =>
      (await apiGet<{ context: ContextNote }>(`/api/contexts/${encodeURIComponent(key)}`)).context,
  });

export type NewContextValues = {
  projectKey: string;
  title: string;
  body: string;
  issueId?: string;
};

/** Stores a committed context in the caches that show it. */
function cacheContext(client: ReturnType<typeof useQueryClient>, saved: ContextNote) {
  client.setQueryData(contextQuery(saved.key).queryKey, saved);
  void client.invalidateQueries({ queryKey: ["contexts", saved.project_key] });
}

/** Creates a context and returns it once the server has committed it. */
export function useCreateContext() {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: (values: NewContextValues) =>
      apiWrite<{ context: ContextNote }>(
        `/api/projects/${encodeURIComponent(values.projectKey)}/contexts`,
        "POST",
        { title: values.title, body: values.body, issue_id: values.issueId ?? "" },
      ),
    onSuccess: ({ context }) => cacheContext(client, context),
  });
  return mutation.mutateAsync;
}

export type ContextChange = {
  title?: string;
  body?: string;
  body_revision?: number;
  issue_id?: string;
};

/** Edits a context. A body edit carries the revision it started from and fails when it is stale. */
export function useUpdateContext(key: string) {
  const client = useQueryClient();
  return useMutation({
    scope: { id: `context:${key}` },
    mutationFn: (change: ContextChange) =>
      apiWrite<{ context: ContextNote }>(
        `/api/contexts/${encodeURIComponent(key)}`,
        "PATCH",
        change,
      ),
    onSuccess: ({ context }) => cacheContext(client, context),
  });
}

/** Deletes a context; only people can. */
export function useDeleteContext(context: ContextNote) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => apiWrite<void>(`/api/contexts/${encodeURIComponent(context.key)}`, "DELETE"),
    onSuccess: () => {
      client.removeQueries({ queryKey: contextQuery(context.key).queryKey });
      void client.invalidateQueries({ queryKey: ["contexts", context.project_key] });
    },
  });
}
