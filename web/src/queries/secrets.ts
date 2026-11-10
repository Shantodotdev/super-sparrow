import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import type { DownloadFormat } from "../lib/secrets";
import type {
  NewServiceToken,
  SecretAccessEntry,
  SecretConfigId,
  SecretConfigListing,
  SecretsOverview,
  SecretValue,
  SecretVersion,
  ServiceToken,
} from "../lib/types";
import type { Project } from "./resources";

const accessLogPageSize = 50;

const secretsPath = (projectKey: string) =>
  `/api/projects/${encodeURIComponent(projectKey)}/secrets`;

export const secretsQuery = (projectKey: string) =>
  queryOptions({
    queryKey: ["secrets", projectKey],
    queryFn: () => apiGet<SecretsOverview>(secretsPath(projectKey)),
  });

export const serviceTokensQuery = (projectKey: string) =>
  queryOptions({
    queryKey: ["secrets", projectKey, "service-tokens"],
    queryFn: async () =>
      (
        await apiGet<{ tokens: ServiceToken[] }>(
          `/api/projects/${encodeURIComponent(projectKey)}/service-tokens`,
        )
      ).tokens,
  });

export const secretHistoryQuery = (projectKey: string, config: SecretConfigId, name: string) =>
  queryOptions({
    queryKey: ["secrets", projectKey, "history", config, name],
    queryFn: async () =>
      (
        await apiGet<{ versions: SecretVersion[] }>(
          `${secretsPath(projectKey)}/${config}/history/${encodeURIComponent(name)}`,
        )
      ).versions,
  });

export const accessLogQuery = (projectKey: string, config: SecretConfigId) =>
  infiniteQueryOptions({
    queryKey: ["secrets", projectKey, "access-log", config],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      apiGet<{ entries: SecretAccessEntry[]; next_cursor: string }>(
        `${secretsPath(projectKey)}/access-log?config=${config}&limit=${accessLogPageSize}&cursor=${encodeURIComponent(pageParam)}`,
      ),
    getNextPageParam: (page) => page.next_cursor || undefined,
  });

/** Reads one value. The server logs every call as a reveal, so values are never cached. */
export function fetchSecretValue(
  projectKey: string,
  config: SecretConfigId,
  name: string,
): Promise<SecretValue> {
  return apiGet<SecretValue>(
    `${secretsPath(projectKey)}/${config}/values/${encodeURIComponent(name)}`,
  );
}

/** Reads a whole config in the given format; the server logs it as a download. */
export function downloadPath(
  projectKey: string,
  config: SecretConfigId,
  format: DownloadFormat,
): string {
  return `${secretsPath(projectKey)}/${config}/download?format=${format}`;
}

/** Refreshes the access logs after a reveal, copy, or download, which the server records. */
export function refreshAccessLog(client: QueryClient, projectKey: string) {
  void client.invalidateQueries({ queryKey: ["secrets", projectKey, "access-log"] });
}

/** Refreshes everything a secrets change can alter, including the activity timeline. */
function refreshSecrets(client: QueryClient, projectKey: string) {
  void client.invalidateQueries({ queryKey: ["secrets", projectKey] });
  void client.invalidateQueries({ queryKey: ["activity"] });
}

/** Stores a config listing that the server returned from a write. */
function storeListing(
  client: QueryClient,
  projectKey: string,
  config: SecretConfigId,
  listing: SecretConfigListing,
) {
  client.setQueryData(secretsQuery(projectKey).queryKey, (current) =>
    current?.configs ? { ...current, configs: { ...current.configs, [config]: listing } } : current,
  );
  refreshSecrets(client, projectKey);
}

export function useEnableSecrets(projectKey: string) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: () => apiWrite<{ project: Project }>(`${secretsPath(projectKey)}/enable`, "POST"),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["projects"] });
      refreshSecrets(client, projectKey);
    },
  });
}

export function usePatchSecrets(projectKey: string, config: SecretConfigId) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (body: { set: Record<string, string>; delete: string[] }) =>
      apiWrite<SecretConfigListing>(`${secretsPath(projectKey)}/${config}`, "PATCH", body),
    onSuccess: (listing) => storeListing(client, projectKey, config, listing),
  });
}

export function useRestoreSecret(projectKey: string, config: SecretConfigId, name: string) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (version: number) =>
      apiWrite<SecretConfigListing>(
        `${secretsPath(projectKey)}/${config}/history/${encodeURIComponent(name)}/restore`,
        "POST",
        { version },
      ),
    onSuccess: (listing) => {
      storeListing(client, projectKey, config, listing);
      void client.invalidateQueries({
        queryKey: secretHistoryQuery(projectKey, config, name).queryKey,
      });
    },
  });
}

export function useSetRequiredKeys(projectKey: string, config: SecretConfigId) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (names: string[]) =>
      apiWrite<SecretConfigListing>(`${secretsPath(projectKey)}/${config}/required`, "PUT", {
        names,
      }),
    onSuccess: (listing) => storeListing(client, projectKey, config, listing),
  });
}

export function useCreateServiceToken(projectKey: string) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (body: { config: "staging" | "production"; label: string }) =>
      apiWrite<NewServiceToken>(
        `/api/projects/${encodeURIComponent(projectKey)}/service-tokens`,
        "POST",
        body,
      ),
    onSuccess: () => refreshSecrets(client, projectKey),
  });
}

export function useRenewServiceToken(projectKey: string) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (id: string) =>
      apiWrite<{ token: ServiceToken }>(`/api/service-tokens/${id}/renew`, "POST"),
    onSuccess: () => refreshSecrets(client, projectKey),
  });
}

export function useRevokeServiceToken(projectKey: string) {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => apiWrite(`/api/service-tokens/${id}`, "DELETE"),
    onSuccess: () => refreshSecrets(client, projectKey),
  });
}

export function useSetEnvironmentsPermission() {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (v: { id: string; canManage: boolean }) =>
      apiWrite(`/api/people/${v.id}/environments`, "PATCH", {
        can_manage_environments: v.canManage,
      }),
    onSuccess: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: ["actors"] }),
        client.invalidateQueries({ queryKey: ["secrets"] }),
      ]),
  });
}
