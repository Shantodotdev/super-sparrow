import { queryOptions } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import { activeIssueStatuses } from "../lib/issue-status";
import type { Issue, Label, Milestone } from "../lib/types";
import { getIssues, getProjects } from "./resources";
import { resourceQuery } from "./options";

export const projectsQuery = resourceQuery("projects", getProjects);

export const projectIssuesQuery = (key: string) =>
  queryOptions({ queryKey: ["issues", key], queryFn: () => getIssues([{ key }]) });

/**
 * Preloads every active issue (excluding Done and Canceled) in a project with its full plan and
 * checklist, seeding each issue's detail cache so opening an active issue does not wait on a fetch.
 */
export const projectActiveIssuesQuery = (key: string) =>
  queryOptions({
    queryKey: ["issues", key, "active"],
    queryFn: async ({ client }): Promise<Issue[]> => {
      const issues = await getIssues([{ key }], {
        statuses: activeIssueStatuses,
        includePlan: true,
      });
      for (const issue of issues) {
        client.setQueryData<Issue>(["issue", issue.key], (current) =>
          current &&
          (current.plan_revision > issue.plan_revision || current.updated_at > issue.updated_at)
            ? current
            : issue,
        );
      }
      return issues;
    },
  });
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
