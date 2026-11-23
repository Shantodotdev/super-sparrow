import { expect, test } from "bun:test";
import { describeGithubEvent } from "./github-events";
import type { ActivityEvent } from "./types";

function event(kind: string, payload: Record<string, unknown>): ActivityEvent {
  return {
    id: "e1",
    sequence: 1,
    project_id: "p1",
    target_type: "project",
    target_id: "p1",
    kind,
    actor_id: "system",
    reported_model: "",
    payload,
    created_at: "2026-11-23T00:00:00Z",
  };
}

const pull = {
  repository: "acme/web",
  number: 12,
  title: "Add login",
  url: "https://github.com/acme/web/pull/12",
  github_login: "octocat",
};

test("a pull request event reads as the login, an action, and a link", () => {
  const line = describeGithubEvent(event("github.pull_request.opened", pull));
  expect(line?.login).toBe("octocat");
  expect(line?.action).toBe("opened");
  expect(line?.link).toEqual({ url: pull.url, text: "#12 Add login" });
});

test("a pull request link that is not http is not linked", () => {
  const line = describeGithubEvent(
    event("github.pull_request.merged", { ...pull, url: "javascript:alert(1)" }),
  );
  expect(line?.link).toBeUndefined();
  expect(line?.detail).toBe("#12 Add login");
});

test("review, comment, and sync events say what happened", () => {
  expect(
    describeGithubEvent(event("github.review.submitted", { ...pull, state: "approved" }))?.action,
  ).toBe("approved");
  expect(
    describeGithubEvent(event("github.comment.created", { ...pull, body_excerpt: "Looks good" }))
      ?.detail,
  ).toBe("Looks good");
  expect(
    describeGithubEvent(
      event("github.repository.synced", { repository: "acme/web", github_login: "", count: 1 }),
    )?.action,
  ).toBe("loaded 1 pull request from acme/web");
});

test("an account link names the teammate and a Discord channel names its actor", () => {
  const linked = describeGithubEvent(event("github.account.linked", { login: "octocat" }));
  expect(linked?.actorId).toBe("p1");
  expect(linked?.action).toBe("linked GitHub account @octocat");
  expect(describeGithubEvent(event("discord.channel.set", { channel_id: "9" }))?.actorId).toBe(
    "system",
  );
});

test("other kinds are left to the fallback", () => {
  expect(describeGithubEvent(event("issue.created", {}))).toBeNull();
});
