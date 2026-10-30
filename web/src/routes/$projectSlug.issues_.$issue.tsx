import { useProject } from "../hooks/use-project";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Discussion } from "../components/discussion";
import { EventLine, eventIssueId } from "../components/event-line";
import { Icon, type IconName } from "../components/icon";
import { IssuePlan } from "../components/issue-plan";
import { IssuePrioritySelect } from "../components/issue-priority-select";
import { IssueTextField } from "../components/issue-text-field";
import { IssueProperties } from "../components/issue-properties";
import { NotFoundState } from "../components/not-found-state";
import { ReviewRoundList } from "../components/review-round-list";
import { Tabs } from "../components/tabs";
import { isUnresolved } from "../lib/finding-status";
import { slugify } from "../lib/format";
import { statusIcon } from "../lib/issue-status";
import { validateIssueSearch, type IssueTab } from "../lib/search";
import type { ActivityEvent, Finding, Issue, ReviewRound } from "../lib/types";
import { findingsQuery } from "../queries/findings";
import { issueQuery, issuesQuery } from "../queries/issues";
import {
  activityQuery,
  actorsQuery,
  commentsQuery,
  labelsQuery,
  milestonesQuery,
  reviewRoundsQuery,
} from "../queries/project";
import { loadQuery } from "../queries/options";

export const Route = createFileRoute("/$projectSlug/issues_/$issue")({
  staticData: { section: "issues" },
  validateSearch: validateIssueSearch,
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.fetchQuery(issueQuery(params.issue)),
      loadQuery(context.queryClient, issuesQuery),
      loadQuery(context.queryClient, findingsQuery),
      loadQuery(context.queryClient, reviewRoundsQuery),
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, labelsQuery),
      loadQuery(context.queryClient, milestonesQuery),
      loadQuery(context.queryClient, commentsQuery),
      loadQuery(context.queryClient, activityQuery),
    ]);
  },
  component: IssuePage,
});

const tabs: { id: IssueTab; label: string; icon: IconName }[] = [
  { id: "plan", label: "Plan", icon: "file-text" },
  { id: "findings", label: "Findings", icon: "list-filter" },
  { id: "rounds", label: "Review rounds", icon: "list-todo" },
  { id: "activity", label: "Activity", icon: "activity" },
  { id: "comments", label: "Comments", icon: "message-square" },
];

/** An issue's plan, review rounds, activity, and comments; the selected tab lives in the URL. */
function IssuePage() {
  const project = useProject();
  const { issue: issueKey } = Route.useParams();
  const { data: issue } = useSuspenseQuery(issueQuery(issueKey));
  const { data: findings } = useSuspenseQuery(findingsQuery);
  const { data: rounds } = useSuspenseQuery(reviewRoundsQuery);
  const { data: events } = useSuspenseQuery(activityQuery);

  if (!issue || issue.project_id !== project.id) {
    return (
      <NotFoundState
        title="Issue not found"
        description={`There is no issue called ${issueKey}.`}
      />
    );
  }

  return (
    <IssueDetail
      key={issue.id}
      issue={issue}
      rounds={rounds.filter((round) => round.issue_id === issue.id)}
      findings={findings.filter((finding) => finding.issue_id === issue.id)}
      events={events.filter((event) => eventIssueId(event) === issue.id)}
    />
  );
}

type IssueDetailProps = {
  issue: Issue;
  rounds: ReviewRound[];
  findings: Finding[];
  events: ActivityEvent[];
};

function IssueDetail({ issue, rounds, findings, events }: IssueDetailProps) {
  const project = useProject();
  const { tab = "plan" } = Route.useSearch();

  return (
    <div className="grid min-h-screen grid-cols-[minmax(0,1fr)_320px] max-[1100px]:grid-cols-[minmax(0,1fr)_280px] max-[700px]:block [&_.issue-icon-approach]:text-[#b28aff] [&_.issue-icon-branch]:text-[#b28aff] [&_.issue-icon-checklist]:text-[#61c99d] [&_.issue-icon-comments]:text-[#7bbdcf] [&_.issue-icon-goal]:text-[#7c9dff] [&_.issue-icon-pull-request]:text-[#7fcd9c] [&_.issue-icon-review]:text-[#a38af1] [&_.issue-icon-warning]:text-[#e8ae62]">
      <div className="px-[32px] pt-[27px] pb-0 max-[700px]:px-[20px] max-[700px]:py-[24px]">
        <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
          <Link to="/$projectSlug/issues" params={{ projectSlug: project.slug }}>
            Issues
          </Link>{" "}
          / {issue.key}
        </div>
        <h1>
          <IssueTextField
            key={issue.id}
            issue={issue}
            field="title"
            label="Issue title"
            className="h-auto w-full rounded-none border-0 bg-transparent p-0 text-[28px] font-semibold text-[#f2f5fc]"
          />
        </h1>
        <div className="mt-3">
          <IssuePrioritySelect issue={issue} />
        </div>
        <div className="mt-4 text-[#a9b8ca]">
          <Icon name={statusIcon[issue.status]} className={`status-${slugify(issue.status)}`} />{" "}
          &nbsp;{issue.key}
          {issue.branch_name && (
            <>
              {"　 ·　 "}
              <Icon name="git-branch" className="issue-icon-branch" /> &nbsp;
              {issue.branch_name}
            </>
          )}
          {"　 ·　 "}
          <Icon name="message-square" className="issue-icon-comments" /> &nbsp;{" "}
          {issue.comment_count} {issue.comment_count === 1 ? "comment" : "comments"}
        </div>
        <Tabs className="mt-[27px] overflow-x-auto border-b border-b-[#252a32]">
          {tabs.map(({ id, label, icon }) => (
            <Link
              key={id}
              to="/$projectSlug/issues/$issue"
              params={{ projectSlug: project.slug, issue: issue.key }}
              search={{ tab: id === "plan" ? undefined : id }}
              replace
              className={`inline-flex items-center gap-2 whitespace-nowrap ${tab === id ? "selected" : ""}`}
            >
              <Icon name={icon} className="size-4" />
              {label}{" "}
              {id === "rounds" && (
                <span className="count rounded-[5px] bg-[#262b34] px-2 py-[3px] text-[12px] text-[#dce2ec]">
                  {rounds.length}
                </span>
              )}
            </Link>
          ))}
        </Tabs>
        {tab === "plan" && <IssuePlan key={issue.id} issue={issue} />}
        {tab === "findings" && (
          <section className="mt-8">
            <h2>Open findings</h2>
            {findings.filter(isUnresolved).length === 0 && <p>No unresolved findings.</p>}
            {findings.filter(isUnresolved).map((finding) => (
              <Link
                className="mt-3 block text-[#719bff]"
                key={finding.id}
                to="/$projectSlug/issues/$issue/reviews/$round"
                params={{
                  projectSlug: project.slug,
                  issue: issue.key,
                  round: String(finding.round),
                }}
                search={{ finding: finding.label }}
              >
                {finding.label} · {finding.title}
              </Link>
            ))}
          </section>
        )}
        {tab === "rounds" && (
          <div className="px-0 py-[20px]">
            <ReviewRoundList issue={issue} rounds={rounds} findings={findings} />
          </div>
        )}
        {tab === "activity" && (
          <div className="px-0 py-[20px]">
            {events.length === 0 && <p className="muted text-[#a2aec2]">No activity yet.</p>}
            {[...events].reverse().map((event) => (
              <EventLine key={event.id} event={event} />
            ))}
          </div>
        )}
        {tab === "comments" && (
          <div className="py-[20px]">
            <Discussion issueId={issue.id} findingId="" />
          </div>
        )}
      </div>
      <IssueProperties issue={issue} rounds={rounds} findings={findings} />
    </div>
  );
}
