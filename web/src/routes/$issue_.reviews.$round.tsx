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
      <header className="px-8 pt-[25px] pb-[18px] max-[700px]:px-[16px] max-[700px]:py-[22px] [&_.breadcrumb]:mb-[24px]">
        <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
          <Link to="/$issue" params={{ issue: issue.key }}>
            {issue.key}
          </Link>{" "}
          / Reviews / Round {round}
        </div>
        <div className="flex items-start justify-between gap-[24px] max-[700px]:flex-wrap">
          <div>
            <h1>Review round {round}</h1>
            <p className="mt-4 text-[#a9b8ca]">
              {currentRound && (
                <>
                  <Icon name="git-branch" /> &nbsp; {currentRound.from} &nbsp;{" "}
                  <Icon name="arrow-right" /> &nbsp;
                  {currentRound.to}　 ·　 <Icon name="clock" /> &nbsp; {currentRound.date} ·{" "}
                  {currentRound.author}
                </>
              )}
            </p>
            <p className="mt-[16px] text-[16px] text-[#abb8cc]">
              {currentRound?.summary ?? "This review round is not available in the preview."}
            </p>
          </div>
          <div className="flex gap-[20px] max-[700px]:w-full [&_>_.custom-select]:w-[150px] [&_select]:w-[150px]">
            <Select
              label="Review round"
              value={round}
              onChange={(value) => void navigate({ params: { issue: issue.key, round: value } })}
              options={rounds.map((item) => ({
                value: String(item.number),
                label: `Round ${item.number}`,
              }))}
            />
            <Link
              to="/findings"
              className="text-button border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
            >
              {openCount} open findings
              <Icon name="chevron-down" />
            </Link>
          </div>
        </div>
      </header>
      <div className="grid grid-cols-[minmax(0,1.1fr)_minmax(380px,1fr)] border-t border-t-[#252a32] max-[700px]:block [&_>_div:first-child]:min-w-0">
        <div>
          <div className="border-b border-b-[#252a32] px-8 py-[18px] font-semibold after:relative after:top-[18px] after:block after:h-[3px] after:w-32 after:bg-[#6486ff] after:content-['']">
            Findings{" "}
            <span className="count rounded-[5px] bg-[#262b34] px-2 py-[3px] text-[12px] text-[#dce2ec]">
              {roundFindings.length}
            </span>
          </div>
          {roundFindings.length ? (
            <div className="p-[12px]">
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
            <div className="mx-auto my-40 max-w-120 p-[24px] text-center text-[#93a0b2] [&_h2]:mx-0 [&_h2]:mt-[18px] [&_h2]:mb-2 [&_h2]:text-[#e9edf5]">
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
