import { describe, expect, test } from "bun:test";
import { filterIssues } from "./issue-filter";

const issues = [
  {
    key: "SP-142",
    title: "Add email sign-in",
    status: "in_review",
    assignee_actor_id: "person-shanto",
    label_ids: ["label-auth", "label-frontend"],
    milestone_id: "milestone-launch",
  },
  {
    key: "SP-143",
    title: "Improve sign-in copy",
    status: "todo",
    assignee_actor_id: "agent-shanto-codex",
    label_ids: ["label-ux", "label-frontend"],
    milestone_id: "milestone-later",
  },
];

describe("filterIssues", () => {
  test("finds issue keys and titles without regard to case or surrounding spaces", () => {
    expect(filterIssues(issues, { q: " sp-142 " })).toEqual([issues[0]!]);
    expect(filterIssues(issues, { q: "SIGN-IN" })).toEqual(issues);
  });
  test("matches any label on an issue", () => {
    expect(filterIssues(issues, { label: "label-frontend" })).toEqual(issues);
    expect(filterIssues(issues, { label: "label-auth" })).toEqual([issues[0]!]);
    expect(filterIssues(issues, { label: "label-missing" })).toEqual([]);
  });
  test("combines filters and returns no unrelated issues", () => {
    expect(
      filterIssues(issues, {
        status: "todo",
        assignee: "agent-shanto-codex",
        label: "label-ux",
        milestone: "milestone-later",
      }),
    ).toEqual([issues[1]!]);
    expect(filterIssues(issues, { status: "todo", assignee: "person-shanto" })).toEqual([]);
  });
});
