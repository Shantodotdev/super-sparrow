import { afterEach, expect, test } from "bun:test";
import { getEvents, getFindings, getIssues } from "./resources";

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
});

/** Serves only expected API requests; unexpected requests fail the test. */
function responses(pages: Record<string, unknown>) {
  const requests: string[] = [];
  globalThis.fetch = (async (path: string) => {
    requests.push(path);
    if (!(path in pages)) throw new Error(`Unexpected request: ${path}`);
    return Response.json(pages[path]);
  }) as typeof fetch;
  return requests;
}

test("loads every issue page for each project with its actual key", async () => {
  const requests = responses({
    "/api/projects": { projects: [{ key: "WP" }, { key: "SS" }] },
    "/api/projects/WP/issues?limit=200": { issues: [{ key: "WP-201" }], next_before: 201 },
    "/api/projects/WP/issues?limit=200&before=201": {
      issues: [{ key: "WP-1" }],
      next_before: null,
    },
    "/api/projects/SS/issues?limit=200": { issues: [{ key: "SS-1" }], next_before: null },
  });
  expect((await getIssues()).map((i) => i.key)).toEqual(["WP-201", "WP-1", "SS-1"]);
  expect(requests).toHaveLength(4);
});

test("empty projects need no issue or findings requests", async () => {
  const requests = responses({ "/api/projects": { projects: [] } });
  expect(await getIssues()).toEqual([]);
  expect(await getFindings()).toEqual([]);
  expect(requests).toEqual(["/api/projects", "/api/projects"]);
});

test("activity stops when the server repeats its cursor on an empty page", async () => {
  responses({
    "/api/events?limit=200&after=0": { events: [{ sequence: 12 }], next_after: 12 },
    "/api/events?limit=200&after=12": { events: [], next_after: 12 },
  });
  expect((await getEvents()).map((event) => event.sequence)).toEqual([12]);
});
