import { webUrl } from "./format";
import type { ActivityEvent } from "./types";

/** How a GitHub event reads in a timeline: who did it, what they did, and what it links to. */
export type GithubEventLine = {
  /** The GitHub login that did it, or "" when the event names a teammate through `actorId`. */
  login: string;
  /** The teammate to show instead of a login, or "" when `login` is shown. */
  actorId: string;
  action: string;
  link?: { url: string; text: string };
  detail?: string;
};

const reviewActions: Record<string, string> = {
  approved: "approved",
  changes_requested: "requested changes on",
  commented: "reviewed",
};

const pullRequestActions: Record<string, string> = {
  opened: "opened",
  reopened: "reopened",
  closed: "closed",
  merged: "merged",
  ready_for_review: "marked ready for review",
  converted_to_draft: "converted to draft",
  edited: "edited",
  pushed: "pushed to",
};

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

/** Describes a github.* or discord.channel.set event, or returns null for any other kind. */
export function describeGithubEvent(event: ActivityEvent): GithubEventLine | null {
  const p = event.payload;
  const login = text(p.github_login);
  const repository = text(p.repository);
  const pull =
    typeof p.number === "number"
      ? {
          url: webUrl(text(p.url)),
          text: `#${p.number}${text(p.title) ? ` ${text(p.title)}` : ""}`,
        }
      : undefined;
  const link = pull?.url ? pull : undefined;
  const line = (action: string, extra: Partial<GithubEventLine> = {}): GithubEventLine => ({
    login: login || "GitHub",
    actorId: "",
    action,
    link,
    ...extra,
  });
  const plainPull = (action: string): GithubEventLine =>
    line(action, { link: undefined, detail: pull?.text });

  if (event.kind.startsWith("github.pull_request.")) {
    const name = event.kind.slice("github.pull_request.".length);
    if (name === "review_requested") {
      const reviewer = text(p.reviewer);
      const action = reviewer ? `requested a review from ${reviewer} on` : "requested a review on";
      return link ? line(action) : plainPull(action);
    }
    const action = pullRequestActions[name] ?? name.replace(/_/g, " ");
    return link ? line(action) : plainPull(action);
  }

  switch (event.kind) {
    case "github.review.submitted": {
      const action = reviewActions[text(p.state)] ?? "reviewed";
      return link ? line(action) : plainPull(action);
    }
    case "github.review.dismissed":
      return link ? line("dismissed a review on") : plainPull("dismissed a review on");
    case "github.comment.created":
      return line(link ? "commented on" : "commented on a pull request", {
        detail: text(p.body_excerpt) || undefined,
      });
    case "github.checks.changed": {
      const state = text(p.checks_state);
      const action = state ? `checks ${state} on` : "checks changed on";
      return link ? line(action, { login: login || "GitHub" }) : plainPull(action);
    }
    case "github.repository.added":
      return line(`added repository ${repository}`, { link: undefined });
    case "github.repository.removed":
      return line(`removed repository ${repository}`, { link: undefined });
    case "github.repository.linked": {
      const project = text(p.project_key);
      return line(
        project ? `linked ${repository} to ${project}` : `unlinked ${repository} from its project`,
        { link: undefined },
      );
    }
    case "github.repository.synced": {
      const count = typeof p.count === "number" ? p.count : 0;
      return line(
        `loaded ${count} pull ${count === 1 ? "request" : "requests"} from ${repository}`,
        { link: undefined },
      );
    }
    case "github.account.linked": {
      const linked = text(p.login);
      return {
        login: "",
        actorId: event.target_id,
        action: linked ? `linked GitHub account @${linked}` : "unlinked their GitHub account",
      };
    }
    case "discord.channel.set":
      return {
        login: "",
        actorId: event.actor_id,
        action: "set a Discord channel for GitHub updates",
        detail: text(p.channel_id) ? `Channel ${text(p.channel_id)}` : undefined,
      };
    default:
      return null;
  }
}
