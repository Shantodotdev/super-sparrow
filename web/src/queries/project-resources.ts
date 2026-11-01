import { queryOptions } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import type { Issue, Label, Milestone } from "../lib/types";
import { getIssues, getProjects } from "./resources";
import { resourceQuery } from "./options";

export const projectsQuery = resourceQuery("projects", getProjects);

export const projectIssuesQuery = (key: string) =>
  queryOptions({ queryKey: ["issues", key], queryFn: () => getIssues([{ key }]) });
export const projectLabelsQuery = (key: string) =>
  queryOptions({
    queryKey: ["labels", key],
    queryFn: async () =>
      (await apiGet<{ labels: Label[] }>(`/api/projects/${encodeURIComponent(key)}/labels`)).labels,
  });
export const projectMilestonesQuery = (key: string) =>
  queryOptions({
    queryKey: ["milestones", key],
    queryFn: async () =>
      (
        await apiGet<{ milestones: Milestone[] }>(
          `/api/projects/${encodeURIComponent(key)}/milestones`,
        )
      ).milestones,
  });

/** Reads global lists through project caches, sharing requests with project views. */
export const allIssuesQuery = queryOptions({
  queryKey: ["issues"],
  queryFn: async ({ client }): Promise<Issue[]> => {
    const projects = await client.fetchQuery(projectsQuery);
    return (
      await Promise.all(projects.map((p) => client.fetchQuery(projectIssuesQuery(p.key))))
    ).flat();
  },
});
