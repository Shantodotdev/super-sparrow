import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { seedFindings } from "../data/findings";
import { useToast } from "../components/toast";
import type { Finding } from "../lib/types";
import { seededQuery } from "./seeded";

export const findingsQuery = seededQuery("findings", seedFindings);

/** Returns a function that records a decision on one finding. */
export function useDecideFinding() {
  const queryClient = useQueryClient();
  const { notify } = useToast();

  return useCallback(
    (finding: Finding, status: string) => {
      queryClient.setQueryData(findingsQuery.queryKey, (findings) =>
        findings?.map((item) =>
          item.issue === finding.issue && item.round === finding.round && item.id === finding.id
            ? { ...item, status }
            : item,
        ),
      );
      notify(`${finding.id} marked ${status.toLowerCase()} in this preview`);
    },
    [queryClient, notify],
  );
}
