import { queryOptions } from "@tanstack/react-query";
import { apiGet } from "../lib/api";
import type { GitHubAccount, GitHubInventory, PullRequest, PullRequestFilter } from "../lib/types";

/** A project's pull requests in one state, newest updated first. The project key leads so live events can refresh by project. */
export const pullRequestsQuery = (projectKey: string, state: PullRequestFilter) =>
  queryOptions({
    queryKey: ["pull-requests", projectKey, state],
    queryFn: async () =>
      (
        await apiGet<{ pull_requests: PullRequest[] }>(
          `/api/projects/${encodeURIComponent(projectKey)}/pull-requests?state=${state}`,
        )
      ).pull_requests,
  });

/** Repositories, linked accounts, and Discord channels; owners only. */
export const githubInventoryQuery = queryOptions({
  queryKey: ["github"],
  queryFn: () => apiGet<GitHubInventory>("/api/github"),
  retry: false,
});

/** The GitHub usernames linked to teammates, readable by everyone signed in. */
export const githubAccountsQuery = queryOptions({
  queryKey: ["github-accounts"],
  queryFn: async () =>
    (await apiGet<{ accounts: GitHubAccount[] }>("/api/github/accounts")).accounts,
  retry: false,
});
