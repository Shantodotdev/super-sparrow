import { useProjectSlugs } from "../hooks/use-project";
import { Link } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { findingStatusLabel } from "../lib/finding-status";
import { issueStatusLabel } from "../lib/issue-status";
import { formatTime } from "../lib/format";
import { secretConfigLabel } from "../lib/secrets";
import type { ActivityEvent, FindingStatus, IssueStatus, SecretConfigId } from "../lib/types";
import { issuesQuery } from "../queries/issues";
import { Person } from "./person";

/** Returns the issue an event is about: its target, or the issue its payload names. */
export function eventIssueId(event: ActivityEvent): string {
  if (event.target_type === "issue") return event.target_id;
  return typeof event.payload.issue_id === "string" ? event.payload.issue_id : "";
}

function finding(payload: Record<string, unknown>): string {
  return `round ${String(payload.round)} R${String(payload.number)}`;
}

function names(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function configName(value: unknown): string {
  return secretConfigLabel[value as SecretConfigId] ?? String(value);
}

function joinNames(value: unknown): string {
  return names(value).join(", ");
}

const changeLabel = { added: "Added", changed: "Changed", removed: "Removed" } as const;

/** Lists the key names a secrets change touched, never values. */
function secretChanges(p: Record<string, unknown>): string | undefined {
  const parts = (["added", "changed", "removed"] as const)
    .filter((kind) => names(p[kind]).length)
    .map((kind) => `${changeLabel[kind]}: ${joinNames(p[kind])}`);

  return parts.length ? parts.join(" · ") : undefined;
}

/** Says what an event did in words, such as "added a finding to", with a detail line if any. */
function describe(event: ActivityEvent): { action: string; detail?: string } {
  const p = event.payload;
  switch (event.kind) {
    case "issue.created":
      return { action: "created" };
    case "issue.subissue_created":
      return {
        action: `added subissue ${String(p.number)} to`,
        detail: typeof p.title === "string" ? p.title : undefined,
      };
    case "issue.subissue_updated":
      return { action: `updated subissue ${String(p.number)} on` };
    case "issue.status_changed":
      return { action: `moved to ${issueStatusLabel[p.to as IssueStatus] ?? String(p.to)}` };
    case "issue.checklist_changed":
      return { action: "updated the checklist of" };
    case "review_round.started":
      return { action: `started review round ${String(p.number)} on` };
    case "review_round.finished":
      return { action: `finished review round ${String(p.number)} on` };
    case "finding.added":
      return {
        action: `added ${finding(p)} to`,
        detail: typeof p.title === "string" ? p.title : undefined,
      };
    case "finding.status_changed": {
      const status = findingStatusLabel[p.to as FindingStatus] ?? String(p.to);
      const reason = typeof p.reason === "string" && p.reason ? p.reason : undefined;
      return { action: `marked ${finding(p)} ${status.toLowerCase()} on`, detail: reason };
    }
    case "finding.duplicate_marked":
      return { action: `marked ${finding(p)} a duplicate on` };
    case "comment.added":
      return { action: "commented on" };
    case "secrets.enabled":
      return { action: "turned on secrets for this project" };
    case "secret.changed":
      return { action: `changed secrets in ${configName(p.config)}`, detail: secretChanges(p) };
    case "secret.restored":
      return {
        action: `restored ${String(p.name)} to version ${String(p.version)} in ${configName(p.config)}`,
      };
    case "dev_secret.changed":
      return { action: "changed their dev secrets", detail: secretChanges(p) };
    case "dev_secret.restored":
      return { action: `restored ${String(p.name)} to version ${String(p.version)} in their dev` };
    case "secret.required_keys_changed":
      return {
        action: `changed the required keys of ${configName(p.config)}`,
        detail: joinNames(p.names) || "No required keys",
      };
    case "service_token.created":
    case "service_token.renewed":
    case "service_token.revoked":
      return {
        action: `${event.kind.slice("service_token.".length)} a ${configName(p.config).toLowerCase()} service token`,
        detail: typeof p.label === "string" ? p.label : undefined,
      };
    case "actor.environments_permission_changed":
      return {
        action: p.can_manage_environments
          ? "allowed a member to manage staging and production"
          : "stopped a member from managing staging and production",
      };
    default:
      return { action: event.kind.replace(/[._]/g, " ") };
  }
}

/**
 * One event in an activity timeline: when, who, and what, linking to its issue. An agent's event
 * shows the model it reported, which the server records but cannot verify.
 */
export function EventLine({ event }: { event: ActivityEvent }) {
  const projectSlug = useProjectSlugs();
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const issue = issues.find((item) => item.id === eventIssueId(event));
  const { action, detail } = describe(event);

  return (
    <div className="grid grid-cols-[100px_14px_180px_1fr] items-start gap-[18px] px-0 py-[15px] max-[700px]:grid-cols-[65px_10px_1fr] max-[700px]:gap-[8px] max-[700px]:[&_>_div:last-child]:col-3 [&_p]:mt-[6px] [&_time]:text-[#95a6bd]">
      <time dateTime={event.created_at}>{formatTime(event.created_at)}</time>
      <div className="mt-[5px] h-[10px] w-[10px] rounded-full bg-[#77889f]" />
      <Person actorId={event.actor_id} />
      <div>
        <span>{action} </span>
        {issue && (
          <Link
            to="/$projectSlug/issues/$issue"
            params={{ projectSlug: projectSlug(issue.project_id), issue: issue.key }}
            className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
          >
            {issue.key}
          </Link>
        )}
        {detail && <p className="muted text-[#a2aec2]">{detail}</p>}
        {event.reported_model && (
          <p className="muted text-[12px] text-[#7e8b9d]">Reported model: {event.reported_model}</p>
        )}
      </div>
    </div>
  );
}
