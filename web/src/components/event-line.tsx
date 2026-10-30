import { Link } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { findingStatusLabel } from "../lib/finding-status";
import { issueStatusLabel } from "../lib/issue-status";
import { formatTime } from "../lib/format";
import type { ActivityEvent, FindingStatus, IssueStatus } from "../lib/types";
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

/** Says what an event did in words, such as "added a finding to", with a detail line if any. */
function describe(event: ActivityEvent): { action: string; detail?: string } {
  const p = event.payload;
  switch (event.kind) {
    case "issue.created":
      return { action: "created" };
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
    default:
      return { action: event.kind.replace(/[._]/g, " ") };
  }
}

/**
 * One event in an activity timeline: when, who, and what, linking to its issue. An agent's event
 * shows the model it reported, which the server records but cannot verify.
 */
export function EventLine({ event }: { event: ActivityEvent }) {
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
            to="/$issue"
            params={{ issue: issue.key }}
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
