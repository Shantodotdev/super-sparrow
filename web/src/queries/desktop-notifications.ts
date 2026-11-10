import type { QueryClient } from "@tanstack/react-query";
import { desktopInvoke } from "../lib/desktop";
import { notificationFor } from "../lib/notifications";
import type { Actor, ActivityEvent, Issue } from "../lib/types";
import { actorsQuery, meQuery } from "./project";

/** How long live events gather before at most one notification tells about them. */
const notificationWindowMs = 2000;

/**
 * Returns a function that takes live events and, in the desktop app, shows a native notification
 * for new findings and decisions made by others. Events arriving within two seconds
 * share one notification. In a browser it does nothing.
 */
export function desktopNotifier(client: QueryClient): {
  push: (events: ActivityEvent[]) => void;
  stop: () => void;
} {
  const invoke = desktopInvoke();
  const pending: ActivityEvent[] = [];
  let timer: ReturnType<typeof setTimeout> | undefined;

  const flush = () => {
    timer = undefined;
    const events = pending.splice(0);
    const me = client.getQueryData<Actor>(meQuery.queryKey);
    if (!invoke || !me) return;

    const actors = client.getQueryData<Actor[]>(actorsQuery.queryKey) ?? [];
    const issues = client
      .getQueriesData<Issue[]>({ queryKey: ["issues"] })
      .flatMap(([, data]) => (Array.isArray(data) ? data : []));
    const notification = notificationFor(events, {
      isMine: (id) =>
        id === me.id || actors.some((actor) => actor.id === id && actor.owner_actor_id === me.id),
      issueKey: (id) => issues.find((issue) => issue.id === id)?.key ?? "",
      actorName: (id) => actors.find((actor) => actor.id === id)?.display_name ?? "",
    });
    if (notification) void invoke("notify", notification).catch(() => undefined);
  };

  return {
    push: (events) => {
      if (!invoke) return;
      pending.push(...events);
      timer ??= setTimeout(flush, notificationWindowMs);
    },
    stop: () => {
      clearTimeout(timer);
      timer = undefined;
      pending.length = 0;
    },
  };
}
