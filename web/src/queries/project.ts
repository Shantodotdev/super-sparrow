import { useSuspenseQuery } from "@tanstack/react-query";
import { seedEvents } from "../data/activity";
import { seedComments } from "../data/comments";
import { seedLabels, seedMilestones } from "../data/project";
import { seedReviewRounds } from "../data/review-rounds";
import { seedActors } from "../data/team";
import type { Actor } from "../lib/types";
import { seededQuery } from "./seeded";

export const actorsQuery = seededQuery("actors", seedActors);
export const labelsQuery = seededQuery("labels", seedLabels);
export const milestonesQuery = seededQuery("milestones", seedMilestones);
export const reviewRoundsQuery = seededQuery("review-rounds", seedReviewRounds);
export const commentsQuery = seededQuery("comments", seedComments);
export const activityQuery = seededQuery("activity", seedEvents);

/** Returns a function that finds a person or agent by ID; undefined for "" or an unknown ID. */
export function useActor() {
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const byId = new Map(actors.map((actor) => [actor.id, actor]));

  return (id: string): Actor | undefined => byId.get(id);
}

/** Returns a function that names a label by ID. */
export function useLabelName() {
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const byId = new Map(labels.map((label) => [label.id, label.name]));

  return (id: string): string => byId.get(id) ?? id;
}
