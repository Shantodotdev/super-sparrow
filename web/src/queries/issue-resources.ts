import { queryOptions } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import type { ActivityEvent, Comment, Finding, ReviewRound } from "../lib/types";

/** Reads one issue's review rounds without fetching rounds for other issues. */
export const issueReviewRoundsQuery = (issueKey: string) =>
  queryOptions({
    queryKey: ["review-rounds", issueKey.toUpperCase()],
    queryFn: async () =>
      (
        await apiGet<{ rounds: ReviewRound[] }>(
          `/api/issues/${encodeURIComponent(issueKey)}/reviews`,
        )
      ).rounds,
  });

/** Reads one issue's findings (including duplicates) without loading other projects. */
export const issueFindingsQuery = (issueKey: string) =>
  queryOptions({
    queryKey: ["findings", issueKey.toUpperCase()],
    queryFn: async () =>
      (
        await apiGet<{ findings: Finding[] }>(
          `/api/issues/${encodeURIComponent(issueKey)}/findings?include_duplicates=true`,
        )
      ).findings,
  });

/** Reads one issue's own comments. */
export const issueCommentsQuery = (issueKey: string) =>
  queryOptions({
    queryKey: ["comments", issueKey.toUpperCase(), ""],
    queryFn: async () =>
      (
        await apiGet<{ comments: Comment[] }>(
          `/api/issues/${encodeURIComponent(issueKey)}/comments`,
        )
      ).comments,
  });

/** Reads one finding's discussion thread. */
export const findingCommentsQuery = (issueKey: string, round: number, number: number) =>
  queryOptions({
    queryKey: ["comments", issueKey.toUpperCase(), `${round}:${number}`],
    queryFn: async () =>
      (
        await apiGet<{ comments: Comment[] }>(
          `/api/issues/${encodeURIComponent(issueKey)}/reviews/${round}/findings/${number}/comments`,
        )
      ).comments,
  });

/** Reads one issue's event timeline using sequence cursors. */
export const issueEventsQuery = (issueKey: string) =>
  queryOptions({
    queryKey: ["activity", issueKey.toUpperCase()],
    queryFn: async (): Promise<ActivityEvent[]> => {
      const events: ActivityEvent[] = [];
      let after = 0;
      for (;;) {
        const page = await apiGet<{ events: ActivityEvent[]; next_after: number | null }>(
          `/api/issues/${encodeURIComponent(issueKey)}/events?limit=200&after=${after}`,
        );
        events.push(...page.events);
        if (page.next_after === null || page.next_after <= after) return events;
        after = page.next_after;
      }
    },
  });
