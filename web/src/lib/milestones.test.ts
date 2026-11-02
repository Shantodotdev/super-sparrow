import { describe, expect, test } from "bun:test";
import { milestoneProgressById, validateMilestonesSearch } from "./milestones";
import type { IssueStatus } from "./types";

describe("milestoneProgressById", () => {
  test("partitions every issue status without treating canceled work as done", () => {
    const statuses: IssueStatus[] = [
      "done",
      "canceled",
      "in_progress",
      "in_review",
      "backlog",
      "todo",
    ];
    const progress = milestoneProgressById(
      statuses.map((status) => ({ milestone_id: "launch", status })),
    );
    expect(progress.get("launch")).toEqual({
      total: 6,
      done: 1,
      canceled: 1,
      active: 2,
      remaining: 2,
    });
  });

  test("keeps milestones separate and ignores issues with no milestone", () => {
    const progress = milestoneProgressById([
      { milestone_id: "launch", status: "done" },
      { milestone_id: "later", status: "todo" },
      { milestone_id: "", status: "in_progress" },
    ]);
    expect(progress.size).toBe(2);
    expect(progress.get("launch")).toEqual({
      total: 1,
      done: 1,
      canceled: 0,
      active: 0,
      remaining: 0,
    });
    expect(progress.get("later")).toEqual({
      total: 1,
      done: 0,
      canceled: 0,
      active: 0,
      remaining: 1,
    });
  });

  test("returns no invented progress for milestones with no issues", () => {
    expect(milestoneProgressById([]).size).toBe(0);
  });
});

describe("validateMilestonesSearch", () => {
  test("retains text and supported statuses for reloadable filters", () => {
    expect(validateMilestonesSearch({ q: "Foundation", status: "active" })).toEqual({
      q: "Foundation",
      status: "active",
    });
    expect(validateMilestonesSearch({ status: "planned" }).status).toBe("planned");
    expect(validateMilestonesSearch({ status: "done" }).status).toBe("done");
  });

  test("drops unknown statuses, non-string search text, and extra fields", () => {
    expect(validateMilestonesSearch({ q: ["text"], status: "complete", actor: "someone" })).toEqual(
      { q: undefined, status: undefined },
    );
    expect(validateMilestonesSearch({ q: "", status: "all" })).toEqual({
      q: undefined,
      status: undefined,
    });
  });
});
