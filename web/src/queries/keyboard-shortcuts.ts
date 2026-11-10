import { queryOptions } from "@tanstack/react-query";
import { apiGet } from "../lib/api";

export type KeyboardPreferences = { revision: number; overrides: Record<string, string> };

/** Scopes cached preferences to the signed-in person; the server derives that person from the session. */
export function keyboardPreferencesQuery(actorID: string) {
  return queryOptions({
    queryKey: ["keyboard-shortcuts", actorID],
    queryFn: () => apiGet<KeyboardPreferences>("/api/me/shortcuts"),
    enabled: !!actorID,
    staleTime: 15_000,
    refetchOnWindowFocus: "always",
    refetchInterval: 30_000,
    retry: false,
  });
}
