import { useEffect, useRef, useState } from "react";
import { useProject } from "../hooks/use-project";
import { Link, Navigate, createFileRoute } from "@tanstack/react-router";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Discussion } from "../components/discussion";
import { EventLine } from "../components/event-line";
import { Icon, type IconName } from "../components/icon";
import { AttachmentsPanel } from "../components/attachments-panel";
import { IssuePlan } from "../components/issue-plan";
import { IssuePrioritySelect } from "../components/issue-priority-select";
import { IssueTextField } from "../components/issue-text-field";
import { IssueProperties } from "../components/issue-properties";
import { NotFoundState } from "../components/not-found-state";
import { IssueReviews } from "../components/issue-reviews";
import { IssueDetailSkeleton } from "../components/skeleton";
import { SubissueList } from "../components/subissue-list";
import { subissuesQuery } from "../queries/subissues";
import { Tabs } from "../components/tabs";
import { slugify } from "../lib/format";
import { statusIcon } from "../lib/issue-status";
import { validateIssueSearch, type IssueTab } from "../lib/search";
import type { Issue } from "../lib/types";
import { issueAttachmentsQuery } from "../queries/attachments";
import {
  issueCommentsQuery,
  issueEventsQuery,
  issueFindingsQuery,
  issueReviewRoundsQuery,
} from "../queries/issue-resources";
import { issueQuery } from "../queries/issues";
import { actorsQuery, projectsQuery } from "../queries/project";
import { projectLabelsQuery, projectMilestonesQuery } from "../queries/project-resources";
import { loadQuery } from "../queries/options";
import { useShortcutKeys } from "../components/shortcuts";
import { isTypingTarget } from "../lib/shortcuts";

export const Route = createFileRoute("/$projectSlug/issues_/$issue")({
  staticData: { section: "issues" },
  validateSearch: validateIssueSearch,
  pendingComponent: IssueDetailSkeleton,
  loader: async ({ context, params }) => {
    const projects = await loadQuery(context.queryClient, projectsQuery);
    const project = projects.find((item) => item.slug === params.projectSlug);
    void context.queryClient.prefetchQuery(subissuesQuery(params.issue));
    void context.queryClient.prefetchQuery(issueFindingsQuery(params.issue));
    void context.queryClient.prefetchQuery(issueReviewRoundsQuery(params.issue));
    void context.queryClient.prefetchQuery(issueAttachmentsQuery(params.issue));
    void context.queryClient.prefetchQuery(issueCommentsQuery(params.issue));
    void context.queryClient.prefetchQuery(issueEventsQuery(params.issue));
    await Promise.all([
      loadQuery(context.queryClient, issueQuery(params.issue)),
      loadQuery(context.queryClient, actorsQuery),
      ...(project
        ? [
            loadQuery(context.queryClient, projectLabelsQuery(project.key)),
            loadQuery(context.queryClient, projectMilestonesQuery(project.key)),
          ]
        : []),
    ]);
  },
  component: IssuePage,
});

const tabs: { id: IssueTab; label: string; icon: IconName }[] = [
  { id: "plan", label: "Plan", icon: "file-text" },
  { id: "subissues", label: "Subissues", icon: "square-check" },
  { id: "rounds", label: "Review rounds", icon: "list-todo" },
  { id: "findings", label: "Findings", icon: "list-filter" },
  { id: "activity", label: "Activity", icon: "activity" },
  { id: "comments", label: "Comments", icon: "message-square" },
];

/** An issue's plan, review rounds, activity, and comments; the selected tab lives in the URL. */
function IssuePage() {
  const project = useProject();
  const { issue: issueKey } = Route.useParams();
  const { data: issue } = useSuspenseQuery(issueQuery(issueKey));
  const { data: projects } = useSuspenseQuery(projectsQuery);

  // An old key, from before the issue moved to another project, opens the issue at its new key.
  const home = projects.find((item) => item.id === issue?.project_id);
  if (issue && home && issue.key !== issueKey) {
    return (
      <Navigate
        to="/$projectSlug/issues/$issue"
        params={{ projectSlug: home.slug, issue: issue.key }}
        search={(previous) => previous}
        replace
      />
    );
  }

  if (!issue || issue.project_id !== project.id) {
    return (
      <NotFoundState
        title="Issue not found"
        description={`There is no issue called ${issueKey}.`}
      />
    );
  }

  return <IssueDetail key={issue.id} issue={issue} />;
}

type IssueDetailProps = {
  issue: Issue;
};

function IssueDetail({ issue }: IssueDetailProps) {
  const project = useProject();
  const search = Route.useSearch();
  const { tab = "plan" } = search;
  const navigate = Route.useNavigate();
  const transitionRun = useRef(0);
  const [creatingSubissue, setCreatingSubissue] = useState(false);
  const subissuesQueryResult = useQuery(subissuesQuery(issue.key));
  const roundsQueryResult = useQuery(issueReviewRoundsQuery(issue.key));
  const findingsQueryResult = useQuery(issueFindingsQuery(issue.key));
  const activity = useQuery({ ...issueEventsQuery(issue.key), enabled: tab === "activity" });
  const subissues = subissuesQueryResult.data ?? [];
  const rounds = roundsQueryResult.data ?? [];
  const findings = findingsQueryResult.data ?? [];

  const switchTab = (nextTab: IssueTab) => {
    if (nextTab === tab) return;
    const updateTab = async () => {
      await navigate({ search: { tab: nextTab === "plan" ? undefined : nextTab }, replace: true });
    };
    const pageDocument = document as Document & {
      startViewTransition?: (update: () => Promise<void>) => { finished: Promise<void> };
    };
    if (
      !pageDocument.startViewTransition ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      void updateTab();
      return;
    }
    const run = ++transitionRun.current;
    document.documentElement.dataset.issueTabDirection =
      tabs.findIndex((item) => item.id === nextTab) > tabs.findIndex((item) => item.id === tab)
        ? "forward"
        : "backward";
    const transition = pageDocument.startViewTransition(updateTab);
    const cleanup = () => {
      if (transitionRun.current === run) delete document.documentElement.dataset.issueTabDirection;
    };
    void transition.finished.then(cleanup, cleanup);
  };

  const moveTab = (step: 1 | -1) => {
    const index = tabs.findIndex((item) => item.id === tab);
    switchTab(tabs[(index + step + tabs.length) % tabs.length]!.id);
  };

  useShortcutKeys({
    previousTab: () => moveTab(-1),
    nextTab: () => moveTab(1),
    newSubissue: () => {
      switchTab("subissues");
      setCreatingSubissue(true);
    },
    planTab: () => switchTab("plan"),
    roundsTab: () => switchTab("rounds"),
    findingsTab: () => switchTab("findings"),
    activityTab: () => switchTab("activity"),
    commentsTab: () => switchTab("comments"),
    editTitle: () => document.querySelector<HTMLInputElement>("h1 input, h1 textarea")?.focus(),
  });

  useEffect(() => {
    const leave = (event: KeyboardEvent) => {
      if (
        event.key !== "Escape" ||
        event.defaultPrevented ||
        event.isComposing ||
        isTypingTarget(event.target) ||
        document.querySelector('[role="dialog"]:not([hidden])')
      )
        return;

      void navigate({
        to: "/$projectSlug/issues",
        params: { projectSlug: project.slug },
        search: {},
      });
    };

    window.addEventListener("keydown", leave);

    return () => window.removeEventListener("keydown", leave);
  }, [navigate, project.slug]);

  return (
    <div
      className={`grid grid-cols-[minmax(0,1fr)_440px] max-[1100px]:grid-cols-[minmax(0,1fr)_360px] max-[700px]:block [&_.issue-icon-approach]:text-[#b28aff] [&_.issue-icon-branch]:text-[#b28aff] [&_.issue-icon-checklist]:text-[#61c99d] [&_.issue-icon-comments]:text-[#7bbdcf] [&_.issue-icon-goal]:text-[#7c9dff] [&_.issue-icon-pull-request]:text-[#7fcd9c] [&_.issue-icon-review]:text-[#a38af1] [&_.issue-icon-warning]:text-[#e8ae62] ${tab === "comments" ? "h-dvh grid-rows-[minmax(0,1fr)] overflow-hidden max-[700px]:h-auto max-[700px]:overflow-visible" : "min-h-screen"}`}
    >
      <div
        className={`px-[32px] pt-[27px] pb-0 max-[700px]:px-[20px] max-[700px]:py-[24px] ${tab === "comments" ? "flex min-h-0 flex-col" : tab === "findings" || tab === "rounds" ? "flex flex-col" : ""}`}
      >
        <h1>
          <IssueTextField
            key={issue.id}
            issue={issue}
            field="title"
            label="Issue title"
            className="h-auto w-full rounded-none border-0 bg-transparent p-0 text-[28px] font-semibold text-[#f2f5fc]"
          />
        </h1>
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-[#a9b8ca]">
          <span className="inline-flex items-center gap-2">
            <Icon name={statusIcon[issue.status]} className={`status-${slugify(issue.status)}`} />
            {issue.key}
          </span>
          <IssuePrioritySelect issue={issue} />
          {issue.branch_name && (
            <span className="inline-flex min-w-0 items-center gap-2">
              <Icon name="git-branch" className="issue-icon-branch" />
              <span className="truncate">{issue.branch_name}</span>
            </span>
          )}
          <span className="inline-flex items-center gap-2">
            <Icon name="message-square" className="issue-icon-comments" />
            {issue.comment_count} {issue.comment_count === 1 ? "comment" : "comments"}
          </span>
        </div>
        <Tabs className="mt-[27px] overflow-x-auto border-b border-b-[#252a32] [&>a.selected]:after:hidden">
          {tabs.map(({ id, label, icon }) => (
            <Link
              key={id}
              to="/$projectSlug/issues/$issue"
              params={{ projectSlug: project.slug, issue: issue.key }}
              search={{ tab: id === "plan" ? undefined : id }}
              replace
              onClick={(event) => {
                if (
                  event.button !== 0 ||
                  event.metaKey ||
                  event.ctrlKey ||
                  event.shiftKey ||
                  event.altKey
                )
                  return;
                event.preventDefault();
                switchTab(id);
              }}
              className={`inline-flex items-center gap-2 whitespace-nowrap ${tab === id ? "selected" : ""}`}
            >
              <Icon name={icon} className="size-4" />
              {label}{" "}
              {tab === id && (
                <span
                  aria-hidden="true"
                  className="absolute inset-x-0 bottom-0 h-[3px] rounded-t-[3px] bg-[#426fff] [view-transition-name:issue-tab-indicator]"
                />
              )}
              {id === "subissues" && (
                <span className="count rounded-[5px] bg-[#262b34] px-2 py-[3px] text-[12px] text-[#dce2ec]">
                  {subissues.length}
                </span>
              )}
              {id === "rounds" && (
                <span className="count rounded-[5px] bg-[#262b34] px-2 py-[3px] text-[12px] text-[#dce2ec]">
                  {rounds.length}
                </span>
              )}
            </Link>
          ))}
        </Tabs>
        <div
          className={`[view-transition-name:issue-tab-content] ${tab === "comments" ? "flex min-h-0 flex-1 flex-col" : tab === "findings" || tab === "rounds" ? "flex flex-1 flex-col" : "flow-root"}`}
        >
          {tab === "plan" && (
            <>
              <IssuePlan key={issue.id} issue={issue} />
              <AttachmentsPanel key={`attachments-${issue.id}`} scope={{ issueKey: issue.key }} />
            </>
          )}
          {tab === "subissues" && (
            <SubissueList
              key={issue.id}
              issue={issue}
              creating={creatingSubissue}
              onCreatingChange={setCreatingSubissue}
            />
          )}
          {(tab === "rounds" || tab === "findings") &&
            (roundsQueryResult.isPending || findingsQueryResult.isPending ? (
              <p role="status" className="muted py-[20px] text-[#a2aec2]">
                Loading reviews…
              </p>
            ) : (
              <IssueReviews
                issue={issue}
                rounds={rounds}
                findings={findings}
                search={search}
                onSearchChange={(next) => {
                  void navigate({ search: next });
                }}
              />
            ))}
          {tab === "activity" && (
            <div className="px-0 py-[20px]">
              {activity.isPending && (
                <p role="status" className="muted text-[#a2aec2]">
                  Loading activity…
                </p>
              )}
              {activity.data && activity.data.length === 0 && (
                <p className="muted text-[#a2aec2]">No activity yet.</p>
              )}
              {activity.data &&
                [...activity.data]
                  .reverse()
                  .map((event) => <EventLine key={event.id} event={event} issue={issue} />)}
            </div>
          )}
          {tab === "comments" && (
            <div className="flex min-h-0 flex-1 flex-col pt-[20px] pb-[24px]">
              <Discussion issueKey={issue.key} />
            </div>
          )}
        </div>
      </div>
      <IssueProperties issue={issue} />
    </div>
  );
}
