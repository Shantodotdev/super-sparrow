import { expect, test } from "bun:test";
import { QueryClient } from "@tanstack/react-query";
import type { ActivityEvent } from "../lib/types";
import { refreshLiveEvents } from "./live-events";

function client() {
  return new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
}

function actorEvent(kind: string, payload: Record<string, unknown> = {}): ActivityEvent {
  return {
    id: `event-${kind}`,
    sequence: 1,
    project_id: "",
    target_type: "actor",
    target_id: "actor-1",
    kind,
    actor_id: "actor-1",
    reported_model: "",
    payload,
    created_at: "2026-11-23T00:00:00Z",
  };
}

test("discord guild approval events invalidate the discord-guilds query", () => {
  for (const kind of ["discord.guild_requested", "discord.guild_approved"]) {
    const cache = client();
    cache.setQueryData(["discord-guilds"], { enabled: true, guilds: [] });
    cache.setQueryData(["actors"], []);

    refreshLiveEvents(cache, [actorEvent(kind, { guild_id: "123456789" })]);

    expect(cache.getQueryState(["discord-guilds"])?.isInvalidated).toBe(true);
    expect(cache.getQueryState(["actors"])?.isInvalidated).toBe(true);
    cache.clear();
  }
});

test("unrelated actor events do not invalidate the discord-guilds query", () => {
  const cache = client();
  cache.setQueryData(["discord-guilds"], { enabled: true, guilds: [] });
  cache.setQueryData(["actors"], []);

  refreshLiveEvents(cache, [actorEvent("actor.updated")]);

  expect(cache.getQueryState(["discord-guilds"])?.isInvalidated).toBe(false);
  expect(cache.getQueryState(["actors"])?.isInvalidated).toBe(true);
  cache.clear();
});
