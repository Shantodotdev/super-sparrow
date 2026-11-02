import { expect, test } from "bun:test";
import { seedFindings } from "../data/findings";
import { agents } from "../data/team";
import { findingActions, orderFindings, orderFindingsByNumber, reviewActorName } from "./review";
import { validateIssueSearch } from "./search";

test("finding order keeps severe problems first across rounds without mutating cached data", () => {
  const base = seedFindings[0]!;
  const findings = [
    { ...base, id: "nit", severity: "nit" as const, round: 3, number: 1 },
    { ...base, id: "old", severity: "blocking" as const, round: 1, number: 1 },
    { ...base, id: "second", severity: "blocking" as const, round: 2, number: 2 },
    { ...base, id: "first", severity: "blocking" as const, round: 2, number: 1 },
  ];
  expect(orderFindings(findings).map((finding) => finding.id)).toEqual([
    "first",
    "second",
    "old",
    "nit",
  ]);
  expect(findings[0]!.id).toBe("nit");
});

test("issue findings follow round and finding numbers regardless of severity", () => {
  const base = seedFindings[0]!;
  const findings = [
    { ...base, id: "round-2", round: 2, number: 1, severity: "blocking" as const },
    { ...base, id: "round-1-second", round: 1, number: 2, severity: "should_fix" as const },
    { ...base, id: "round-1-first", round: 1, number: 1, severity: "nit" as const },
  ];
  expect(orderFindingsByNumber(findings).map((finding) => finding.id)).toEqual([
    "round-1-first",
    "round-1-second",
    "round-2",
  ]);
  expect(findings[0]!.id).toBe("round-2");
});

test("finding actions follow each lifecycle stage", () => {
  expect(findingActions("open")).toEqual(["fix", "wontfix", "later"]);
  expect(findingActions("fix")).toEqual(["fixed", "open"]);
  expect(findingActions("fixed")).toEqual(["verified", "fix"]);
  expect(findingActions("verified")).toEqual([]);
  expect(findingActions("wontfix")).toEqual(["open"]);
  expect(findingActions("later")).toEqual(["open"]);
});

test("review labels use tool names without changing actor identity", () => {
  expect(reviewActorName(agents.shantoClaude)).toBe("Claude");
  expect(reviewActorName(agents.shantoCodex)).toBe("Codex");
});

test("issue drawer URLs preserve selected records only on review tabs", () => {
  expect(validateIssueSearch({ tab: "rounds", round: "2", finding: "finding-id" })).toEqual({
    tab: "rounds",
    round: 2,
    finding: "finding-id",
    findingStatus: undefined,
  });
  const unrelated = validateIssueSearch({
    tab: "comments",
    round: 2,
    finding: "finding-id",
    findingStatus: "all",
  });
  expect(unrelated.round).toBeUndefined();
  expect(unrelated.finding).toBeUndefined();
  expect(unrelated.findingStatus).toBeUndefined();
  expect(
    validateIssueSearch({ tab: "findings", round: "-1", findingStatus: "all" }).findingStatus,
  ).toBe("all");
  for (const round of ["-1", "0", "1.5", "no", "Infinity", "9007199254740992"]) {
    expect(validateIssueSearch({ tab: "rounds", round }).round).toBeUndefined();
  }
  for (const round of [true, [1], { number: 1 }]) {
    expect(validateIssueSearch({ tab: "rounds", round }).round).toBeUndefined();
  }
});
