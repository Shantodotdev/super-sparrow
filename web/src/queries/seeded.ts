import { queryOptions } from "@tanstack/react-query";

/**
 * Builds query options for preview data held in memory.
 *
 * The Go API does not serve these resources yet. Swapping `queryFn` for an `apiGet` call later
 * leaves every route loader and component unchanged. The data never goes stale, so a refetch
 * cannot overwrite changes made through the preview mutations.
 */
export function seededQuery<T>(name: string, seed: T) {
  return queryOptions({
    queryKey: [name],
    queryFn: async () => seed,
    staleTime: Infinity,
    gcTime: Infinity,
  });
}
