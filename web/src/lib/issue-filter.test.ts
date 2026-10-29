import { describe, expect, test } from "bun:test";
import { filterIssues } from "./issue-filter";

const issues = [
  {
    key: "SP-142",
    title: "Add email sign-in",
    status: "In Review",
    assignee: "Shanto",
    labels: ["auth", "frontend"],
    milestone: "Launch",
  },
  {
    key: "SP-143",
    title: "Improve sign-in copy",
    status: "Todo",
    assignee: "Codex",
    labels: ["UX", "frontend"],
    milestone: "Later",
  },
];
const all = { query: "", status: "All", assignee: "All", label: "All", milestone: "All" };

describe("filterIssues", () => {
  test("finds issue keys and titles without regard to case or surrounding spaces", () => {
    expect(filterIssues(issues, { ...all, query: " sp-142 " })).toEqual([issues[0]!]);
    expect(filterIssues(issues, { ...all, query: "SIGN-IN" })).toEqual(issues);
  });
  test("matches any label on an issue", () => {
    expect(filterIssues(issues, { ...all, label: "frontend" })).toEqual(issues);
    expect(filterIssues(issues, { ...all, label: "auth" })).toEqual([issues[0]!]);
    expect(filterIssues(issues, { ...all, label: "missing" })).toEqual([]);
  });
  test("combines filters and returns no unrelated issues", () => {
    expect(
      filterIssues(issues, {
        ...all,
        status: "Todo",
        assignee: "Codex",
        label: "UX",
        milestone: "Later",
      }),
    ).toEqual([issues[1]!]);
    expect(filterIssues(issues, { ...all, status: "Todo", assignee: "Shanto" })).toEqual([]);
  });
});
