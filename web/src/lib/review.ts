import { findingSeverities } from "./finding-status";
import type { Actor, Finding, FindingStatus } from "./types";

/** Uses tool names for agents while retaining each actor's identity in the data. */
export function reviewActorName(actor: Actor | undefined): string {
  if (!actor) return "Unknown";
  if (actor.kind === "agent") {
    if (actor.agent_name === "claude-code") return "Claude";
    if (actor.agent_name === "codex") return "Codex";
  }
  return actor.display_name;
}

/** Orders findings by severity, then newest round, then number within the round. */
export function orderFindings(findings: Finding[]): Finding[] {
  return [...findings].sort(
    (left, right) =>
      findingSeverities.indexOf(left.severity) - findingSeverities.indexOf(right.severity) ||
      right.round - left.round ||
      left.number - right.number,
  );
}

/** Orders findings by their round and number, without changing cached data. */
export function orderFindingsByNumber(findings: Finding[]): Finding[] {
  return [...findings].sort(
    (left, right) => left.round - right.round || left.number - right.number,
  );
}

/** Returns the lifecycle moves offered to a signed-in person, matching the server transitions. */
export function findingActions(status: FindingStatus): FindingStatus[] {
  switch (status) {
    case "open":
      return ["fix", "wontfix", "later"];
    case "fix":
      return ["fixed", "open"];
    case "fixed":
      return ["verified", "fix"];
    case "wontfix":
    case "later":
      return ["open"];
    case "verified":
      return [];
  }
}
