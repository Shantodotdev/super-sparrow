import { QueryClient } from "@tanstack/react-query";
import { createRouter } from "@tanstack/react-router";
import { ApiError } from "./lib/api";
import type { NavSection } from "./lib/types";
import { routeTree } from "./routeTree.gen";

/** Retries only failures that may be transient; a 4xx answer will not change on retry. */
function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status < 500) return false;

  return failureCount < 2;
}

/** Creates the router with a shared TanStack Query client in its context. */
export function getRouter() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { staleTime: 30_000, retry: shouldRetry } },
  });

  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: "intent",
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
