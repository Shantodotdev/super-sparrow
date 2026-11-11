import { describe, expect, test } from "bun:test";
import { notificationFor, type NotificationLookup } from "./notifications";
import type { ActivityEvent } from "./types";

const lookup: NotificationLookup = {
  isMine: (id) => id === "me" || id === "my-agent",
  isMe: (id) => id === "me",
  ownsIssue: (id) => id === "issue-1",
  issueTitle: (id) => (id === "issue-1" ? "Fix the store" : ""),
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
    const myAgent = event("comment.added", "my-agent", {});
    expect(notificationFor([mine, myAgent], lookup)).toBeNull();
  });

  test("summarizes a batch with several notable events", () => {
    const batch = [
      event("finding.added", "agent", { issue_id: "issue-1", title: "One" }),
      event("comment.added", "agent", {}),
      event("finding.status_changed", "teammate", {
        issue_id: "issue-1",
        number: 3,
        to: "wontfix",
        reason: "Later",
      }),
    ];
    expect(notificationFor(batch, lookup)).toEqual({
      title: "2 updates in Super Sparrow",
      body: "New finding on WP-1\nR3 on WP-1: Won't fix",
    });
  });

  test("tells the person when a teammate assigns them an issue", () => {
    const assigned = event("issue.updated", "teammate", {
      assignee_actor_id: { from: "", to: "me" },
    });
    assigned.target_type = "issue";
    assigned.target_id = "issue-1";
    expect(notificationFor([assigned], lookup)).toEqual({
      title: "Assigned to you: WP-1",
      body: "Fix the store · Assigned by Tara",
    });
  });

  test("tells about an issue created already assigned to the person", () => {
    const created = event("issue.created", "agent", {
      key: "WP-9",
      title: "New work",
      assignee_actor_id: "me",
    });
    expect(notificationFor([created], lookup)).toEqual({
      title: "Assigned to you: WP-9",
      body: "New work · Assigned by Claude Code",
    });
  });

  test("ignores assignments to others, self-assignment, and assignment to the person's agent", () => {
    const other = event("issue.updated", "teammate", {
      assignee_actor_id: { from: "", to: "bob" },
    });
    const self = event("issue.updated", "me", { assignee_actor_id: { from: "", to: "me" } });
    const toAgent = event("issue.updated", "teammate", {
      assignee_actor_id: { from: "", to: "my-agent" },
    });
    expect(notificationFor([other, self, toAgent], lookup)).toBeNull();
  });

  test("tells about status changes, comments, and finished reviews on the person's issues", () => {
    const moved = event("issue.status_changed", "teammate", { from: "todo", to: "done" });
    moved.target_id = "issue-1";
    const comment = event("comment.added", "teammate", { issue_id: "issue-1" });
    const review = event("review_round.finished", "agent", { issue_id: "issue-1", number: 2 });
    expect(notificationFor([moved], lookup)).toEqual({
      title: "WP-1 moved to Done",
      body: "Fix the store · Moved by Tara",
    });
    expect(notificationFor([comment], lookup)).toEqual({
      title: "New comment on WP-1",
      body: "Fix the store · By Tara",
    });
    expect(notificationFor([review], lookup)).toEqual({
      title: "Review finished on WP-1",
      body: "Round 2 · By Claude Code",
    });
  });

  test("ignores activity on issues the person does not own", () => {
    const comment = event("comment.added", "teammate", { issue_id: "issue-2" });
    const finding = event("finding.added", "agent", { issue_id: "issue-2", title: "Other" });
    expect(notificationFor([comment, finding], lookup)).toBeNull();
  });
});
