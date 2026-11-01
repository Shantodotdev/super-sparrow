import { queryOptions, type QueryClient } from "@tanstack/react-query";

/** Defines a resource loaded from the Go server. */
export function resourceQuery<T>(name: string, load: () => Promise<T>) {
  return queryOptions({ queryKey: [name], queryFn: load });
}

/** Uses cached data immediately while stale reads refresh in the background. */
export function loadQuery<T>(queryClient: QueryClient, query: ReturnType<typeof resourceQuery<T>>) {
  return queryClient.ensureQueryData({ ...query, revalidateIfStale: true });
}
