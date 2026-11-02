import { formatRelative } from "../lib/format";
import { isUnresolved } from "../lib/finding-status";
import type { Finding, ReviewRound } from "../lib/types";
import { Icon } from "./icon";
import { ReviewPerson } from "./review-person";

type ReviewRoundListProps = {
  rounds: ReviewRound[];
  findings: Finding[];
  onOpen: (round: ReviewRound) => void;
};

/** Lists newest rounds first, separating review completion from finding resolution. */
export function ReviewRoundList({ rounds, findings, onOpen }: ReviewRoundListProps) {
  return (
    <div className="space-y-2">
      {[...rounds]
        .sort((left, right) => right.number - left.number)
        .map((round) => {
          const items = findings.filter((finding) => finding.review_round_id === round.id);
          const unresolved = items.filter(isUnresolved).length;
          const active = !round.finished_at;

          return (
            <button
              type="button"
              key={round.id}
              onClick={() => onOpen(round)}
              aria-label={`Open review round ${round.number}`}
              className="group block w-full rounded-[10px] px-3 py-4 text-left text-[14.5px] transition-colors hover:bg-[#18191a] focus-visible:outline-2 focus-visible:outline-[#6c82ff]"
            >
              <span className="flex items-center justify-between gap-4">
                <span className="text-[16px] font-semibold">Round {round.number}</span>
                <span className="flex items-center gap-3 text-[13px]">
                  <span className={active ? "text-[#89a5ff]" : "text-[#97a4b7]"}>
                    {active ? "● In progress" : "Finished"}
                  </span>
                  <Icon name="chevron-right" className="text-[#8290a4]" />
                </span>
              </span>
              <span className="mt-2 flex flex-wrap items-center gap-2 text-[13px] text-[#97a4b7]">
                <ReviewPerson actorId={round.author_actor_id} />
                <span>
                  · {active ? "Started " : ""}
                  {formatRelative(round.finished_at ?? round.created_at)}
                </span>
              </span>
              {(active ? round.plan_coverage : round.summary) && (
                <span className="mt-3 block leading-relaxed wrap-anywhere whitespace-pre-wrap text-[#b7c3d4]">
                  {active ? round.plan_coverage : round.summary}
                </span>
              )}
              <span className="mt-3 flex flex-wrap items-center gap-3 text-[13px] text-[#97a4b7]">
                <span>
                  {items.length
                    ? `${items.length} ${items.length === 1 ? "finding" : "findings"}${active ? " so far" : ""}`
                    : "No findings"}
                  {unresolved > 0 && (
                    <span className="ml-3 text-[#e8b35e]">
                      · {unresolved} {unresolved === 1 ? "needs" : "need"} attention
                    </span>
                  )}
                </span>
              </span>
            </button>
          );
        })}
    </div>
  );
}
