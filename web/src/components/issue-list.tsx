import { useProjectSlugs } from "../hooks/use-project";
import { Link } from "@tanstack/react-router";
import type { CSSProperties } from "react";
import { slugify } from "../lib/format";
import { issueStatusLabel, issueStatuses, statusIcon } from "../lib/issue-status";
import type { Issue, IssueStatus } from "../lib/types";
import { useActor } from "../queries/project";
import { Avatar } from "./avatar";
import { useActorTitle } from "./person";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";

const groupBackground: Record<IssueStatus, string> = {
  in_review: "bg-[#2b2634]",
  in_progress: "bg-[#1c2736]",
  todo: "bg-[#27292c]",
  backlog: "bg-[#292c30]",
  done: "bg-[#23302a]",
  canceled: "bg-[#27292c]",
};

type IssueListProps = {
  issues: Issue[];
  toolbarHeight: number;
  collapsed: IssueStatus[];
  onToggleGroup: (status: IssueStatus) => void;
  onCreateIssue?: (status: IssueStatus) => void;
  createDisabled?: boolean;
};

/** Lists issues grouped by status. Each group can be collapsed under its sticky heading. */
export function IssueList({
  issues,
  toolbarHeight,
  collapsed,
  onToggleGroup,
  onCreateIssue,
  createDisabled,
}: IssueListProps) {
  const projectSlug = useProjectSlugs();
  const actor = useActor();
  const actorTitle = useActorTitle();

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
            <div
              className={`group/heading sticky top-(--issues-toolbar-height,0px) z-1 flex w-full items-center rounded-[8px] ${groupBackground[status]}`}
            >
              <button
                type="button"
                className="group/toggle flex min-w-0 flex-1 items-center gap-[10px] rounded-[8px] border-0 bg-transparent px-[12px] py-[8px] text-left"
                aria-expanded={!isCollapsed}
                onClick={() => onToggleGroup(status)}
              >
                <Icon
                  name="chevron-down"
                  className="transition-transform duration-180 ease-[ease] group-aria-[expanded=false]/toggle:-rotate-90 motion-reduce:transition-none"
                />
                <span
                  className={`text-[17px] text-[#98a9c3] [&.status-backlog]:text-[#a9b6c8] [&.status-done]:text-[#44ce91] [&.status-in-progress]:text-[#4c7aff] [&.status-in-review]:text-[#a574ff] status-${slugify(status)}`}
                >
                  <Icon name={statusIcon[status]} />
                </span>
                <strong>{issueStatusLabel[status]}</strong>
                <span className="text-[13px] font-medium text-[#9ba9bc]">{rows.length}</span>
              </button>
              {onCreateIssue && (
                <button
                  type="button"
                  className="pointer-events-none mr-[8px] grid size-7 shrink-0 place-items-center rounded-[6px] border-0 bg-transparent text-[#9ba9bc] opacity-0 transition-opacity group-hover/heading:pointer-events-auto group-hover/heading:opacity-100 hover:bg-white/10 hover:text-white focus-visible:pointer-events-auto focus-visible:opacity-100 disabled:cursor-not-allowed disabled:text-[#657084] motion-reduce:transition-none"
                  aria-label={`Create issue in ${issueStatusLabel[status]}`}
                  title={`Create issue in ${issueStatusLabel[status]}`}
                  disabled={createDisabled}
                  onClick={() => onCreateIssue(status)}
                >
                  <Icon name="plus" />
                </button>
              )}
            </div>
            <div
              className="grid grid-rows-[minmax(0,1fr)] opacity-100 [transition:grid-template-rows_220ms_ease,opacity_180ms_ease] aria-hidden:grid-rows-[minmax(0,0fr)] aria-hidden:opacity-0 motion-reduce:[transition:none]"
              aria-hidden={isCollapsed}
              inert={isCollapsed}
            >
              <div className="min-h-0 overflow-hidden">
                {rows.map((issue) => {
                  const creatorTitle = actorTitle(actor(issue.created_by_actor_id));
                  const sameActor = issue.assignee_actor_id === issue.created_by_actor_id;
                  const creatorLabel = sameActor
                    ? `Created by and assigned to ${creatorTitle}`
                    : `Created by ${creatorTitle}`;
                  return (
                    <Link
                      className="ml-[24px] grid min-h-[45px] w-[calc(100%-24px)] grid-cols-[20px_90px_minmax(210px,1fr)_230px_60px] items-center gap-[15px] rounded-[10px] border-0 bg-transparent px-[12px] py-[6px] text-left text-[13px] [transition:background-color_160ms_ease] hover:bg-[#18191a] motion-reduce:[transition:none] max-[1100px]:grid-cols-[20px_90px_minmax(180px,1fr)_170px_60px] max-[700px]:min-w-[660px] [&_.label-chip]:text-[13px]"
                      key={issue.key}
                      to="/$projectSlug/issues/$issue"
                      params={{ projectSlug: projectSlug(issue.project_id), issue: issue.key }}
                    >
                      <Icon name={statusIcon[issue.status]} />
                      <span className="flex items-center gap-4 text-[14px] text-[#aebed4]">
                        {issue.key}
                      </span>
                      <span className="text-[15px] text-[#eef0f5]">{issue.title}</span>
                      <LabelChips labelIds={issue.label_ids} projectKey={issue.project_key} />
                      <span className="flex items-center justify-end gap-[4px] [&_.agent-icon]:h-[26px] [&_.agent-icon]:w-[26px] [&_.avatar]:h-[26px] [&_.avatar]:w-[26px] [&_.avatar]:basis-[26px] [&_.avatar]:text-[9px] [&_.icon]:size-[26px] [&_.icon]:text-[#7d8899]">
                        <span
                          className="flex"
                          role="img"
                          aria-label={creatorLabel}
                          title={creatorLabel}
                        >
                          <Avatar actor={actor(issue.created_by_actor_id)} />
                        </span>
                        {issue.assignee_actor_id && !sameActor && (
                          <span
                            className="flex"
                            role="img"
                            aria-label={`Assigned to ${actorTitle(actor(issue.assignee_actor_id))}`}
                            title={`Assigned to ${actorTitle(actor(issue.assignee_actor_id))}`}
                          >
                            <Avatar actor={actor(issue.assignee_actor_id)} />
                          </span>
                        )}
                      </span>
                    </Link>
                  );
                })}
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
