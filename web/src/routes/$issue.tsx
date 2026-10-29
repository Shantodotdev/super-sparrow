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
    <div className="issue-layout">
      <div className="issue-body">
        <div className="breadcrumb">
          <Link to="/issues">Issues</Link> / {issue.key}
        </div>
        <h1>{issue.title}</h1>
        <div className="issue-meta">
          <Icon name={statusIcon[issue.status]} className={`status-${slugify(issue.status)}`} />{" "}
          &nbsp;{issue.key}　 ·　 <Icon name="git-branch" className="issue-icon-branch" /> &nbsp;
          {branch}　 ·　 <Icon name="message-square" className="issue-icon-comments" /> &nbsp;{" "}
          {commentCount} comments
        </div>
        {issue.plan === undefined && <p className="issue-summary">{summary}</p>}
        <div className="tabs issue-tabs">
          {tabs.map(({ id, label }) => (
            <Link
              key={id}
              to="/$issue"
              params={{ issue: issue.key }}
              search={{ tab: id === "plan" ? undefined : id }}
              replace
              className={tab === id ? "selected" : ""}
            >
              {label} {id === "rounds" && <span className="count">{rounds.length}</span>}
            </Link>
          ))}
        </div>
        {tab === "plan" && issue.plan !== undefined && (
          <div className="plan issue-created-plan">{issue.plan || "No plan added yet."}</div>
        )}
        {tab === "plan" && issue.plan === undefined && (
          <IssuePlan checks={checks} onToggleCheck={toggleCheck} />
        )}
        {tab === "rounds" && (
          <div className="inline-list">
            <ReviewRoundList rounds={rounds} findings={findings} />
          </div>
        )}
        {tab === "activity" && (
          <div className="inline-list">
            <Person name="Shanto" />{" "}
            <span className="muted">created this issue and added the initial plan.</span>
          </div>
        )}
        <div className="issue-activity">
          <h2>Activity</h2>
          <p>
            <Person name="Shanto" />{" "}
            <span className="muted">created this issue. Added initial plan and approach.</span>
          </p>
        </div>
      </div>
      <IssueProperties issue={issue} rounds={rounds} findings={findings} />
    </div>
  );
}
