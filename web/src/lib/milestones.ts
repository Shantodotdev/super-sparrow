import type { Issue, MilestoneStatus } from "./types";

export const milestoneStatuses = ["planned", "active", "done"] as const;

export const milestoneStatusLabel: Record<MilestoneStatus, string> = {
  planned: "Planned",
  active: "In progress",
  done: "Completed",
};

export type MilestonesSearch = { q?: string; status?: MilestoneStatus };

export function validateMilestonesSearch(search: Record<string, unknown>): MilestonesSearch {
  return {
    q: typeof search.q === "string" && search.q !== "" ? search.q : undefined,
    status: milestoneStatuses.find((status) => status === search.status),
  };
}

export type MilestoneProgress = {
  total: number;
  done: number;
  canceled: number;
  active: number;
  remaining: number;
};

/** Counts each milestone's issues once, keeping canceled work separate from completed work. */
export function milestoneProgressById(
  issues: Pick<Issue, "milestone_id" | "status">[],
): Map<string, MilestoneProgress> {
  const progress = new Map<string, MilestoneProgress>();

  for (const issue of issues) {
    if (!issue.milestone_id) continue;
    const counts = progress.get(issue.milestone_id) ?? {
      total: 0,
      done: 0,
      canceled: 0,
      active: 0,
      remaining: 0,
    };
    counts.total++;
    if (issue.status === "done") counts.done++;
    else if (issue.status === "canceled") counts.canceled++;
    else if (issue.status === "in_progress" || issue.status === "in_review") counts.active++;
    else counts.remaining++;
    progress.set(issue.milestone_id, counts);
  }

  return progress;
}
