import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { isUnresolved } from "../lib/finding-status";
import { orderFindingsByNumber } from "../lib/review";
import type { IssueSearch } from "../lib/search";
import type { Finding, Issue, ReviewRound } from "../lib/types";
import { FindingDetail } from "./finding-detail";
import { FindingRow } from "./finding-row";
import { Icon } from "./icon";
import { Modal } from "./modal";
import { AddFindingForm, FinishReviewForm, StartReviewForm, reviewButton } from "./review-forms";
import { ReviewRoundDetail } from "./review-round-detail";
import { ReviewRoundList } from "./review-round-list";

type ReviewForm = "start" | "add" | "finish";

type IssueReviewsProps = {
  issue: Issue;
  rounds: ReviewRound[];
  findings: Finding[];
  search: IssueSearch;
  onSearchChange: (search: IssueSearch) => void;
};

/** Keeps resolved-finding visibility and drawer navigation in the issue URL. */
export function IssueReviews({
  issue,
  rounds,
  findings,
  search,
  onSearchChange,
}: IssueReviewsProps) {
  const [form, setForm] = useState<ReviewForm | null>(null);
  const content = useRef<HTMLDivElement>(null);
  const reviewTab = search.tab === "rounds";
  const activeRound = rounds.find((round) => !round.finished_at);
  const selectedRound = rounds.find((round) => round.number === search.round);
  const selectedFinding = findings.find((finding) => finding.id === search.finding);
  const unresolved = findings.filter(isUnresolved);
  const visible = orderFindingsByNumber(unresolved);
  const resolved = orderFindingsByNumber(findings.filter((finding) => !isUnresolved(finding)));
  const showResolved = search.findingStatus === "all";
  const open = Boolean(form || search.finding || search.round);
  const addingRound = selectedRound && !selectedRound.finished_at ? selectedRound : activeRound;

  const close = () => {
    setForm(null);
    onSearchChange({ ...search, finding: undefined, round: undefined });
  };
  const openRound = (round: ReviewRound) => {
    setForm(null);
    onSearchChange({ ...search, round: round.number, finding: undefined });
  };
  const openFinding = (finding: Finding) => {
    setForm(null);
    onSearchChange({ ...search, finding: finding.id });
  };

  useEffect(() => {
    if (!open) return;
    if (!form) content.current?.focus({ preventScroll: true });
    content.current?.scrollTo(0, 0);
  }, [open, search.finding, search.round, form]);

  return (
    <section
      className="flex flex-1 flex-col pt-5 text-[14.5px]"
      aria-label={reviewTab ? "Review rounds" : "Issue findings"}
    >
      <div className="flex-1">
        {reviewTab ? (
          rounds.length ? (
            <ReviewRoundList rounds={rounds} findings={findings} onOpen={openRound} />
          ) : (
            <div className="flex min-h-120 flex-col items-center justify-center px-4 text-center">
              <Icon name="file-text" className="mb-5 size-10 text-[#8e93ef]" />
              <h3 className="text-[20px] font-semibold">No reviews yet</h3>
              <p className="mt-3 max-w-sm leading-relaxed text-[#97a4b7]">
                Keep each review and its findings in one place.
              </p>
              <p className="mt-4 text-[13px] text-[#8290a4]">
                Ask Claude or Codex to start one, or create one below.
              </p>
            </div>
          )
        ) : visible.length ? (
          <div>
            {visible.map((finding) => (
              <FindingRow key={finding.id} finding={finding} onOpen={() => openFinding(finding)} />
            ))}
          </div>
        ) : showResolved && resolved.length > 0 ? null : (
          <div className="flex min-h-120 flex-col items-center justify-center px-4 text-center">
            <Icon
              name={findings.length ? "circle-check" : "file-text"}
              className="mb-5 size-10 text-[#8e93ef]"
            />
            <h3 className="text-[20px] font-semibold">
              {findings.length ? "No findings need attention" : "No findings yet"}
            </h3>
            <p className="mt-3 max-w-sm leading-relaxed text-[#97a4b7]">
              {findings.length
                ? "You can still browse past findings and their discussions."
                : "Review findings will appear here. Add a finding or ask your connected agent to review this issue."}
            </p>
            {findings.length === 0 && (
              <button
                type="button"
                className="mt-5 inline-flex items-center gap-2 text-[13px] text-[#89a5ff] hover:underline"
                onClick={() => onSearchChange({ tab: "rounds" })}
              >
                View review rounds
                <Icon name="arrow-right" />
              </button>
            )}
          </div>
        )}
        {!reviewTab && resolved.length > 0 && (
          <div className="mt-5">
            <button
              type="button"
              aria-expanded={showResolved}
              className="inline-flex items-center gap-2 py-2 text-[13px] text-[#a9b8ca] hover:text-white"
              onClick={() =>
                onSearchChange({ ...search, findingStatus: showResolved ? undefined : "all" })
              }
            >
              <Icon name="chevron-down" className={showResolved ? "rotate-180" : ""} />
              {showResolved ? "Hide" : "Show"} {resolved.length} resolved{" "}
              {resolved.length === 1 ? "finding" : "findings"}
            </button>
            {showResolved && (
              <div aria-label="Resolved findings" className="mt-2">
                {resolved.map((finding) => (
                  <FindingRow
                    key={finding.id}
                    finding={finding}
                    onOpen={() => openFinding(finding)}
                  />
                ))}
              </div>
            )}
          </div>
        )}
      </div>
      {(!reviewTab || !activeRound) && (
        <div
          aria-label={reviewTab ? "Review actions" : "Finding actions"}
          className="sticky bottom-0 z-1 mt-6 flex justify-start bg-transparent py-4"
        >
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-lg border border-[#426fff66] bg-transparent px-3.5 py-2 text-[13px] font-medium text-[#89a5ff] transition-colors hover:border-[#7293ff] hover:text-[#b2c4ff] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#89a5ff]"
            onClick={() => setForm(reviewTab ? "start" : "add")}
          >
            <Icon name="plus" />
            {reviewTab ? "New round" : "Add finding"}
          </button>
        </div>
      )}
      {createPortal(
        <Modal
          variant="drawer"
          open={open}
          onClose={close}
          label={
            form
              ? `${form === "start" ? "Start review round" : form === "add" ? "Add finding" : "Finish review round"}`
              : selectedFinding
                ? `${selectedFinding.label} — ${selectedFinding.title}`
                : selectedRound
                  ? `Review round ${selectedRound.number}`
                  : "Review details"
          }
          className="flex h-dvh w-[min(100%,720px)] min-w-0 flex-col border-l border-[#303641] bg-[#101216] shadow-[-20px_0_60px_#0006]"
        >
          <div
            ref={content}
            tabIndex={-1}
            className="min-h-0 flex-1 overflow-y-auto px-8 py-7 text-[14.5px] outline-none max-[600px]:px-5"
          >
            <div className="mb-6 flex items-center justify-between gap-4">
              <nav
                aria-label="Review navigation"
                className="flex min-w-0 flex-wrap items-center gap-2 text-[13px] text-[#97a4b7]"
              >
                <button type="button" onClick={close} className="text-[#89a5ff] hover:underline">
                  {issue.key}
                </button>
                <span>/</span>
                {selectedFinding ? (
                  <>
                    <button
                      type="button"
                      onClick={() => {
                        const round = rounds.find((item) => item.number === selectedFinding.round);
                        if (round) openRound(round);
                      }}
                      className="text-[#89a5ff] hover:underline"
                    >
                      Round {selectedFinding.round}
                    </button>
                    <span>/ {selectedFinding.label}</span>
                  </>
                ) : form && selectedRound ? (
                  <button
                    type="button"
                    className="text-[#89a5ff] hover:underline"
                    onClick={() => setForm(null)}
                  >
                    Round {selectedRound.number}
                  </button>
                ) : (
                  <button
                    type="button"
                    className="text-[#89a5ff] hover:underline"
                    onClick={() => {
                      setForm(null);
                      onSearchChange({ tab: "rounds" });
                    }}
                  >
                    Review rounds
                  </button>
                )}
              </nav>
              <button
                type="button"
                aria-label="Close review drawer"
                className="grid size-8 shrink-0 place-items-center rounded-lg text-[#97a4b7] hover:bg-[#252a32] hover:text-white"
                onClick={close}
              >
                <Icon name="circle-x" />
              </button>
            </div>
            {form === "start" ? (
              activeRound ? (
                <div>
                  <p className="text-[#97a4b7]">
                    Round {activeRound.number} is already in progress. Finish it before starting
                    another.
                  </p>
                  <button
                    type="button"
                    className={`${reviewButton} mt-5`}
                    onClick={() => openRound(activeRound)}
                  >
                    Open round {activeRound.number}
                  </button>
                </div>
              ) : (
                <StartReviewForm issue={issue} onCreated={openRound} />
              )
            ) : form === "add" ? (
              addingRound ? (
                <AddFindingForm
                  key={addingRound.id}
                  issue={issue}
                  round={addingRound}
                  onCreated={openFinding}
                />
              ) : (
                <div>
                  <h2 className="text-[23px] font-semibold">Start a review first</h2>
                  <p className="mt-4 leading-relaxed text-[#97a4b7]">
                    Every finding belongs to a review round. Start a round to add your finding.
                  </p>
                  <button
                    type="button"
                    className={`${reviewButton} mt-6`}
                    onClick={() => setForm("start")}
                  >
                    Start review round
                  </button>
                </div>
              )
            ) : form === "finish" && selectedRound && !selectedRound.finished_at ? (
              <FinishReviewForm
                key={selectedRound.id}
                issue={issue}
                round={selectedRound}
                onFinished={() => setForm(null)}
              />
            ) : selectedFinding ? (
              <FindingDetail
                key={selectedFinding.id}
                finding={selectedFinding}
                variant="drawer"
                onOpenFinding={openFinding}
              />
            ) : selectedRound ? (
              <ReviewRoundDetail
                issue={issue}
                round={selectedRound}
                findings={findings.filter(
                  (finding) => finding.review_round_id === selectedRound.id,
                )}
                onFindingOpen={openFinding}
                onAddFinding={() => setForm("add")}
                onFinish={() => setForm("finish")}
              />
            ) : (
              open && (
                <p className="text-[#97a4b7]">
                  This finding or review round is no longer available.
                </p>
              )
            )}
          </div>
        </Modal>,
        document.body,
      )}
    </section>
  );
}
