import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Columns3 } from "lucide-react";
import { Filter } from "../components/filter";
import { Icon } from "../components/icon";
import { IssueBoard } from "../components/issue-board";
import { IssueList } from "../components/issue-list";
import { NewIssueModal } from "../components/new-issue-modal";
import { useElementHeight } from "../hooks/use-element-height";
import { filterIssues } from "../lib/issue-filter";
import { issueStatuses } from "../lib/issue-status";
import type { IssueStatus } from "../lib/types";
import { validateIssuesSearch } from "../lib/search";
import { issueLabelOptions, issuesQuery, useCreateIssue } from "../queries/issues";

export const Route = createFileRoute("/issues")({
  staticData: { section: "issues" },
  validateSearch: validateIssuesSearch,
  loader: async ({ context }) => {
    await context.queryClient.query({ ...issuesQuery, staleTime: "static" });
  },
  component: IssuesPage,
});

/** The project issue list, shown as a grouped list or a board, with search and filters. */
function IssuesPage() {
  const { view } = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const createIssue = useCreateIssue();

  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("All");
  const [assignee, setAssignee] = useState("All");
  const [label, setLabel] = useState("All");
  const [milestone, setMilestone] = useState("All");
  const [collapsed, setCollapsed] = useState<IssueStatus[]>([]);
  const [creating, setCreating] = useState(false);
  const { ref: toolbarRef, height: toolbarHeight } = useElementHeight<HTMLDivElement>();

  const assignees = [...new Set(issues.map((issue) => issue.assignee))];
  const milestones = [...new Set(issues.map((issue) => issue.milestone))];
  const labels = issueLabelOptions(issues);
  const filtered = filterIssues(issues, { query, status, assignee, label, milestone });

  const toggleGroup = (group: IssueStatus) =>
    setCollapsed((items) =>
      items.includes(group) ? items.filter((item) => item !== group) : [...items, group],
    );

  return (
    <>
      <div className="issues-toolbar" ref={toolbarRef}>
        <div className="issue-controls">
          <div className="issue-topline">
            <div className="breadcrumb">Projects / Sparrow</div>
          </div>
          <div className="filters issue-filters">
            <input
              className="filter-search"
              placeholder="Search issues..."
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-label="Search issues"
            />
            <Filter name="Status" value={status} onChange={setStatus} options={issueStatuses} />
            <Filter name="Assignee" value={assignee} onChange={setAssignee} options={assignees} />
            <Filter name="Label" value={label} onChange={setLabel} options={labels} />
            <Filter
              name="Milestone"
              value={milestone}
              onChange={setMilestone}
              options={milestones}
            />
            <div className="view-switch" role="group" aria-label="Issue view">
              <button
                type="button"
                aria-pressed={view !== "board"}
                onClick={() => void navigate({ search: { view: undefined } })}
              >
                <Icon name="list" /> List
              </button>
              <button
                type="button"
                aria-pressed={view === "board"}
                onClick={() => void navigate({ search: { view: "board" } })}
              >
                <Columns3 aria-hidden="true" className="icon" /> Board
              </button>
            </div>
            <button className="primary new-issue-button" onClick={() => setCreating(true)}>
              <Icon name="plus" /> &nbsp; New issue
            </button>
          </div>
        </div>
      </div>
      {view === "board" ? (
        <IssueBoard issues={filtered} statusFilter={status} />
      ) : (
        <IssueList
          issues={filtered}
          toolbarHeight={toolbarHeight}
          collapsed={collapsed}
          onToggleGroup={toggleGroup}
        />
      )}
      <NewIssueModal
        open={creating}
        statuses={issueStatuses}
        assignees={assignees}
        milestones={milestones}
        labels={labels}
        onCreate={createIssue}
        onClose={() => setCreating(false)}
      />
    </>
  );
}
