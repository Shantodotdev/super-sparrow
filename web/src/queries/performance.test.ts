import { afterEach, expect, test } from "bun:test";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import type { ActivityEvent, Issue } from "../lib/types";
import { cacheCreatedIssue } from "./issue-cache";
import { refreshLiveEvents } from "./live-events";
import {
  allIssuesQuery,
  projectIssuesQuery,
  projectLabelsQuery,
  projectsQuery,
} from "./project-resources";
import { labelsQuery, milestonesQuery } from "./project";
import { seedIssues } from "../data/issues";

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
});

const issue: Issue = {
  ...seedIssues[0]!,
  id: "new-issue",
  key: "WP-201",
  project_id: "wp",
  project_key: "WP",
};

function client() {
  return new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
}

function creation(): ActivityEvent {
  return {
    id: "event",
    sequence: 1,
    project_id: "wp",
    target_type: "issue",
    target_id: issue.id,
    kind: "issue.created",
    actor_id: "teammate",
    reported_model: "",
    payload: { key: issue.key },
    created_at: issue.created_at,
  };
}

test("committed creation updates loaded lists and detail without awaiting active reads", async () => {
  const cache = client();
  cache.setQueryData(["issues"], []);
  cache.setQueryData(["issues", "WP"], []);
  cache.setQueryData(["issues", "SS"], []);
  let release!: (value: Issue[]) => void;
  const observer = new QueryObserver(cache, {
    queryKey: ["issues", "WP"],
    queryFn: () =>
      new Promise<Issue[]>((resolve) => {
        release = resolve;
      }),
  });
  const stop = observer.subscribe(() => {});
  expect(cacheCreatedIssue(cache, issue)).toBeUndefined();
  expect(cache.getQueryData(["issue", issue.key])).toEqual(issue);
  expect(cache.getQueryData(["issues"])).toEqual([issue]);
  expect(cache.getQueryData(["issues", "WP"])).toEqual([issue]);
  expect(cache.getQueryData(["issues", "SS"])).toEqual([]);
  expect(cache.isFetching()).toBe(1);
  release([issue]);
  await cache.refetchQueries({ queryKey: ["issues", "WP"] }, { cancelRefetch: false });
  stop();
  cache.clear();
});

test("creation does not seed incomplete lists and repeated responses do not duplicate issues", () => {
  const cache = client();
  cacheCreatedIssue(cache, issue);
  expect(cache.getQueryData(["issues"])).toBeUndefined();
  cache.setQueryData(["issues", "WP"], [issue]);
  cacheCreatedIssue(cache, issue);
  expect(cache.getQueryData(["issues", "WP"])).toEqual([issue]);
  cache.clear();
});

test("live creation refreshes only affected lists and skips a creation already cached from the POST", () => {
  const cache = client();
  cache.setQueryData(projectsQuery.queryKey, [{ id: "wp", key: "WP", name: "WP", slug: "wp" }]);
  for (const key of [
    ["issues"],
    ["issues", "WP"],
    ["issues", "SS"],
    ["actors"],
    ["me"],
    ["labels"],
    ["milestones"],
    ["activity"],
  ])
    cache.setQueryData(key, []);
  refreshLiveEvents(cache, [creation()]);
  expect(cache.getQueryState(["issues", "WP"])?.isInvalidated).toBe(true);
  expect(cache.getQueryState(["issues", "SS"])?.isInvalidated).toBe(false);
  for (const name of ["actors", "me", "labels", "milestones"])
    expect(cache.getQueryState([name])?.isInvalidated).toBe(false);
  cache.setQueryData(["issues"], [issue]);
  cache.setQueryData(["issues", "WP"], [issue]);
  cache.setQueryData(["issue", issue.key], issue);
  refreshLiveEvents(cache, [creation()]);
  expect(cache.getQueryState(["issues"])?.isInvalidated).toBe(false);
  expect(cache.getQueryState(["issues", "WP"])?.isInvalidated).toBe(false);
  expect(cache.getQueryState(["activity"])?.isInvalidated).toBe(true);
  cache.clear();
});

test("teammate issue edits and finding decisions refresh detail and related resources", () => {
  const cache = client();
  cache.setQueryData(["issue", issue.key], issue);
  for (const name of ["findings", "comments", "review-rounds"]) cache.setQueryData([name], []);
  refreshLiveEvents(cache, [{ ...creation(), kind: "issue.updated" }]);
  expect(cache.getQueryState(["issue", issue.key])?.isInvalidated).toBe(true);
  cache.setQueryData(["issue", issue.key], issue);
  refreshLiveEvents(cache, [
    {
      ...creation(),
      target_type: "finding",
      kind: "finding.status_changed",
      payload: { issue_id: issue.id },
    },
  ]);
  for (const name of ["findings", "comments", "review-rounds"])
    expect(cache.getQueryState([name])?.isInvalidated).toBe(true);
  expect(cache.getQueryState(["issue", issue.key])?.isInvalidated).toBe(true);
  cache.clear();
});

test("project views use only their own resources and aggregate loaders reuse project requests", async () => {
  const cache = client();
  const requests: string[] = [];
  globalThis.fetch = (async (path: string) => {
    requests.push(path);
    if (path === "/api/projects")
      return Response.json({
        projects: [
          { id: "wp", key: "WP" },
          { id: "ss", key: "SS" },
        ],
      });
    if (path.includes("/issues?")) return Response.json({ issues: [], next_before: null });
    if (path.endsWith("/labels")) return Response.json({ labels: [] });
    if (path.endsWith("/milestones")) return Response.json({ milestones: [] });
    throw new Error(`Unexpected request ${path}`);
  }) as typeof fetch;
  await Promise.all([
    cache.fetchQuery(projectIssuesQuery("WP")),
    cache.fetchQuery(projectLabelsQuery("WP")),
  ]);
  expect(requests).toEqual(["/api/projects/WP/issues?limit=200", "/api/projects/WP/labels"]);
  await Promise.all([
    cache.fetchQuery(allIssuesQuery),
    cache.fetchQuery(labelsQuery),
    cache.fetchQuery(milestonesQuery),
  ]);
  expect(requests.filter((path) => path === "/api/projects")).toHaveLength(1);
  expect(requests.filter((path) => path === "/api/projects/WP/issues?limit=200")).toHaveLength(1);
  expect(requests.filter((path) => path === "/api/projects/WP/labels")).toHaveLength(1);
  cache.clear();
});

test("a delayed creation response preserves a newer teammate edit", () => {
  const cache = client();
  const newer = { ...issue, plan_revision: issue.plan_revision + 1, title: "Updated by teammate" };
  cache.setQueryData(["issues", "WP"], [newer]);
  cache.setQueryData(["issue", issue.key], newer);
  cacheCreatedIssue(cache, issue);
  expect(cache.getQueryData(["issue", issue.key])).toEqual(newer);
  expect(cache.getQueryData(["issues", "WP"])).toEqual([newer]);
  cache.clear();
});
