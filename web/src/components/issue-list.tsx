import { Link } from "@tanstack/react-router";
import type { CSSProperties } from "react";
import { slugify } from "../lib/format";
import { issueStatuses, statusIcon } from "../lib/issue-status";
import type { Issue, IssueStatus } from "../lib/types";
import { Avatar } from "./avatar";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";

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
      className="list-content app-scrollbar"
      style={{ "--issues-toolbar-height": `${toolbarHeight}px` } as CSSProperties}
    >
      {issueStatuses.map((status) => {
        const rows = issues.filter((issue) => issue.status === status);
        if (!rows.length) return null;

        const isCollapsed = collapsed.includes(status);

        return (
          <section key={status}>
            <button
              className={`group-heading group-heading--${slugify(status)}`}
              aria-expanded={!isCollapsed}
              onClick={() => onToggleGroup(status)}
            >
              <Icon name={isCollapsed ? "chevron-right" : "chevron-down"} />
              <span className={`status-icon status-${slugify(status)}`}>
                <Icon name={statusIcon[status]} />
              </span>
              <strong>{status}</strong>
              <span className="count">{rows.length}</span>
            </button>
            {!isCollapsed &&
              rows.map((issue) => (
                <Link
                  className="issue-grid issue-row"
                  key={issue.key}
                  to="/$issue"
                  params={{ issue: issue.key }}
                >
                  <span className="issue-key">{issue.key}</span>
                  <span className="issue-title">{issue.title}</span>
                  <LabelChips labels={issue.labels} />
                  <span
                    className="issue-assignee"
                    role="img"
                    aria-label={`Assigned to ${issue.assignee}`}
                    title={issue.assignee}
                  >
                    <Avatar name={issue.assignee} />
                  </span>
                </Link>
              ))}
          </section>
        );
      })}
      {!issues.length && (
        <div className="empty-state">
          <h2>No issues found</h2>
          <p>Try changing your filters.</p>
        </div>
      )}
    </div>
  );
}
