import { expect, test } from "bun:test";
import { pullRequestStateLabel, validatePullRequestsSearch } from "./pull-requests";

test("the state filter defaults to open and ignores unknown values", () => {
  expect(validatePullRequestsSearch({})).toEqual({ state: "open" });
  expect(validatePullRequestsSearch({ state: "bogus" })).toEqual({ state: "open" });
  expect(validatePullRequestsSearch({ state: 3 })).toEqual({ state: "open" });
  for (const state of ["open", "merged", "closed", "all"] as const) {
    expect(validatePullRequestsSearch({ state })).toEqual({ state });
  }
});

test("a draft is only a draft while it is open", () => {
  expect(pullRequestStateLabel({ state: "open", draft: true })).toBe("Draft");
  expect(pullRequestStateLabel({ state: "open", draft: false })).toBe("Open");
  expect(pullRequestStateLabel({ state: "closed", draft: true })).toBe("Closed");
  expect(pullRequestStateLabel({ state: "merged", draft: false })).toBe("Merged");
});
