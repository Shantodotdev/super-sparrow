import { createAsyncStoragePersister } from "@tanstack/query-async-storage-persister";
import {
  persistQueryClientSubscribe,
  type PersistedClient,
} from "@tanstack/query-persist-client-core";
import { dehydrate, hydrate, type QueryClient } from "@tanstack/react-query";
import type { Actor } from "../lib/types";

export const queryCacheMaxAge = 60 * 60 * 1000;

const storageKey = "super-sparrow:query-cache:v1";
const resources = new Set([
  "me",
  "projects",
  "issues",
  "issue",
  "subissues",
  "actors",
  "labels",
  "milestones",
  "findings",
  "review-rounds",
  "comments",
  "activity",
  "tokens",
  "sessions",
]);

type CacheSession = {
  actorID: string;
  stop: () => void;
};

const sessions = new WeakMap<QueryClient, CacheSession>();

const dehydrateOptions = {
  shouldDehydrateMutation: () => false,
  shouldDehydrateQuery: (query: { queryKey: readonly unknown[]; state: { data: unknown } }) =>
    resources.has(String(query.queryKey[0])) && query.state.data !== undefined,
};

/** Evicts the oldest resource while preserving the profile required to restore the remaining cache. */
function evictOldestResource({
  persistedClient,
}: {
  persistedClient: PersistedClient;
}): PersistedClient | undefined {
  const oldest = persistedClient.clientState.queries
    .filter((query) => !(query.queryKey.length === 1 && query.queryKey[0] === "me"))
    .sort((left, right) => left.state.dataUpdatedAt - right.state.dataUpdatedAt)[0];
  if (!oldest) return undefined;

  return {
    ...persistedClient,
    clientState: {
      ...persistedClient.clientState,
      queries: persistedClient.clientState.queries.filter((query) => query !== oldest),
    },
  };
}

/** Restores a recent display cache synchronously; the cached profile never authorizes API requests. */
export function restoreQueryCache(client: QueryClient): void {
  if (typeof window === "undefined") return;

  try {
    const raw = window.sessionStorage.getItem(storageKey);
    if (!raw) return;

    const saved = JSON.parse(raw) as PersistedClient;
    const age = Date.now() - saved.timestamp;
    if (!Number.isFinite(age) || age < 0 || age > queryCacheMaxAge)
      throw new Error("Expired display cache");

    const queries = saved.clientState.queries
      .filter(
        (query) =>
          Array.isArray(query.queryKey) &&
          resources.has(String(query.queryKey[0])) &&
          query.state.data !== undefined &&
          Number.isFinite(query.state.dataUpdatedAt),
      )
      .map((query) => ({
        ...query,
        state: {
          ...query.state,
          status: "success" as const,
          fetchStatus: "idle" as const,
          error: null,
          fetchFailureReason: null,
          fetchFailureCount: 0,
        },
      }));
    const profile = queries.find(
      (query) => query.queryKey.length === 1 && query.queryKey[0] === "me",
    )?.state.data as Actor | undefined;
    if (
      !profile ||
      profile.kind !== "human" ||
      typeof profile.id !== "string" ||
      !profile.id ||
      saved.buster !== `v3:${profile.id}`
    )
      throw new Error("Invalid display cache identity");

    // Hydrate before the router mounts, then revalidate reads without clearing their displayed data.
    hydrate(client, { mutations: [], queries });
    void client.invalidateQueries({ refetchType: "none" });
    persistQueryCache(client, profile.id);
  } catch {
    discardQueryCache(client);
  }
}

/** Persists display data and metadata, returning false if a changed account required clearing it. */
export function persistQueryCache(client: QueryClient, actorID: string): boolean {
  const existing = sessions.get(client);
  if (existing) {
    if (existing.actorID === actorID) return true;
    clearQueryCache(client);
    return false;
  }
  if (typeof window === "undefined") return true;

  let storage: Storage;
  try {
    storage = window.sessionStorage;
  } catch {
    return true;
  }

  // Retiring the storage adapter also blocks any delayed writes after sign-out.
  let active = true;
  const persister = createAsyncStoragePersister({
    storage: {
      getItem: (key) => (active ? storage.getItem(key) : null),
      setItem: (key, value) => {
        if (active) storage.setItem(key, value);
      },
      removeItem: (key) => {
        if (active) storage.removeItem(key);
      },
    },
    key: storageKey,
    throttleTime: 500,
    retry: evictOldestResource,
  });
  const buster = `v3:${actorID}`;
  const unsubscribe = persistQueryClientSubscribe({
    queryClient: client,
    persister,
    buster,
    dehydrateOptions,
  });

  // Flush on reload so the latest rendered reads do not wait for the throttled write.
  const flush = () => {
    if (!active) return;

    let snapshot: PersistedClient | undefined = {
      timestamp: Date.now(),
      buster,
      clientState: dehydrate(client, dehydrateOptions),
    };
    while (snapshot) {
      try {
        storage.setItem(storageKey, JSON.stringify(snapshot));
        return;
      } catch {
        snapshot = evictOldestResource({ persistedClient: snapshot });
      }
    }
  };

  sessions.set(client, {
    actorID,
    stop: () => {
      active = false;
      unsubscribe();
      window.removeEventListener("pagehide", flush);
    },
  });
  window.addEventListener("pagehide", flush);
  return true;
}

/** Discards unusable query data without hiding a rendered preview while authentication recovers. */
function discardQueryCache(client: QueryClient): void {
  sessions.get(client)?.stop();
  sessions.delete(client);
  client.clear();

  try {
    if (typeof window !== "undefined") window.sessionStorage.removeItem(storageKey);
  } catch {
    // Blocked browser storage must not prevent sign-out.
  }
}

/** Stops pending persistence writes and removes both caches when a session ends. */
export function clearQueryCache(client: QueryClient): void {
  if (typeof window !== "undefined") window.dispatchEvent(new Event("sparrow:clear-cache"));
  discardQueryCache(client);
}
