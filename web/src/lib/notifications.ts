import { issueStatusLabel } from "./issue-status";
import type { ActivityEvent, IssueStatus } from "./types";

/** A native notification's text. */
export type Notification = { title: string; body: string };

/** What the mapping needs to know beyond the events themselves. */
export type NotificationLookup = {
  /** Whether the person signed in here made the change, directly or through their own agent. */
  isMine: (actorId: string) => boolean;
  /** Whether the ID is the person signed in here, not one of their agents. */
  isMe: (actorId: string) => boolean;
  /** The key of an issue by its ID, such as WP-142, or "" when it is not loaded. */
  issueKey: (issueId: string) => string;
  /** Whether the issue is assigned to the person or was created by them. */
  ownsIssue: (issueId: string) => boolean;
  /** The title of an issue by its ID, or "" when it is not loaded. */
  issueTitle: (issueId: string) => string;
  /** The display name of an actor, or "" when it is not loaded. */
  actorName: (actorId: string) => string;
};

const decisions: Record<string, string> = { fix: "Fix", wontfix: "Won't fix", later: "Later" };

/** The new assignee recorded by an issue update, if the update changed it. */
function assignee(payload: Record<string, unknown>): unknown {
  const change = payload.assignee_actor_id;
  return typeof change === "object" && change !== null
    ? (change as { to?: unknown }).to
    : undefined;
}

/** Describes one event worth a notification, or returns null for any other event. */
function describe(event: ActivityEvent, lookup: NotificationLookup): Notification | null {
  if (lookup.isMine(event.actor_id)) return null;

  const payload = event.payload;
  const text = (value: unknown) => (typeof value === "string" ? value : "");
  const issue = lookup.issueKey(text(payload.issue_id));
  const finding = typeof payload.number === "number" ? `R${payload.number}` : "A finding";
  const where = issue ? ` on ${issue}` : "";
  const by = lookup.actorName(event.actor_id);

  switch (event.kind) {
    case "issue.created":
    case "issue.updated": {
      const assigned =
        event.kind === "issue.created" ? payload.assignee_actor_id : assignee(payload);
      if (typeof assigned !== "string" || !lookup.isMe(assigned)) return null;

      const key = text(payload.key) || lookup.issueKey(event.target_id);
      const title = text(payload.title) || lookup.issueTitle(event.target_id);
      return {
        title: `Assigned to you${key ? `: ${key}` : ""}`,
        body: [title, by && `Assigned by ${by}`].filter(Boolean).join(" · "),
      };
    }
    case "finding.added":
      if (!lookup.ownsIssue(text(payload.issue_id))) return null;
      return {
        title: `New finding${where}`,
        body: [text(payload.title), by && `Found by ${by}`].filter(Boolean).join(" · "),
      };
    case "finding.status_changed": {
      if (!lookup.ownsIssue(text(payload.issue_id))) return null;
      const decision = decisions[text(payload.to)];
      if (!decision) return null;
      return {
        title: `${finding}${where}: ${decision}`,
        body: [by && `Decided by ${by}`, text(payload.reason)].filter(Boolean).join(" · "),
      };
    }
    case "issue.status_changed": {
      if (!lookup.ownsIssue(event.target_id)) return null;

      const key = lookup.issueKey(event.target_id);
      const status = issueStatusLabel[text(payload.to) as IssueStatus];
      if (!status) return null;
      return {
        title: `${key || "An issue"} moved to ${status}`,
        body: [lookup.issueTitle(event.target_id), by && `Moved by ${by}`]
          .filter(Boolean)
          .join(" · "),
      };
    }
    case "comment.added": {
      const issueId = text(payload.issue_id);
      if (!lookup.ownsIssue(issueId)) return null;
      return {
        title: `New comment${where}`,
        body: [lookup.issueTitle(issueId), by && `By ${by}`].filter(Boolean).join(" · "),
      };
    }
    case "review_round.finished": {
      if (!lookup.ownsIssue(text(payload.issue_id))) return null;
      const round = typeof payload.number === "number" ? `Round ${payload.number}` : "A round";
      return {
        title: `Review finished${where}`,
        body: [round, by && `By ${by}`].filter(Boolean).join(" · "),
      };
    }
  }
  return null;
}

/**
 * Turns a batch of live events into at most one notification: the event itself when only one is
 * worth telling, or a count when several arrive together.
 */
export function notificationFor(
  events: ActivityEvent[],
  lookup: NotificationLookup,
): Notification | null {
  const notable = events
    .map((event) => describe(event, lookup))
    .filter((item): item is Notification => item !== null);
  if (notable.length === 0) return null;
  if (notable.length === 1) return notable[0] ?? null;
  return {
    title: `${notable.length} updates in Super Sparrow`,
    body: notable.map((item) => item.title).join("\n"),
  };
}
