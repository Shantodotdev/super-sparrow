import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import type { Milestone } from "../lib/types";
import { projectMilestonesQuery } from "./project-resources";

/** Saves milestone fields through the authenticated API and refreshes milestone consumers. */
export function useSaveMilestone(projectKey: string) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: ({ id, name }: { id?: string; name: string }) =>
      apiWrite<{ milestone: Milestone }>(
        id
          ? `/api/milestones/${encodeURIComponent(id)}`
          : `/api/projects/${encodeURIComponent(projectKey)}/milestones`,
        id ? "PATCH" : "POST",
        { name },
      ),
    onSuccess: ({ milestone }) => {
      client.setQueryData(projectMilestonesQuery(projectKey).queryKey, (current) =>
        current
          ? [...current.filter((item) => item.id !== milestone.id), milestone].sort(
              (a, b) => a.position - b.position,
            )
          : undefined,
      );
      void client.invalidateQueries({ queryKey: ["milestones"] });
      void client.invalidateQueries({ queryKey: ["activity"] });
    },
  });
}
