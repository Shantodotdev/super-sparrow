import { useSuspenseQuery } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import type { Actor, Comment, Label, Milestone, ReviewRound } from "../lib/types";
import { findingPath, getEvents, getFindings, getIssues, getProjects } from "./resources";
import { resourceQuery } from "./options";

export const projectsQuery = resourceQuery("projects", getProjects);
export const meQuery = resourceQuery(
  "me",
  async () => (await apiGet<{ actor: Actor }>("/api/me")).actor,
);
export const actorsQuery = resourceQuery(
  "actors",
  async () => (await apiGet<{ actors: Actor[] }>("/api/people")).actors,
);
export const labelsQuery = resourceQuery("labels", async () =>
  (
    await Promise.all(
      (await getProjects()).map(
        async (p) => (await apiGet<{ labels: Label[] }>(`/api/projects/${p.key}/labels`)).labels,
      ),
    )
  ).flat(),
);
export const milestonesQuery = resourceQuery("milestones", async () =>
  (
    await Promise.all(
      (await getProjects()).map(
        async (p) =>
          (await apiGet<{ milestones: Milestone[] }>(`/api/projects/${p.key}/milestones`))
            .milestones,
      ),
    )
  ).flat(),
);
export const reviewRoundsQuery = resourceQuery("review-rounds", async () =>
  (
    await Promise.all(
      (await getIssues()).map(
        async (i) =>
          (await apiGet<{ rounds: ReviewRound[] }>(`/api/issues/${i.key}/reviews`)).rounds,
      ),
    )
  ).flat(),
);
export const commentsQuery = resourceQuery("comments", async () => {
  const [issues, findings] = await Promise.all([getIssues(), getFindings()]);
  const paths = [
    ...issues.map((i) => `/api/issues/${i.key}/comments`),
    ...findings.map((f) => `${findingPath(f)}/comments`),
  ];
  return (
    await Promise.all(
      paths.map(async (path) => (await apiGet<{ comments: Comment[] }>(path)).comments),
    )
  ).flat();
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
