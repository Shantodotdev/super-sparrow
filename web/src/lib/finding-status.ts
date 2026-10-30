import type { FindingSeverity, FindingStatus } from "./types";

export const findingSeverities: FindingSeverity[] = ["blocking", "should_fix", "nit"];

export const severityLabel: Record<FindingSeverity, string> = {
  blocking: "Blocking",
  should_fix: "Should fix",
  nit: "Nit",
};

export const findingStatuses: FindingStatus[] = [
  "open",
  "fix",
  "wontfix",
  "later",
  "fixed",
  "verified",
];

export const findingStatusLabel: Record<FindingStatus, string> = {
  open: "Open",
  fix: "Fix",
  wontfix: "Won't fix",
  later: "Later",
  fixed: "Fixed",
  verified: "Verified",
};

/**
 * The statuses the server counts as unresolved: waiting for a decision, decided to be fixed, or
 * fixed but not yet verified. They match store.UnresolvedFindingStatuses.
 */
export const unresolvedStatuses: FindingStatus[] = ["open", "fix", "fixed"];

/** Reports whether a finding still needs attention. Duplicates are settled through the original. */
export function isUnresolved(finding: { status: FindingStatus; duplicate_of_finding_id: string }) {
  return unresolvedStatuses.includes(finding.status) && finding.duplicate_of_finding_id === "";
}
