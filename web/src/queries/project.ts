import { queryOptions, useSuspenseQuery } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import type { Actor, Comment, Label, Milestone, ReviewRound } from "../lib/types";
import { findingPath, getEvents } from "./resources";
import { findingsQuery } from "./findings";
import { resourceQuery } from "./options";
import {
  allIssuesQuery,
  projectsQuery,
  projectLabelsQuery,
  projectMilestonesQuery,
} from "./project-resources";
export { projectsQuery } from "./project-resources";
export const meQuery = resourceQuery(
  "me",
  async () => (await apiGet<{ actor: Actor }>("/api/me")).actor,
);
export const actorsQuery = resourceQuery(
  "actors",
  async () => (await apiGet<{ actors: Actor[] }>("/api/people")).actors,
);
export const labelsQuery = queryOptions({
  queryKey: ["labels"],
  queryFn: async ({ client }): Promise<Label[]> => {
    const projects = await client.fetchQuery(projectsQuery);
    return (
      await Promise.all(projects.map((p) => client.fetchQuery(projectLabelsQuery(p.key))))
    ).flat();
  },
});
export const milestonesQuery = queryOptions({
  queryKey: ["milestones"],
  queryFn: async ({ client }): Promise<Milestone[]> => {
    const projects = await client.fetchQuery(projectsQuery);
    return (
      await Promise.all(projects.map((p) => client.fetchQuery(projectMilestonesQuery(p.key))))
    ).flat();
  },
});
export const reviewRoundsQuery = queryOptions({
  queryKey: ["review-rounds"],
  queryFn: async ({ client }) =>
    (
      await Promise.all(
        (await client.fetchQuery(allIssuesQuery)).map(
          async (i) =>
            (await apiGet<{ rounds: ReviewRound[] }>(`/api/issues/${i.key}/reviews`)).rounds,
        ),
      )
    ).flat(),
});
export const commentsQuery = queryOptions({
  queryKey: ["comments"],
  queryFn: async ({ client }) => {
    const [issues, findings] = await Promise.all([
      client.fetchQuery(allIssuesQuery),
      client.fetchQuery(findingsQuery),
    ]);
    const paths = [
      ...issues.map((i) => `/api/issues/${i.key}/comments`),
      ...findings.map((f) => `${findingPath(f)}/comments`),
    ];
    return (
      await Promise.all(
        paths.map(async (path) => (await apiGet<{ comments: Comment[] }>(path)).comments),
      )
    ).flat();
  },
});
export const activityQuery = resourceQuery("activity", getEvents);

/** Resolves actor IDs from the server's team directory. */
export function useActor() {
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const byId = new Map(actors.map((actor) => [actor.id, actor]));
  return (id: string): Actor | undefined => byId.get(id);
}

export function useLabelName() {
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const byId = new Map(labels.map((label) => [label.id, label.name]));
  return (id: string): string => byId.get(id) ?? id;
}
