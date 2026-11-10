import type { ActivityEvent } from "./types";

/** A native notification's text. */
export type Notification = { title: string; body: string };

/** What the mapping needs to know beyond the events themselves. */
export type NotificationLookup = {
  /** Whether the person signed in here made the change, directly or through their own agent. */
  isMine: (actorId: string) => boolean;
  /** The key of an issue by its ID, such as WP-142, or "" when it is not loaded. */
  issueKey: (issueId: string) => string;
  /** The display name of an actor, or "" when it is not loaded. */
  actorName: (actorId: string) => string;
};

const decisions: Record<string, string> = { fix: "Fix", wontfix: "Won't fix", later: "Later" };

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
    case "finding.added":
      return {
        title: `New finding${where}`,
        body: [text(payload.title), by && `Found by ${by}`].filter(Boolean).join(" · "),
      };
    case "finding.status_changed": {
      const decision = decisions[text(payload.to)];
      if (!decision) return null;
      return {
        title: `${finding}${where}: ${decision}`,
        body: [by && `Decided by ${by}`, text(payload.reason)].filter(Boolean).join(" · "),
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
