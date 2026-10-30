import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { FindingCard } from "../components/finding-card";
import { FindingDetail } from "../components/finding-detail";
import { Icon } from "../components/icon";
import { NotFoundState } from "../components/not-found-state";
import { Select } from "../components/select";
import { formatDate, shortSha } from "../lib/format";
import { validateReviewSearch } from "../lib/search";
import type { Finding, Issue, ReviewRound } from "../lib/types";
import { findingsQuery } from "../queries/findings";
import { issuesQuery } from "../queries/issues";
import {
  actorsQuery,
  commentsQuery,
  labelsQuery,
  reviewRoundsQuery,
  useActor,
} from "../queries/project";
import { loadQuery } from "../queries/options";

export const Route = createFileRoute("/$issue_/reviews/$round")({
  staticData: { section: "issues" },
  validateSearch: validateReviewSearch,
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, issuesQuery),
      loadQuery(context.queryClient, findingsQuery),
      loadQuery(context.queryClient, reviewRoundsQuery),
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, commentsQuery),
      loadQuery(context.queryClient, labelsQuery),
    ]);
  },
  component: ReviewPage,
});

/** One review round of an issue: its findings on the left and the selected finding on the right. */
function ReviewPage() {
  const { issue: issueKey, round: roundParam } = Route.useParams();
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

  const issueRounds = rounds.filter((item) => item.issue_id === issue.id);
  const round = issueRounds.find((item) => String(item.number) === roundParam);
  if (!round) {
    return (
      <NotFoundState
        title="Review round not found"
        description={`${issue.key} has no review round ${roundParam}.`}
      />
    );
  }

  return (
    <ReviewRoundView
      issue={issue}
      round={round}
      rounds={issueRounds}
      roundFindings={findings
        .filter((item) => item.review_round_id === round.id)
        .sort((a, b) => a.number - b.number)}
    />
  );
}

type ReviewRoundViewProps = {
  issue: Issue;
  round: ReviewRound;
  rounds: ReviewRound[];
  roundFindings: Finding[];
};

/** A round's findings and the selected one, which the URL names by label so it can be linked. */
function ReviewRoundView({ issue, round, rounds, roundFindings }: ReviewRoundViewProps) {
  const navigate = Route.useNavigate();
  const { finding: selectedLabel } = Route.useSearch();
  const actor = useActor();

  const selectedFinding =
    roundFindings.find((item) => item.label === selectedLabel) ?? roundFindings[0];

  return (
    <>
      <header className="px-8 pt-[25px] pb-[18px] max-[700px]:px-[16px] max-[700px]:py-[22px] [&_.breadcrumb]:mb-[24px]">
        <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
          <Link to="/$issue" params={{ issue: issue.key }}>
            {issue.key}
          </Link>{" "}
          / Reviews / Round {round.number}
        </div>
        <div className="flex items-start justify-between gap-[24px] max-[700px]:flex-wrap">
          <div>
            <h1>Review round {round.number}</h1>
            <p className="mt-4 text-[#a9b8ca]">
              <Icon name="git-branch" /> &nbsp; {shortSha(round.from_commit) || "branch start"}{" "}
              &nbsp; <Icon name="arrow-right" /> &nbsp;
              {shortSha(round.to_commit)}
              {round.includes_uncommitted && " and uncommitted changes"}　 ·　 <Icon name="clock" />{" "}
              &nbsp; {formatDate(round.finished_at ?? round.created_at)} ·{" "}
              {actor(round.author_actor_id)?.display_name ?? "Unknown"}
              {!round.finished_at && " · in progress"}
            </p>
            {round.summary && (
              <p className="mt-[16px] text-[16px] whitespace-pre-wrap text-[#abb8cc]">
                {round.summary}
              </p>
            )}
          </div>
          <div className="flex gap-[20px] max-[700px]:w-full [&_>_.custom-select]:w-[150px] [&_select]:w-[150px]">
            <Select
              label="Review round"
              value={String(round.number)}
              onChange={(value) =>
                void navigate({ params: { issue: issue.key, round: value }, search: {} })
              }
              options={rounds.map((item) => ({
                value: String(item.number),
                label: `Round ${item.number}`,
              }))}
            />
            <Link
              to="/findings"
              search={{ issue: issue.key }}
              className="text-button border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
            >
              {issue.unresolved_findings} unresolved{" "}
              {issue.unresolved_findings === 1 ? "finding" : "findings"}
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
              {roundFindings.map((finding) => (
                <FindingCard
                  key={finding.id}
                  finding={finding}
                  selected={finding === selectedFinding}
                  onSelect={() =>
                    void navigate({ search: { finding: finding.label }, replace: true })
                  }
                />
              ))}
            </div>
          ) : (
            <div className="mx-auto my-40 max-w-120 p-[24px] text-center text-[#93a0b2] [&_h2]:mx-0 [&_h2]:mt-[18px] [&_h2]:mb-2 [&_h2]:text-[#e9edf5]">
              <h2>No findings</h2>
              <p>This round found no problems.</p>
            </div>
          )}
        </div>
        {selectedFinding && <FindingDetail key={selectedFinding.id} finding={selectedFinding} />}
      </div>
    </>
  );
}
