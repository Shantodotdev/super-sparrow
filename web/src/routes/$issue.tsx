import { Tabs } from "../components/tabs";
import { useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Icon } from "../components/icon";
import { IssuePlan } from "../components/issue-plan";
import { IssueProperties } from "../components/issue-properties";
import { NotFoundState } from "../components/not-found-state";
import { Person } from "../components/person";
import { ReviewRoundList } from "../components/review-round-list";
import { sampleIssueDetails } from "../data/issue-details";
import { slugify } from "../lib/format";
import { statusIcon } from "../lib/issue-status";
import { validateIssueSearch, type IssueTab } from "../lib/search";
import type { Finding, Issue, ReviewRound } from "../lib/types";
import { findingsQuery } from "../queries/findings";
import { issuesQuery } from "../queries/issues";
import { reviewRoundsQuery } from "../queries/review-rounds";

export const Route = createFileRoute("/$issue")({
  staticData: { section: "issues" },
  validateSearch: validateIssueSearch,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...issuesQuery, staleTime: "static" }),
      context.queryClient.query({ ...findingsQuery, staleTime: "static" }),
      context.queryClient.query({ ...reviewRoundsQuery, staleTime: "static" }),
    ]);
  },
  component: IssuePage,
});

const tabs: { id: IssueTab; label: string }[] = [
  { id: "plan", label: "Plan" },
  { id: "rounds", label: "Review rounds" },
  { id: "activity", label: "Activity" },
];

/** An issue's plan, review rounds, and properties; the selected tab lives in the URL. */
function IssuePage() {
  const { issue: issueKey } = Route.useParams();
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const { data: findings } = useSuspenseQuery(findingsQuery);
  const { data: rounds } = useSuspenseQuery(reviewRoundsQuery);

  const issue = issues.find((item) => item.key === issueKey);
  if (!issue) {
    return (
      <NotFoundState
        title="Issue not found"
        description={`There is no issue called ${issueKey}.`}
      />
    );
  }

  return (
    <IssueDetail
      issue={issue}
      rounds={rounds.filter((round) => round.issue === issue.key)}
      findings={findings.filter((finding) => finding.issue === issue.key)}
    />
  );
}

type IssueDetailProps = { issue: Issue; rounds: ReviewRound[]; findings: Finding[] };

function IssueDetail({ issue, rounds, findings }: IssueDetailProps) {
  const { tab = "plan" } = Route.useSearch();
  const [checks, setChecks] = useState(() =>
    sampleIssueDetails.plan.checklist.map((item) => item.done),
  );
  const { branch, commentCount, summary } = sampleIssueDetails;

  const toggleCheck = (index: number) =>
    setChecks((items) => items.map((done, i) => (i === index ? !done : done)));

  return (
    <div className="grid min-h-screen grid-cols-[minmax(0,1fr)_410px] max-[1100px]:grid-cols-[minmax(0,1fr)_310px] max-[700px]:block [&_.issue-icon-approach]:text-[#b28aff] [&_.issue-icon-branch]:text-[#b28aff] [&_.issue-icon-checklist]:text-[#61c99d] [&_.issue-icon-comments]:text-[#7bbdcf] [&_.issue-icon-goal]:text-[#7c9dff] [&_.issue-icon-pull-request]:text-[#7fcd9c] [&_.issue-icon-review]:text-[#a38af1] [&_.issue-icon-warning]:text-[#e8ae62]">
      <div className="px-[32px] pt-[27px] pb-0 max-[700px]:px-[20px] max-[700px]:py-[24px]">
        <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
          <Link to="/issues">Issues</Link> / {issue.key}
        </div>
        <h1>{issue.title}</h1>
        <div className="mt-4 text-[#a9b8ca]">
          <Icon name={statusIcon[issue.status]} className={`status-${slugify(issue.status)}`} />{" "}
          &nbsp;{issue.key}　 ·　 <Icon name="git-branch" className="issue-icon-branch" /> &nbsp;
          {branch}　 ·　 <Icon name="message-square" className="issue-icon-comments" /> &nbsp;{" "}
          {commentCount} comments
        </div>
        {issue.plan === undefined && (
          <p className="mt-[16px] text-[16px] text-[#abb8cc]">{summary}</p>
        )}
        <Tabs className="mt-[27px] border-b border-b-[#252a32]">
          {tabs.map(({ id, label }) => (
            <Link
              key={id}
              to="/$issue"
              params={{ issue: issue.key }}
              search={{ tab: id === "plan" ? undefined : id }}
              replace
              className={tab === id ? "selected" : ""}
            >
              {label}{" "}
              {id === "rounds" && (
                <span className="count rounded-[5px] bg-[#262b34] px-2 py-[3px] text-[12px] text-[#dce2ec]">
                  {rounds.length}
                </span>
              )}
            </Link>
          ))}
        </Tabs>
        {tab === "plan" && issue.plan !== undefined && (
          <div className="max-w-[850px] whitespace-pre-wrap [&_h2]:mb-[8px] [&_ol]:list-decimal [&_ol]:pl-[18px] [&_ol]:leading-[1.55] [&_ol]:text-[#b4c1d3] [&_p]:leading-[1.55] [&_p]:text-[#b4c1d3] [&_section]:mt-[24px] [&_section]:grid [&_section]:grid-cols-[38px_1fr] [&_section]:gap-[10px] [&_ul]:list-disc [&_ul]:pl-[18px] [&_ul]:leading-[1.55] [&_ul]:text-[#b4c1d3]">
            {issue.plan || "No plan added yet."}
          </div>
        )}
        {tab === "plan" && issue.plan === undefined && (
          <IssuePlan checks={checks} onToggleCheck={toggleCheck} />
        )}
        {tab === "rounds" && (
          <div className="px-0 py-[20px]">
            <ReviewRoundList rounds={rounds} findings={findings} />
          </div>
        )}
        {tab === "activity" && (
          <div className="px-0 py-[20px]">
            <Person name="Shanto" />{" "}
            <span className="muted text-[#a2aec2]">
              created this issue and added the initial plan.
            </span>
          </div>
        )}
        <div className="mx-[-32px] mt-[25px] mb-0 border-t border-t-[#232832] px-[32px] py-[20px] [&_p]:mt-[16px]">
          <h2>Activity</h2>
          <p>
            <Person name="Shanto" />{" "}
            <span className="muted text-[#a2aec2]">
              created this issue. Added initial plan and approach.
            </span>
          </p>
        </div>
      </div>
      <IssueProperties issue={issue} rounds={rounds} findings={findings} />
    </div>
  );
}
