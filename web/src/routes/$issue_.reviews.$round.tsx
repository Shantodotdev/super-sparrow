import { useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { FindingCard } from "../components/finding-card";
import { FindingDetail } from "../components/finding-detail";
import { Icon } from "../components/icon";
import { NotFoundState } from "../components/not-found-state";
import { Select } from "../components/select";
import type { Finding, Issue, ReviewRound } from "../lib/types";
import { findingsQuery } from "../queries/findings";
import { issuesQuery } from "../queries/issues";
import { reviewRoundsQuery } from "../queries/review-rounds";

export const Route = createFileRoute("/$issue_/reviews/$round")({
  staticData: { section: "issues" },
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...issuesQuery, staleTime: "static" }),
      context.queryClient.query({ ...findingsQuery, staleTime: "static" }),
      context.queryClient.query({ ...reviewRoundsQuery, staleTime: "static" }),
    ]);
  },
  component: ReviewPage,
});

/** One review round of an issue: its findings on the left and the selected finding on the right. */
function ReviewPage() {
  const { issue: issueKey, round } = Route.useParams();
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

  const issueRounds = rounds.filter((item) => item.issue === issue.key);
  const issueFindings = findings.filter((item) => item.issue === issue.key);

  return (
    // Keyed so the selected finding resets when the issue or round changes.
    <ReviewRoundView
      key={`${issue.key}-${round}`}
      issue={issue}
      round={round}
      rounds={issueRounds}
      issueFindings={issueFindings}
    />
  );
}

type ReviewRoundViewProps = {
  issue: Issue;
  round: string;
  rounds: ReviewRound[];
  issueFindings: Finding[];
};

function ReviewRoundView({ issue, round, rounds, issueFindings }: ReviewRoundViewProps) {
  const navigate = Route.useNavigate();
  const [selected, setSelected] = useState(0);

  const currentRound = rounds.find((item) => String(item.number) === round);
  const roundFindings = issueFindings.filter((item) => String(item.round) === round);
  const selectedFinding = roundFindings[selected] ?? roundFindings[0];
  const openCount = issueFindings.filter((item) => item.status === "Open").length;

  return (
    <>
      <header className="review-header">
        <div className="breadcrumb">
          <Link to="/$issue" params={{ issue: issue.key }}>
            {issue.key}
          </Link>{" "}
          / Reviews / Round {round}
        </div>
        <div className="title-row">
          <div>
            <h1>Review round {round}</h1>
            <p className="issue-meta">
              {currentRound && (
                <>
                  <Icon name="git-branch" /> &nbsp; {currentRound.from} &nbsp;{" "}
                  <Icon name="arrow-right" /> &nbsp;
                  {currentRound.to}　 ·　 <Icon name="clock" /> &nbsp; {currentRound.date} ·{" "}
                  {currentRound.author}
                </>
              )}
            </p>
            <p className="issue-summary">
              {currentRound?.summary ?? "This review round is not available in the preview."}
            </p>
          </div>
          <div className="review-actions">
            <Select
              label="Review round"
              value={round}
              onChange={(value) => void navigate({ params: { issue: issue.key, round: value } })}
              options={rounds.map((item) => ({
                value: String(item.number),
                label: `Round ${item.number}`,
              }))}
            />
            <Link to="/findings" className="text-button">
              {openCount} open findings
              <Icon name="chevron-down" />
            </Link>
          </div>
        </div>
      </header>
      <div className="review-split">
        <div>
          <div className="review-tab">
            Findings <span className="count">{roundFindings.length}</span>
          </div>
          {roundFindings.length ? (
            <div className="finding-cards">
              {roundFindings.map((finding, index) => (
                <FindingCard
                  key={finding.id}
                  finding={finding}
                  position={index + 1}
                  selected={index === selected}
                  onSelect={() => setSelected(index)}
                />
              ))}
            </div>
          ) : (
            <div className="empty-state">
              <h2>No findings in this preview</h2>
              <p>There are no findings for this round.</p>
            </div>
          )}
        </div>
        {selectedFinding && (
          <FindingDetail
            key={`${selectedFinding.issue}-${selectedFinding.round}-${selectedFinding.id}`}
            finding={selectedFinding}
          />
        )}
      </div>
    </>
  );
}
