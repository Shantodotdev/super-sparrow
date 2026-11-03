import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import type { IssueStatus, Subissue } from "../lib/types";

export const subissuesQuery = (issueKey: string) =>
  queryOptions({
    queryKey: ["subissues", issueKey],
    queryFn: async () =>
      (
        await apiGet<{ subissues: Subissue[] }>(
          `/api/issues/${encodeURIComponent(issueKey)}/subissues`,
        )
      ).subissues,
  });

/** The fields explicitly changed by a subissue edit, or supplied for a new subissue. */
export type SubissueChanges = {
  id?: string;
  title?: string;
  description?: string;
  description_revision?: number;
  status?: IssueStatus;
  label_ids?: string[];
};

/** Publishes committed subissues immediately and refreshes lists and history in the background. */
export function useSaveSubissue(issueKey: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...content }: SubissueChanges) =>
      apiWrite<{ subissue: Subissue }>(
        `/api/issues/${encodeURIComponent(issueKey)}/subissues${id ? `/${encodeURIComponent(id)}` : ""}`,
        id ? "PATCH" : "POST",
        content,
      ),
    onSuccess: ({ subissue }) => {
      const queryKey = subissuesQuery(issueKey).queryKey;
      void client.cancelQueries({ queryKey, exact: true });
      client.setQueryData(queryKey, (current) => {
        if (!current) return current;
        if (!current.some((item) => item.id === subissue.id)) return [...current, subissue];
        return current.map((item) =>
          item.id === subissue.id && item.description_revision <= subissue.description_revision
            ? subissue
            : item,
        );
      });
      void client.invalidateQueries({ queryKey });
      void client.invalidateQueries({ queryKey: ["activity"] });
    },
  });
}
