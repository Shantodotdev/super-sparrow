import { QueryCache, QueryClient } from "@tanstack/react-query";
import { createRouter } from "@tanstack/react-router";
import { ApiError } from "./lib/api";
import type { Actor, NavSection } from "./lib/types";
import { routeTree } from "./routeTree.gen";
import {
  clearQueryCache,
  persistQueryCache,
  queryCacheMaxAge,
  restoreQueryCache,
} from "./queries/persistence";

/** Retries only failures that may be transient; a 4xx answer will not change on retry. */
function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status < 500) return false;

  return failureCount < 2;
}

/** Creates the router with a shared TanStack Query client in its context. */
export function getRouter() {
  const queryClient = new QueryClient({
    queryCache: new QueryCache({
      onSuccess: (data, query) => {
        if (query.queryKey.length !== 1 || query.queryKey[0] !== "me") return;
        if (!persistQueryCache(queryClient, (data as Actor).id)) window.location.reload();
      },
      onError: (error) => {
        if (
          error instanceof ApiError &&
          error.status === 401 &&
          typeof window !== "undefined" &&
          window.location.pathname !== "/signin"
        ) {
          clearQueryCache(queryClient);
          window.location.replace("/signin");
        }
      },
    }),
    defaultOptions: {
      queries: { staleTime: 30_000, gcTime: queryCacheMaxAge, retry: shouldRetry },
    },
  });

  if (typeof window !== "undefined" && window.location.pathname.replace(/\/+$/, "") !== "/signin")
    restoreQueryCache(queryClient);

  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: "intent",
    defaultPendingComponent: () => null,
    defaultPendingMs: 0,
    defaultPendingMinMs: 0,
    // Loaders always run on preload; TanStack Query decides whether cached data is fresh enough.
    defaultPreloadStaleTime: 0,
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof getRouter>;
  }

  /** Lets a page route name its sidebar section, or opt out of the app shell with `bare`. */
  interface StaticDataRouteOption {
    section?: NavSection;
    bare?: boolean;
  }
}
