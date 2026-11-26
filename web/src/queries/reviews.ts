import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import type { Finding, FindingSeverity, ReviewRound } from "../lib/types";
import { findingsQuery } from "./findings";
import { issueEventsQuery, issueFindingsQuery, issueReviewRoundsQuery } from "./issue-resources";
import { issueQuery } from "./issues";
import { reviewRoundsQuery } from "./project";

/** Refreshes derived counts and history for one issue after publishing committed review data. */
function refreshReviews(client: QueryClient, issueKey: string) {
  void client.invalidateQueries({ queryKey: issueReviewRoundsQuery(issueKey).queryKey });
  void client.invalidateQueries({ queryKey: issueFindingsQuery(issueKey).queryKey });
  void client.invalidateQueries({ queryKey: issueQuery(issueKey).queryKey });
  void client.invalidateQueries({ queryKey: issueEventsQuery(issueKey).queryKey });
  void client.invalidateQueries({ queryKey: ["issues"] });
  void client.invalidateQueries({ queryKey: ["activity"], exact: true });
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
      const scopedKey = issueReviewRoundsQuery(issueKey).queryKey;
      void client.cancelQueries({ queryKey: scopedKey });
      client.setQueryData<ReviewRound[]>(scopedKey, (current) =>
        current ? [...current.filter((item) => item.id !== round.id), round] : [round],
      );
      client.setQueryData(reviewRoundsQuery.queryKey, (current) =>
        current ? [...current.filter((item) => item.id !== round.id), round] : current,
      );
      refreshReviews(client, issueKey);
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
      const scopedKey = issueReviewRoundsQuery(issueKey).queryKey;
      void client.cancelQueries({ queryKey: scopedKey });
      client.setQueryData<ReviewRound[]>(scopedKey, (current) =>
        current?.map((item) => (item.id === round.id ? round : item)),
      );
      client.setQueryData(reviewRoundsQuery.queryKey, (current) =>
        current?.map((item) => (item.id === round.id ? round : item)),
      );
      refreshReviews(client, issueKey);
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
      const scopedKey = issueFindingsQuery(issueKey).queryKey;
      void client.cancelQueries({ queryKey: scopedKey });
      client.setQueryData<Finding[]>(scopedKey, (current) =>
        current ? [...current.filter((item) => item.id !== finding.id), finding] : [finding],
      );
      client.setQueryData(findingsQuery.queryKey, (current) =>
        current ? [...current.filter((item) => item.id !== finding.id), finding] : current,
      );
      refreshReviews(client, issueKey);
    },
  });
}
