import { Link } from "@tanstack/react-router";
import { issueStatuses, statusIcon } from "../lib/issue-status";
import { slugify } from "../lib/format";
import type { Issue } from "../lib/types";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { Person } from "./person";

type IssueBoardProps = {
  issues: Issue[];
  statusFilter: string;
};

/** Lays issues out as one column per status; a status filter other than "All" keeps one column. */
export function IssueBoard({ issues, statusFilter }: IssueBoardProps) {
  const columns = issueStatuses.filter(
    (status) => statusFilter === "All" || status === statusFilter,
  );

  return (
    <div className="issue-board app-scrollbar" aria-label="Issues by status">
      {columns.map((status) => {
        const rows = issues.filter((issue) => issue.status === status);

        return (
          <section className="board-column" key={status} aria-label={status}>
            <div className="board-heading">
              <span className={`status-icon status-${slugify(status)}`}>
                <Icon name={statusIcon[status]} />
              </span>
              <strong>{status}</strong>
              <span className="count">{rows.length}</span>
            </div>
            <div
              className="board-cards app-scrollbar"
              role="region"
              aria-label={`${status} issues`}
              tabIndex={0}
            >
              {rows.map((issue) => (
                <Link
                  className="board-card"
                  key={issue.key}
                  to="/$issue"
                  params={{ issue: issue.key }}
                >
                  <span className="board-card-key">{issue.key}</span>
                  <strong>{issue.title}</strong>
                  <LabelChips labels={issue.labels} />
                  <span className="board-card-footer">
                    <Person name={issue.assignee} />
                    <span className="muted">
                      <Icon name="message-square" /> &nbsp;{issue.findings}
                    </span>
                  </span>
                </Link>
              ))}
              {rows.length === 0 && <p className="board-empty">No issues</p>}
            </div>
          </section>
        );
      })}
    </div>
  );
}
