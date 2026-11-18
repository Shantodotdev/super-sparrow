import { useDragAutoscroll } from "../hooks/use-drag-autoscroll";
import { useIssueDrag } from "../hooks/use-issue-drag";
import { useProjectSlugs } from "../hooks/use-project";
import { Link } from "@tanstack/react-router";
import { useEffect, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import { slugify } from "../lib/format";
import {
  issuePriorityColor,
  issuePriorityIcon,
  issuePriorityLabel,
  issueStatusLabel,
  issueStatuses,
  statusGroupBackground as groupBackground,
  statusIcon,
} from "../lib/issue-status";
import type { Issue, IssueStatus } from "../lib/types";
import { useMoveIssueStatus } from "../queries/issues";
import { useActor } from "../queries/project";
import { Avatar } from "./avatar";
import { useActorTitle } from "./person";
import { Icon } from "./icon";
import { IssueDragPreview } from "./issue-drag-preview";
import { useIssueContextMenu } from "./issue-context-menu";
import { IssueSelectionBar } from "./issue-selection-bar";
import { LabelChips } from "./label-chips";

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
  const moveIssue = useMoveIssueStatus();
  const contextMenu = useIssueContextMenu();
  const listRef = useRef<HTMLDivElement>(null);
  const { dragged, dropTarget, origin, dragProps } = useIssueDrag(moveIssue);

  const [selected, setSelected] = useState<string[]>([]);
  const lastToggled = useRef<string | undefined>(undefined);

  useDragAutoscroll(!!dragged, listRef, toolbarHeight);

  // Issues that disappear (filtered out, deleted) leave the selection.
  useEffect(() => {
    setSelected((ids) => {
      const kept = ids.filter((id) => issues.some((issue) => issue.id === id));
      return kept.length === ids.length ? ids : kept;
    });
  }, [issues]);

  // Shift-clicking a box selects every visible issue between it and the last one toggled.
  const toggleSelected = (issue: Issue, event: MouseEvent) => {
    const visible = issueStatuses
      .filter((status) => !collapsed.includes(status))
      .flatMap((status) => issues.filter((item) => item.status === status));
    const from = visible.findIndex((item) => item.id === lastToggled.current);
    const to = visible.findIndex((item) => item.id === issue.id);
    lastToggled.current = issue.id;

    if (event.shiftKey && from >= 0 && to >= 0) {
      const range = visible
        .slice(Math.min(from, to), Math.max(from, to) + 1)
        .map((item) => item.id);
      setSelected((ids) => [...new Set([...ids, ...range])]);
      return;
    }
    setSelected((ids) =>
      ids.includes(issue.id) ? ids.filter((id) => id !== issue.id) : [...ids, issue.id],
    );
  };

  const moveSelected = (status: IssueStatus) => {
    issues
      .filter((issue) => selected.includes(issue.id))
      .forEach((issue) => moveIssue(issue, status));
    setSelected([]);
  };

  return (
    <div
      ref={listRef}
      className="scrollbar-thin [scrollbar-color:#465062_transparent] px-[26px] pt-0 pb-[50px] [view-transition-name:issue-view-content] max-[700px]:px-[16px] max-[700px]:pt-0 max-[700px]:pb-[16px] [&_>_section+section]:mt-[8px]"
      style={{ "--issues-toolbar-height": `${toolbarHeight}px` } as CSSProperties}
    >
      {issueStatuses.map((status) => {
        const rows = issues.filter((issue) => issue.status === status);
        if (!rows.length && !dragged) return null;

        const isCollapsed = collapsed.includes(status);

        return (
          <section key={status} className="relative" data-drop-status={status}>
            <span
              aria-hidden="true"
              className={`pointer-events-none absolute inset-0 z-2 rounded-[8px] border-2 transition-[border-color,background-color] duration-150 motion-reduce:transition-none ${
                dropTarget === status && dragged?.status !== status
                  ? "border-[#5873c7] bg-[#5873c7]/5"
                  : "border-transparent"
              }`}
            />
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
                  const isSelected = selected.includes(issue.id);
                  return (
                    <div
                      key={issue.key}
                      className={`group/row relative rounded-[10px] [transition:background-color_160ms_ease,opacity_160ms_ease] motion-reduce:[transition:none] ${
                        dragged
                          ? dragged.id === issue.id
                            ? "bg-[#1e222a]"
                            : dropTarget === status
                              ? "opacity-35"
                              : ""
                          : isSelected
                            ? "bg-[#1a2237]"
                            : contextMenu.activeId === issue.id
                              ? "bg-[#18191a]"
                              : contextMenu.activeId
                                ? ""
                                : "hover:bg-[#18191a] has-[a:focus-visible]:bg-[#18191a]"
                      }`}
                    >
                      <button
                        type="button"
                        role="checkbox"
                        aria-checked={isSelected}
                        aria-label={`Select ${issue.key}`}
                        className={`absolute top-1/2 left-[11px] z-1 grid size-[18px] -translate-y-1/2 place-items-center rounded-[3px] border p-0 text-white transition-[opacity,background-color,border-color] duration-150 focus-visible:opacity-100 motion-reduce:transition-none ${
                          isSelected
                            ? "border-[#4c7aff] bg-[#4c7aff] opacity-100"
                            : `border-[#4b5568] bg-[#1b1f27] hover:border-[#7f9bff] hover:bg-[#232a38] ${selected.length ? "opacity-100" : "opacity-0 group-hover/row:opacity-100"}`
                        }`}
                        onClick={(event) => toggleSelected(issue, event)}
                      >
                        {isSelected && <Icon name="check" className="size-[10px] stroke-3" />}
                      </button>
                      <Link
                        className={`ml-[36px] grid min-h-[45px] w-[calc(100%-36px)] grid-cols-[20px_20px_90px_minmax(210px,1fr)_230px_60px] items-center gap-[15px] rounded-[10px] border-0 bg-transparent px-[12px] py-[6px] text-left text-[13px] focus:outline-none max-[1100px]:grid-cols-[20px_20px_90px_minmax(180px,1fr)_170px_60px] max-[700px]:min-w-[660px] [&_.label-chip]:text-[13px]`}
                        {...dragProps(issue)}
                        onContextMenu={contextMenu.open(issue)}
                        data-shortcut-row
                        to="/$projectSlug/issues/$issue"
                        params={{ projectSlug: projectSlug(issue.project_id), issue: issue.key }}
                      >
                        <Icon name={statusIcon[issue.status]} />
                        {issue.priority ? (
                          <span
                            className="flex"
                            role="img"
                            aria-label={`${issuePriorityLabel[issue.priority]} priority`}
                            title={`${issuePriorityLabel[issue.priority]} priority`}
                          >
                            <Icon
                              name={issuePriorityIcon[issue.priority]}
                              className={issuePriorityColor[issue.priority]}
                            />
                          </span>
                        ) : (
                          <span aria-hidden="true" />
                        )}
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
                    </div>
                  );
                })}
              </div>
            </div>
          </section>
        );
      })}
      {contextMenu.menu}
      {selected.length > 0 && (
        <IssueSelectionBar
          count={selected.length}
          onMove={moveSelected}
          onClear={() => setSelected([])}
        />
      )}
      {dragged && <IssueDragPreview issue={dragged} origin={origin} />}
      {!issues.length && (
        <div className="mx-auto my-40 max-w-120 p-[24px] text-center text-[#93a0b2] [&_h2]:mx-0 [&_h2]:mt-[18px] [&_h2]:mb-2 [&_h2]:text-[#e9edf5]">
          <h2>No issues found</h2>
          <p>Try changing your filters.</p>
        </div>
      )}
    </div>
  );
}
