import { Link } from "@tanstack/react-router";
import type { CSSProperties } from "react";
import { slugify } from "../lib/format";
import { issueStatuses, statusIcon } from "../lib/issue-status";
import type { Issue, IssuePriority, IssueStatus } from "../lib/types";
import { Avatar } from "./avatar";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";

const groupBackground: Record<IssueStatus, string> = {
  "In Review": "bg-[#2b2634]",
  "In Progress": "bg-[#1c2736]",
  Todo: "bg-[#27292c]",
  Backlog: "bg-[#292c30]",
  Done: "bg-[#23302a]",
  Canceled: "bg-[#27292c]",
};

const priorityStyles: Record<IssuePriority, string> = {
  Urgent: "text-[#ff8585] [&>span]:opacity-100",
  High: "text-[#f2ad67] [&>span]:opacity-22 [&>span:nth-child(-n+3)]:opacity-100",
  Medium: "text-[#d8c66d] [&>span]:opacity-22 [&>span:nth-child(-n+2)]:opacity-100",
  Low: "text-[#9ba9bc] [&>span]:opacity-22 [&>span:first-child]:opacity-100",
};

type IssueListProps = {
  issues: Issue[];
  toolbarHeight: number;
  collapsed: IssueStatus[];
  onToggleGroup: (status: IssueStatus) => void;
};

/** Lists issues grouped by status. Each group can be collapsed under its sticky heading. */
export function IssueList({ issues, toolbarHeight, collapsed, onToggleGroup }: IssueListProps) {
  return (
    <div
      className="scrollbar-thin [scrollbar-color:#465062_transparent] px-[26px] pt-0 pb-[50px] [view-transition-name:issue-view-content] max-[700px]:px-[16px] max-[700px]:pt-0 max-[700px]:pb-[16px] [&_>_section+section]:mt-[8px]"
      style={{ "--issues-toolbar-height": `${toolbarHeight}px` } as CSSProperties}
    >
      {issueStatuses.map((status) => {
        const rows = issues.filter((issue) => issue.status === status);
        if (!rows.length) return null;

        const isCollapsed = collapsed.includes(status);

        return (
          <section key={status}>
            <button
              className={`group/heading sticky top-(--issues-toolbar-height,0px) z-1 flex w-full items-center gap-[10px] rounded-[8px] border-0 px-[12px] py-[8px] text-left ${groupBackground[status]}`}
              aria-expanded={!isCollapsed}
              onClick={() => onToggleGroup(status)}
            >
              <Icon
                name="chevron-down"
                className="transition-transform duration-180 ease-[ease] group-aria-[expanded=false]/heading:-rotate-90 motion-reduce:transition-none"
              />
              <span
                className={`text-[17px] text-[#98a9c3] [&.status-backlog]:text-[#a9b6c8] [&.status-done]:text-[#44ce91] [&.status-in-progress]:text-[#4c7aff] [&.status-in-review]:text-[#a574ff] status-${slugify(status)}`}
              >
                <Icon name={statusIcon[status]} />
              </span>
              <strong>{status}</strong>
              <span className="text-[13px] font-medium text-[#9ba9bc]">{rows.length}</span>
            </button>
            <div
              className="grid grid-rows-[minmax(0,1fr)] opacity-100 [transition:grid-template-rows_220ms_ease,opacity_180ms_ease] aria-hidden:grid-rows-[minmax(0,0fr)] aria-hidden:opacity-0 motion-reduce:[transition:none]"
              aria-hidden={isCollapsed}
              inert={isCollapsed}
            >
              <div className="min-h-0 overflow-hidden">
                {rows.map((issue) => (
                  <Link
                    className="ml-[24px] grid min-h-[45px] w-[calc(100%-24px)] grid-cols-[20px_90px_minmax(210px,1fr)_230px_60px] items-center gap-[15px] rounded-[10px] border-0 bg-transparent px-[12px] py-[6px] text-left text-[13px] [transition:background-color_160ms_ease] hover:bg-[#18191a] motion-reduce:[transition:none] max-[1100px]:grid-cols-[20px_90px_minmax(180px,1fr)_170px_60px] max-[700px]:min-w-[660px] [&_.label-chip]:text-[13px]"
                    key={issue.key}
                    to="/$issue"
                    params={{ issue: issue.key }}
                  >
                    <span
                      className={`inline-flex h-[16px] w-[18px] items-end gap-[2px] [&>span]:flex-[0_0_3px] [&>span]:rounded-t-[2px] [&>span]:bg-current [&>span:nth-child(1)]:h-[4px] [&>span:nth-child(2)]:h-2 [&>span:nth-child(3)]:h-[11px] [&>span:nth-child(4)]:h-[15px] ${priorityStyles[issue.priority]}`}
                      role="img"
                      aria-label={`${issue.priority} priority`}
                      title={`${issue.priority} priority`}
                    >
                      <span aria-hidden="true" />
                      <span aria-hidden="true" />
                      <span aria-hidden="true" />
                      <span aria-hidden="true" />
                    </span>
                    <span className="flex items-center gap-4 text-[14px] text-[#aebed4]">
                      {issue.key}
                    </span>
                    <span className="text-[15px] text-[#eef0f5]">{issue.title}</span>
                    <LabelChips labels={issue.labels} />
                    <span
                      className="[&_.agent-icon]:h-[32px] [&_.agent-icon]:w-[32px] [&_.avatar]:h-[32px] [&_.avatar]:w-[32px] [&_.avatar]:text-[11px]"
                      role="img"
                      aria-label={`Assigned to ${issue.assignee}`}
                      title={issue.assignee}
                    >
                      <Avatar name={issue.assignee} />
                    </span>
                  </Link>
                ))}
              </div>
            </div>
          </section>
        );
      })}
      {!issues.length && (
        <div className="mx-auto my-40 max-w-120 p-[24px] text-center text-[#93a0b2] [&_h2]:mx-0 [&_h2]:mt-[18px] [&_h2]:mb-2 [&_h2]:text-[#e9edf5]">
          <h2>No issues found</h2>
          <p>Try changing your filters.</p>
        </div>
      )}
    </div>
  );
}
