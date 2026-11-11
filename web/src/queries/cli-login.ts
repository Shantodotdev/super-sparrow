import { queryOptions } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import type { CliLoginRequest } from "../lib/types";

/**
 * The CLI sign-in request behind a code. It is read once: deciding it makes the server forget the
 * request, so refetching would only report it as expired.
 */
export function cliLoginRequestQuery(code: string) {
  return queryOptions({
    queryKey: ["cli-login-request", code],
    queryFn: () => apiGet<CliLoginRequest>(`/api/auth/cli/requests/${encodeURIComponent(code)}`),
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
}
