import { queryOptions, type QueryClient } from "@tanstack/react-query";

/** Defines a resource loaded from the Go server. */
export function resourceQuery<T>(name: string, load: () => Promise<T>) {
  return queryOptions({ queryKey: [name], queryFn: load });
}

/** Loads a resource before rendering, respecting its cache freshness. */
export function loadQuery<T>(queryClient: QueryClient, query: ReturnType<typeof resourceQuery<T>>) {
  return queryClient.fetchQuery(query);
}
