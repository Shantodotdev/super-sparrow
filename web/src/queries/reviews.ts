import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import type { Finding, FindingSeverity, ReviewRound } from "../lib/types";
import { findingsQuery } from "./findings";
import { reviewRoundsQuery } from "./project";

/** Refreshes derived counts and history after publishing the committed review data. */
function refreshReviews(client: QueryClient) {
  for (const key of ["review-rounds", "findings", "issues", "issue", "activity", "comments"]) {
    void client.invalidateQueries({ queryKey: [key] });
  }
}

/** Starts a review and publishes it before background reads complete. */
export function useStartReview(issueKey: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (coverage: string) =>
      apiWrite<{ round: ReviewRound }>(
        `/api/issues/${encodeURIComponent(issueKey)}/reviews`,
        "POST",
        { plan_coverage: coverage },
      ),
    onSuccess: ({ round }) => {
      void client.cancelQueries({ queryKey: reviewRoundsQuery.queryKey });
      client.setQueryData(reviewRoundsQuery.queryKey, (current) =>
        current ? [...current.filter((item) => item.id !== round.id), round] : [round],
      );
      refreshReviews(client);
    },
  });
}

/** Finishes a round while retaining its findings and their independent lifecycles. */
export function useFinishReview(issueKey: string, round: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (summary: string) =>
      apiWrite<{ round: ReviewRound }>(
        `/api/issues/${encodeURIComponent(issueKey)}/reviews/${round}/finish`,
        "POST",
        { summary },
      ),
    onSuccess: ({ round }) => {
      void client.cancelQueries({ queryKey: reviewRoundsQuery.queryKey });
      client.setQueryData(reviewRoundsQuery.queryKey, (current) =>
        current?.map((item) => (item.id === round.id ? round : item)),
      );
      refreshReviews(client);
    },
  });
}

/** The creation fields accepted by the finding API, without actor or lifecycle metadata. */
export type NewFinding = {
  severity: FindingSeverity;
  title: string;
  problem: string;
  fix: string;
  evidence: string;
  file_path: string;
  line?: number;
};

/** Adds a finding to the active round; the server derives its author from the session. */
export function useCreateFinding(issueKey: string, round: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: NewFinding) =>
      apiWrite<{ finding: Finding }>(
        `/api/issues/${encodeURIComponent(issueKey)}/reviews/${round}/findings`,
        "POST",
        input,
      ),
    onSuccess: ({ finding }) => {
      void client.cancelQueries({ queryKey: findingsQuery.queryKey });
      client.setQueryData(findingsQuery.queryKey, (current) =>
        current ? [...current.filter((item) => item.id !== finding.id), finding] : [finding],
      );
      refreshReviews(client);
    },
  });
}
