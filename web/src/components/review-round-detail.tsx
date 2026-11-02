import { formatDate, formatRelative } from "../lib/format";
import { isUnresolved } from "../lib/finding-status";
import { orderFindings } from "../lib/review";
import type { Finding, Issue, ReviewRound } from "../lib/types";
import { FindingRow } from "./finding-row";
import { reviewButton, reviewSecondaryButton } from "./review-forms";
import { ReviewPerson } from "./review-person";

/** Shows a round's coverage, summary, and findings without mixing in finding decisions. */
export function ReviewRoundDetail({
  issue,
  round,
  findings,
  onFindingOpen,
  onAddFinding,
  onFinish,
}: {
  issue: Issue;
  round: ReviewRound;
  findings: Finding[];
  onFindingOpen: (finding: Finding) => void;
  onAddFinding: () => void;
  onFinish: () => void;
}) {
  const unresolved = findings.filter(isUnresolved).length;

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-[23px] font-semibold">Review round {round.number}</h2>
        <span className={`text-[13px] ${round.finished_at ? "text-[#97a4b7]" : "text-[#89a5ff]"}`}>
          {round.finished_at ? "Finished" : "● In progress"}
        </span>
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-2 text-[13px] text-[#97a4b7]">
        <ReviewPerson actorId={round.author_actor_id} />
        <time
          dateTime={round.finished_at ?? round.created_at}
          title={formatDate(round.finished_at ?? round.created_at)}
        >
          · {formatRelative(round.finished_at ?? round.created_at)}
        </time>
      </div>
      <p className="mt-3 text-[13px] text-[#8290a4]">{issue.title}</p>
      <section className="mt-6 border-t border-[#252a32] pt-5">
        <h3 className="mb-2 font-semibold">Plan coverage</h3>
        <p className="leading-relaxed wrap-break-word whitespace-pre-wrap text-[#b7c3d4]">
          {round.plan_coverage || "No plan coverage note provided."}
        </p>
      </section>
      <section className="mt-6 border-t border-[#252a32] pt-5">
        <h3 className="mb-2 font-semibold">Summary</h3>
        <p className="leading-relaxed wrap-break-word whitespace-pre-wrap text-[#b7c3d4]">
          {round.summary ||
            (round.finished_at
              ? "No summary provided."
              : "The reviewer will add a summary when this round is finished.")}
        </p>
      </section>
      {!round.finished_at && (
        <div className="mt-5 flex flex-wrap gap-2">
          <button type="button" className={reviewButton} onClick={onAddFinding}>
            Add finding
          </button>
          <button type="button" className={reviewSecondaryButton} onClick={onFinish}>
            Finish review round
          </button>
        </div>
      )}
      <section className="mt-6 border-t border-[#252a32] pt-5" aria-label="Round findings">
        <div className="mb-2 flex items-center justify-between gap-4">
          <h3 className="font-semibold">
            Findings <span className="ml-2 font-normal text-[#8290a4]">{findings.length}</span>
          </h3>
          {unresolved > 0 && (
            <span className="text-[13px] text-[#e8b35e]">
              {unresolved} {unresolved === 1 ? "needs" : "need"} attention
            </span>
          )}
        </div>
        {findings.length ? (
          orderFindings(findings).map((finding) => (
            <FindingRow
              key={finding.id}
              finding={finding}
              compact
              onOpen={() => onFindingOpen(finding)}
            />
          ))
        ) : (
          <p className="py-6 text-[13px] text-[#97a4b7]">
            {round.finished_at
              ? "No findings were raised in this review."
              : "No findings yet. Add one as you review."}
          </p>
        )}
      </section>
      {findings.length > 0 && (
        <p className="mt-5 text-[12px] text-[#8290a4]">
          Select a finding to view details and join the discussion.
        </p>
      )}
    </>
  );
}
