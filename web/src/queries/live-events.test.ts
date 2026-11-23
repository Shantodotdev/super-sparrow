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

test("discord events invalidate the discord-links query", () => {
  for (const kind of ["discord.setup", "discord.unlinked", "discord.guild_revoked"]) {
    const cache = client();
    cache.setQueryData(["discord-links"], { links: [] });

    refreshLiveEvents(cache, [actorEvent(kind)]);

    expect(cache.getQueryState(["discord-links"])?.isInvalidated).toBe(true);
    cache.clear();
  }
});

test("discord link events leave the discord-guilds query alone", () => {
  const cache = client();
  cache.setQueryData(["discord-guilds"], { enabled: true, guilds: [] });
  cache.setQueryData(["discord-links"], { links: [] });

  refreshLiveEvents(cache, [actorEvent("discord.unlinked", { guild_id: "1" })]);

  expect(cache.getQueryState(["discord-guilds"])?.isInvalidated).toBe(false);
  expect(cache.getQueryState(["discord-links"])?.isInvalidated).toBe(true);
  cache.clear();
});

test("other actor events do not invalidate the discord-links query", () => {
  const cache = client();
  cache.setQueryData(["discord-links"], { links: [] });

  refreshLiveEvents(cache, [actorEvent("actor.updated")]);

  expect(cache.getQueryState(["discord-links"])?.isInvalidated).toBe(false);
  cache.clear();
});

test("github events invalidate pull requests and the github settings queries", () => {
  const cache = client();
  cache.setQueryData(["pull-requests", "WP", "open"], []);
  cache.setQueryData(["github"], {});
  cache.setQueryData(["github-accounts"], []);
  cache.setQueryData(["milestones"], []);

  refreshLiveEvents(cache, [
    { ...actorEvent("github.pull_request.opened"), target_type: "project", project_id: "p9" },
  ]);

  expect(cache.getQueryState(["pull-requests", "WP", "open"])?.isInvalidated).toBe(true);
  expect(cache.getQueryState(["github"])?.isInvalidated).toBe(true);
  expect(cache.getQueryState(["github-accounts"])?.isInvalidated).toBe(true);
  cache.clear();
});

test("other events leave the github queries alone", () => {
  const cache = client();
  cache.setQueryData(["pull-requests", "WP", "open"], []);
  cache.setQueryData(["github"], {});

  refreshLiveEvents(cache, [actorEvent("actor.updated")]);

  expect(cache.getQueryState(["pull-requests", "WP", "open"])?.isInvalidated).toBe(false);
  expect(cache.getQueryState(["github"])?.isInvalidated).toBe(false);
  cache.clear();
});
