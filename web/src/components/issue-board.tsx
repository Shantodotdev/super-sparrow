import { Link } from "@tanstack/react-router";
import { issueStatusLabel, issueStatuses, statusIcon } from "../lib/issue-status";
import { slugify } from "../lib/format";
import type { Issue, IssueStatus } from "../lib/types";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { Person } from "./person";

type IssueBoardProps = {
  issues: Issue[];
  statusFilter: IssueStatus | undefined;
};

/** Lays issues out as one column per status; a status filter keeps only its column. */
export function IssueBoard({ issues, statusFilter }: IssueBoardProps) {
  const columns = issueStatuses.filter((status) => !statusFilter || status === statusFilter);

  return (
    <div
      className="issue-board flex min-h-0 flex-1 scrollbar-thin [scrollbar-color:#465062_transparent] items-stretch gap-4 overflow-x-auto overflow-y-hidden px-[26px] pt-[4px] pb-[32px] [view-transition-name:issue-view-content] max-[700px]:px-[16px] max-[700px]:pb-[24px]"
      aria-label="Issues by status"
    >
      {columns.map((status) => {
        const rows = issues.filter((issue) => issue.status === status);

        return (
          <section
            className="flex min-h-0 flex-[0_0_270px] flex-col rounded-[10px] border border-[#222831] bg-[#0d0f12] p-[12px]"
            key={status}
            aria-label={issueStatusLabel[status]}
          >
            <div className="flex items-center gap-[8px] px-[2px] pt-[2px] pb-4">
              <span
                className={`text-[17px] text-[#98a9c3] [&.status-backlog]:text-[#a9b6c8] [&.status-done]:text-[#44ce91] [&.status-in-progress]:text-[#4c7aff] [&.status-in-review]:text-[#a574ff] status-${slugify(status)}`}
              >
                <Icon name={statusIcon[status]} />
              </span>
              <strong className="text-[13px]">{issueStatusLabel[status]}</strong>
              <span className="count rounded-[5px] bg-[#262b34] px-2 py-[3px] text-[12px] text-[#dce2ec]">
                {rows.length}
              </span>
            </div>
            <div
              className="grid min-h-0 scrollbar-thin [scrollbar-color:#465062_transparent] auto-rows-max content-start gap-[9px] overflow-y-auto overscroll-contain"
              role="region"
              aria-label={`${issueStatusLabel[status]} issues`}
              tabIndex={0}
            >
              {rows.map((issue) => (
                <Link
                  className="flex w-full flex-col items-start gap-[10px] rounded-[8px] border border-[#252b35] bg-[#12151a] p-[13px] text-left hover:border-[#5873c7] hover:bg-[#171d28]"
                  key={issue.key}
                  to="/$issue"
                  params={{ issue: issue.key }}
                >
                  <span className="text-[12px] text-[#aabbd1]">{issue.key}</span>
                  <strong className="text-[13px] leading-[1.4] text-[#edf0f6]">
                    {issue.title}
                  </strong>
                  <LabelChips labelIds={issue.label_ids} />
                  <span className="flex w-full items-center justify-between gap-[12px] text-[12px]">
                    <Person actorId={issue.assignee_actor_id} />
                    <span className="muted flex gap-[10px] text-[#a2aec2]">
                      {issue.unresolved_findings > 0 && (
                        <span title={`${issue.unresolved_findings} unresolved findings`}>
                          <Icon name="triangle-alert" /> &nbsp;{issue.unresolved_findings}
                        </span>
                      )}
                      <span title={`${issue.comment_count} comments`}>
                        <Icon name="message-square" /> &nbsp;{issue.comment_count}
                      </span>
                    </span>
                  </span>
                </Link>
              ))}
              {rows.length === 0 && (
                <p className="px-[4px] py-[12px] text-[12px] text-[#7e8b9d]">No issues</p>
              )}
            </div>
          </section>
        );
      })}
    </div>
  );
}
