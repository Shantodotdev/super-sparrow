import { matchesChord } from "../lib/shortcuts";
import { useKeyboardShortcuts } from "./keyboard-shortcuts-provider";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { formatRelative } from "../lib/format";
import { findingActions } from "../lib/review";
import type { Finding, FindingStatus } from "../lib/types";
import { findingsQuery, useChangeFindingStatus, type Decision } from "../queries/findings";
import { issuesQuery } from "../queries/issues";
import { useProjectSlugs } from "../hooks/use-project";
import { Discussion } from "./discussion";
import { FindingSeverity, FindingStatus as FindingStatusLabel } from "./finding-row";
import { Icon } from "./icon";
import { Input } from "./input";
import { reviewButton, reviewSecondaryButton } from "./review-forms";
import { ReviewPerson } from "./review-person";

const shortcuts = { findingFix: "fix", findingWontfix: "wontfix", findingLater: "later" } as const;
const actionLabel: Record<FindingStatus, string> = {
  open: "Withdraw decision",
  fix: "Fix",
  wontfix: "Won't fix",
  later: "Later",
  fixed: "Mark fixed",
  verified: "Verify fix",
};

/** Allows finding shortcuts only in the active surface, outside fields and other dialogs. */
function isShortcut(event: KeyboardEvent, scope: HTMLElement | null): boolean {
  if (!scope || event.defaultPrevented || event.repeat || event.isComposing) return false;
  const target = event.target instanceof HTMLElement ? event.target : null;
  if (
    target?.closest("input, textarea, select, [contenteditable], [role=combobox], [role=listbox]")
  )
    return false;
  const dialogs = Array.from(document.querySelectorAll('[role="dialog"][aria-modal="true"]'));
  return dialogs.every((dialog) => dialog.contains(scope));
}

/** Displays evidence, valid lifecycle actions, attribution, and discussion for one finding. */
export function FindingDetail({
  finding,
  variant = "panel",
  onOpenFinding,
}: {
  finding: Finding;
  variant?: "panel" | "drawer";
  onOpenFinding?: (finding: Finding) => void;
}) {
  const { bindings } = useKeyboardShortcuts();
  const { data: findings } = useSuspenseQuery(findingsQuery);
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const projectSlug = useProjectSlugs();
  const change = useChangeFindingStatus();
  const [askingReason, setAskingReason] = useState(false);
  const [reason, setReason] = useState("");
  const reasonInput = useRef<HTMLInputElement>(null);
  const surface = useRef<HTMLElement>(null);
  const original = findings.find((item) => item.id === finding.duplicate_of_finding_id);
  const followUp = issues.find((item) => item.id === finding.follow_up_issue_id);
  const actions = finding.duplicate_of_finding_id ? [] : findingActions(finding.status);
  const location =
    finding.file_path && `${finding.file_path}${finding.line ? `:${finding.line}` : ""}`;

  const decide = async (status: FindingStatus, explanation?: string) => {
    if (change.isPending) return;
    try {
      await change.mutateAsync({ finding, status, reason: explanation });
      setAskingReason(false);
      setReason("");
    } catch {
      // Keep the reason available when the server refuses the change.
    }
  };

  useEffect(() => {
    if (askingReason) reasonInput.current?.focus();
  }, [askingReason]);

  useEffect(() => {
    if (finding.status !== "open") setAskingReason(false);
  }, [finding.status]);

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      const id = (Object.keys(shortcuts) as (keyof typeof shortcuts)[]).find((id) =>
        matchesChord(event, bindings[id]),
      );
      const decision: Decision | undefined = id ? shortcuts[id] : undefined;
      if (
        !decision ||
        !actions.includes(decision) ||
        askingReason ||
        change.isPending ||
        !isShortcut(event, surface.current)
      )
        return;
      event.preventDefault();
      if (decision === "wontfix") setAskingReason(true);
      else void decide(decision);
    };
    window.addEventListener("keydown", handleShortcut);
    return () => window.removeEventListener("keydown", handleShortcut);
  });

  const confirmWontfix = (event: FormEvent) => {
    event.preventDefault();
    if (reason.trim()) void decide("wontfix", reason.trim());
  };

  return (
    <section
      ref={surface}
      aria-label="Finding details"
      className={
        variant === "drawer"
          ? "min-w-0 text-[14.5px]"
          : "min-w-0 border-l border-[#252a32] px-8 py-6 text-[14.5px] max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:px-4"
      }
    >
      <h2 className="text-[23px] leading-snug font-semibold wrap-anywhere text-[#f2f5fc]">
        {finding.title}
      </h2>
      <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-[13px]">
        <FindingSeverity severity={finding.severity} />
        <FindingStatusLabel status={finding.status} />
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-2 text-[13px] text-[#97a4b7]">
        Found by <ReviewPerson actorId={finding.found_by_actor_id} />
        <span>· {formatRelative(finding.created_at)}</span>
      </div>
      {variant === "panel" && (
        <p className="mt-2 text-[12px] text-[#8290a4]">
          {finding.issue_key} · Round {finding.round} · {finding.label}
        </p>
      )}
      {original && (
        <div className="mt-5 rounded-lg bg-[#ffffff05] p-3 text-[13px] text-[#a9b8ca]">
          Duplicate of{" "}
          {onOpenFinding ? (
            <button
              type="button"
              className="text-[#89a5ff] hover:underline"
              onClick={() => onOpenFinding(original)}
            >
              Round {original.round} · {original.label}
            </button>
          ) : (
            `Round ${original.round} · ${original.label}`
          )}
          . Decisions and fixes are tracked on the original finding.
        </div>
      )}
      <div className="mt-6 space-y-6 border-t border-[#252a32] pt-6">
        <section>
          <h3 className="mb-2 text-[13px] font-semibold">Problem</h3>
          <p className="leading-relaxed wrap-break-word whitespace-pre-wrap text-[#b7c3d4]">
            {finding.problem}
          </p>
        </section>
        {finding.fix && (
          <section>
            <h3 className="mb-2 text-[13px] font-semibold">Suggested fix</h3>
            <p className="leading-relaxed wrap-break-word whitespace-pre-wrap text-[#b7c3d4]">
              {finding.fix}
            </p>
          </section>
        )}
        {(location || finding.evidence) && (
          <section>
            <h3 className="mb-2 text-[13px] font-semibold">Evidence</h3>
            {location && (
              <div className="flex items-start gap-2 rounded-lg border border-[#2a3039] p-3 font-mono text-[12px] wrap-anywhere text-[#a7c0ff]">
                <Icon name="file-text" />
                {location}
              </div>
            )}
            {finding.evidence && (
              <p className="mt-3 leading-relaxed wrap-break-word whitespace-pre-wrap text-[#b7c3d4]">
                {finding.evidence}
              </p>
            )}
          </section>
        )}
      </div>
      <section className="mt-6 border-t border-[#252a32] pt-5" aria-label="Finding decision">
        <h3 className="mb-3 text-[13px] font-semibold">
          {finding.status === "open" ? "Decision" : "Progress"}
        </h3>
        {finding.decided_by_actor_id && (
          <div className="mb-3 text-[13px] text-[#97a4b7]">
            Decision by <ReviewPerson actorId={finding.decided_by_actor_id} />
            {finding.decided_at && <span> · {formatRelative(finding.decided_at)}</span>}
            {finding.decision_reason && (
              <p className="mt-2 wrap-break-word whitespace-pre-wrap">{finding.decision_reason}</p>
            )}
          </div>
        )}
        {finding.fixed_by_actor_id && (
          <div className="mb-2 text-[13px] text-[#97a4b7]">
            Fixed by <ReviewPerson actorId={finding.fixed_by_actor_id} />
          </div>
        )}
        {finding.verified_by_actor_id && (
          <div className="mb-2 text-[13px] text-[#97a4b7]">
            Verified by <ReviewPerson actorId={finding.verified_by_actor_id} />
          </div>
        )}
        {followUp && (
          <Link
            className="mb-4 block text-[13px] text-[#89a5ff] hover:underline"
            to="/$projectSlug/issues/$issue"
            params={{ projectSlug: projectSlug(followUp.project_id), issue: followUp.key }}
          >
            Follow-up: {followUp.key} · {followUp.title}
          </Link>
        )}
        {!askingReason && actions.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {actions.map((status, index) => (
              <button
                key={status}
                type="button"
                disabled={change.isPending}
                className={index === 0 ? reviewButton : reviewSecondaryButton}
                onClick={() => (status === "wontfix" ? setAskingReason(true) : void decide(status))}
              >
                {finding.status === "fixed" && status === "fix"
                  ? "Needs more work"
                  : actionLabel[status]}
                {finding.status === "open" && (
                  <kbd className="ml-2 rounded bg-[#ffffff0c] px-1.5 py-0.5 text-[12px] text-inherit">
                    {status === "fix" ? "F" : status === "wontfix" ? "W" : "L"}
                  </kbd>
                )}
              </button>
            ))}
          </div>
        )}
        {askingReason && (
          <form
            onSubmit={confirmWontfix}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                event.stopPropagation();
                setAskingReason(false);
              }
            }}
            className="space-y-3"
          >
            <label className="block text-[13px] text-[#a9b8ca]">
              Why won't this be fixed?
              <Input
                ref={reasonInput}
                aria-label="Decision reason"
                required
                maxLength={100000}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                className="mt-2 w-full rounded-lg px-3 py-2.5"
              />
            </label>
            <div className="flex gap-2">
              <button
                disabled={change.isPending || !reason.trim()}
                className={reviewButton}
                type="submit"
              >
                Confirm won't fix
              </button>
              <button
                type="button"
                disabled={change.isPending}
                className={reviewSecondaryButton}
                onClick={() => setAskingReason(false)}
              >
                Cancel
              </button>
            </div>
          </form>
        )}
        {change.isPending && (
          <p role="status" className="mt-3 text-[13px] text-[#97a4b7]">
            Saving…
          </p>
        )}
        {change.error && (
          <p role="alert" className="mt-3 text-[13px] text-[#f2a9ae]">
            {change.error.message}
          </p>
        )}
      </section>
      <section className="mt-6 border-t border-[#252a32] pt-5">
        <h3 className="mb-4 text-[13px] font-semibold">
          Discussion{" "}
          <span className="ml-2 font-normal text-[#8290a4]">{finding.comment_count}</span>
        </h3>
        <Discussion
          key={finding.id}
          issueId={finding.issue_id}
          findingId={finding.id}
          variant="review"
        />
      </section>
    </section>
  );
}
