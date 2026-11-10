import { describe, expect, test } from "bun:test";
import { notificationFor, type NotificationLookup } from "./notifications";
import type { ActivityEvent } from "./types";

const lookup: NotificationLookup = {
  isMine: (id) => id === "me" || id === "my-agent",
  issueKey: (id) => (id === "issue-1" ? "WP-1" : ""),
  actorName: (id) => ({ teammate: "Tara", agent: "Claude Code" })[id] ?? "",
};

function event(kind: string, actor: string, payload: Record<string, unknown>): ActivityEvent {
  return {
    sequence: 1,
    id: "event",
    project_id: "project",
    target_type: "finding",
    target_id: "finding",
    kind,
    actor_id: actor,
    reported_model: "",
    payload,
    created_at: "2026-11-10T00:00:00.000Z",
  };
}

describe("notificationFor", () => {
  test("tells about a finding someone else added", () => {
    const added = event("finding.added", "agent", {
      issue_id: "issue-1",
      number: 3,
      title: "Race in the store",
    });
    expect(notificationFor([added], lookup)).toEqual({
      title: "New finding on WP-1",
      body: "Race in the store · Found by Claude Code",
    });
  });

  test("tells about decisions but not other status changes", () => {
    const decided = event("finding.status_changed", "teammate", {
      issue_id: "issue-1",
      number: 2,
      to: "wontfix",
      reason: "Out of scope",
    });
    expect(notificationFor([decided], lookup)).toEqual({
      title: "R2 on WP-1: Won't fix",
      body: "Decided by Tara · Out of scope",
    });
    const fixed = event("finding.status_changed", "teammate", { number: 2, to: "fixed" });
    expect(notificationFor([fixed], lookup)).toBeNull();
  });

  test("skips the person's own changes and their agents' changes", () => {
    const mine = event("finding.added", "me", { title: "Mine" });
    const myAgent = event("context.created", "my-agent", { title: "Notes" });
    expect(notificationFor([mine, myAgent], lookup)).toBeNull();
  });

  test("summarizes a batch with several notable events", () => {
    const batch = [
      event("finding.added", "agent", { issue_id: "issue-1", title: "One" }),
      event("comment.added", "agent", {}),
      event("context.created", "teammate", { title: "Inspiration" }),
    ];
    expect(notificationFor(batch, lookup)).toEqual({
      title: "2 updates in Super Sparrow",
      body: "New finding on WP-1\nNew context",
    });
  });
});
