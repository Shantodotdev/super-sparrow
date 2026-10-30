import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Columns3 } from "lucide-react";
import { Filter, all } from "../components/filter";
import { Icon } from "../components/icon";
import { IssueBoard } from "../components/issue-board";
import { IssueList } from "../components/issue-list";
import { NewIssueModal } from "../components/new-issue-modal";
import { useElementHeight } from "../hooks/use-element-height";
import { filterIssues } from "../lib/issue-filter";
import { issueStatusLabel, issueStatuses } from "../lib/issue-status";
import type { IssueStatus } from "../lib/types";
import { validateIssuesSearch, type IssuesSearch } from "../lib/search";
import { issuesQuery, useCreateIssue } from "../queries/issues";
import { actorsQuery, labelsQuery, milestonesQuery } from "../queries/project";
import { loadQuery } from "../queries/seeded";

export const Route = createFileRoute("/issues")({
  staticData: { section: "issues" },
  validateSearch: validateIssuesSearch,
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, issuesQuery),
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, labelsQuery),
      loadQuery(context.queryClient, milestonesQuery),
    ]);
  },
  component: IssuesPage,
});

/**
 * The project issue list, shown as a grouped list or a board, with search and filters. The view
 * and filters live in the URL, so a filtered list can be reloaded and shared.
 */
function IssuesPage() {
  const search = Route.useSearch();
  const { view } = search;
  const navigate = Route.useNavigate();
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const { data: milestones } = useSuspenseQuery(milestonesQuery);
  const createIssue = useCreateIssue();

  const [collapsed, setCollapsed] = useState<IssueStatus[]>([]);
  const [creating, setCreating] = useState(false);
  const { ref: toolbarRef, height: toolbarHeight } = useElementHeight<HTMLDivElement>();

  const assignable = actors.filter((actor) => actor.kind !== "system" && !actor.disabled_at);
  const statusOptions = issueStatuses.map((status) => ({
    value: status,
    label: issueStatusLabel[status],
  }));
  const assigneeOptions = assignable.map((actor) => ({
    value: actor.id,
    label: actor.display_name,
  }));
  const labelOptions = labels.map((label) => ({ value: label.id, label: label.name }));
  const milestoneOptions = milestones.map((milestone) => ({
    value: milestone.id,
    label: milestone.name,
  }));
  const filtered = filterIssues(issues, search);

  /** Sets one filter in the URL; the "All" choice removes it. Filtering replaces the history entry. */
  const setFilter = (name: Exclude<keyof IssuesSearch, "view">, value: string) =>
    void navigate({
      search: (previous) => ({
        ...previous,
        [name]: value === all || value === "" ? undefined : value,
      }),
      replace: true,
    });

  const toggleGroup = (group: IssueStatus) =>
    setCollapsed((items) =>
      items.includes(group) ? items.filter((item) => item !== group) : [...items, group],
    );

  const switchView = (nextView: "list" | "board") => {
    if ((view === "board") === (nextView === "board")) return;

    const updateView = async () => {
      await navigate({
        search: (previous) => ({ ...previous, view: nextView === "board" ? "board" : undefined }),
      });
    };
    const pageDocument = document as Document & {
      startViewTransition?: (updateCallback: () => Promise<void>) => {
        finished: Promise<void>;
      };
    };

    if (!pageDocument.startViewTransition) {
      void updateView();
      return;
    }

    document.documentElement.dataset.issueViewDirection = nextView;
    const transition = pageDocument.startViewTransition(updateView);
    void transition.finished.then(
      () => {
        if (document.documentElement.dataset.issueViewDirection === nextView)
          delete document.documentElement.dataset.issueViewDirection;
      },
      () => {
        if (document.documentElement.dataset.issueViewDirection === nextView)
          delete document.documentElement.dataset.issueViewDirection;
      },
    );
  };

  return (
    <>
      <div className="sticky top-0 z-2 bg-[#08090b] max-[700px]:overflow-visible" ref={toolbarRef}>
        <div className="px-[26px] pt-[24px] pb-0 max-[700px]:px-[16px] max-[700px]:pt-[16px] max-[700px]:pb-0">
          <div className="mb-[16px] flex items-center justify-between gap-[16px] [&_.breadcrumb]:mb-0">
            <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
              Projects / Sparrow
            </div>
          </div>
          <div className="mb-0 flex flex-nowrap items-center gap-[12px] pb-[20px] max-[700px]:flex-wrap">
            <input
              className="w-[295px] max-w-[295px] min-w-[100px] flex-[1_1_180px] rounded-full"
              placeholder="Search issues..."
              value={search.q ?? ""}
              onChange={(event) => setFilter("q", event.target.value)}
              aria-label="Search issues"
            />
            <Filter
              name="Status"
              value={search.status ?? all}
              onChange={(value) => setFilter("status", value)}
              options={statusOptions}
            />
            <Filter
              name="Assignee"
              value={search.assignee ?? all}
              onChange={(value) => setFilter("assignee", value)}
              options={assigneeOptions}
            />
            <Filter
              name="Label"
              value={search.label ?? all}
              onChange={(value) => setFilter("label", value)}
              options={labelOptions}
            />
            <Filter
              name="Milestone"
              value={search.milestone ?? all}
              onChange={(value) => setFilter("milestone", value)}
              options={milestoneOptions}
            />
            <div
              className={`relative isolate ml-auto inline-flex shrink-0 gap-[2px] rounded-full border border-[#292e38] bg-[#101216] p-[3px] before:pointer-events-none before:absolute before:inset-y-[3px] before:left-[3px] before:z-0 before:w-[calc(50%-4px)] before:rounded-full before:bg-[#273142] before:transition-transform before:duration-220 before:ease-[cubic-bezier(0.2,0.8,0.2,1)] before:content-[''] motion-reduce:before:transition-none ${view === "board" ? "before:translate-x-[calc(100%+2px)]" : ""}`}
              role="group"
              aria-label="Issue view"
            >
              <button
                type="button"
                className="relative z-1 inline-flex flex-[0_0_84px] items-center justify-center gap-[6px] rounded-full border-0 bg-transparent px-[10px] py-[6px] text-[12px] text-[#9ba9bc] transition-colors duration-180 ease-[ease] hover:text-[#edf0f6] aria-pressed:text-[#edf0f6] motion-reduce:transition-none"
                aria-pressed={view !== "board"}
                onClick={() => switchView("list")}
              >
                <Icon name="list" className="h-[15px] w-[15px]" /> List
              </button>
              <button
                type="button"
                className="relative z-1 inline-flex flex-[0_0_84px] items-center justify-center gap-[6px] rounded-full border-0 bg-transparent px-4 py-[8px] text-[12px] text-[#9ba9bc] transition-colors duration-180 ease-[ease] hover:text-[#edf0f6] aria-pressed:text-[#edf0f6] motion-reduce:transition-none"
                aria-pressed={view === "board"}
                onClick={() => switchView("board")}
              >
                <Columns3
                  aria-hidden="true"
                  className="inline-block size-[15px] flex-none align-[-3px]"
                />{" "}
                Board
              </button>
            </div>
            <button
              className="rounded-full border border-[#5f7fff] bg-[#2455eb] px-[15px] py-[6px] font-[550] whitespace-nowrap text-white shadow-[0_4px_14px_#2455eb38,inset_0_1px_#ffffff24] transition-[background-color,border-color,box-shadow,transform] duration-160 ease-[ease] hover:-translate-y-px hover:border-[#89a0ff] hover:bg-[#3566f6] hover:shadow-[0_6px_18px_#2455eb55,inset_0_1px_#ffffff33] motion-reduce:transition-none"
              onClick={() => setCreating(true)}
            >
              <Icon name="plus" /> &nbsp; New issue
            </button>
          </div>
        </div>
      </div>
      {view === "board" ? (
        <IssueBoard issues={filtered} statusFilter={search.status} />
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
        statuses={statusOptions}
        assignees={assigneeOptions}
        milestones={milestoneOptions}
        labels={labels}
        onCreate={createIssue}
        onClose={() => setCreating(false)}
      />
    </>
  );
}
