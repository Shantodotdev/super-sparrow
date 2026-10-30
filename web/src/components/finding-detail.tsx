import { Input } from "./input";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { findingStatusLabel } from "../lib/finding-status";
import { formatDate } from "../lib/format";
import type { Finding } from "../lib/types";
import { findingsQuery, useDecideFinding, type Decision } from "../queries/findings";
import { useActor } from "../queries/project";
import { Discussion } from "./discussion";
import { Icon } from "./icon";
import { Person } from "./person";
import { SeverityBadge } from "./severity-badge";

const shortcuts: Record<string, Decision> = { f: "fix", w: "wontfix", l: "later" };

/**
 * Reports whether a key press is meant for the page's shortcuts: not typed into a field, not
 * combined with a modifier, not a held-down repeat, and not made while a dialog is open over the
 * page, such as the command palette or the New issue window.
 */
function isShortcut(event: KeyboardEvent): boolean {
  if (event.defaultPrevented || event.repeat || event.metaKey || event.ctrlKey || event.altKey) {
    return false;
  }

  const target = event.target instanceof HTMLElement ? event.target : null;
  if (
    target?.closest("input, textarea, select, [contenteditable], [role=combobox], [role=dialog]")
  ) {
    return false;
  }
  return !document.querySelector("[role=dialog][aria-modal=true]");
}

/**
 * Shows one finding with its evidence, discussion, and decision controls. Pressing F or L outside
 * a text field decides Fix or Later; W asks for the reason a won't-fix needs, with the cursor in
 * its box.
 */
export function FindingDetail({ finding }: { finding: Finding }) {
  const { data: findings } = useSuspenseQuery(findingsQuery);
  const actor = useActor();
  const decide = useDecideFinding();
  const [askingReason, setAskingReason] = useState(false);
  const [reason, setReason] = useState("");
  const reasonInput = useRef<HTMLInputElement>(null);

  const original = findings.find((item) => item.id === finding.duplicate_of_finding_id);
  const location =
    finding.file_path && `${finding.file_path}${finding.line ? `:${finding.line}` : ""}`;

  useEffect(() => {
    if (askingReason) reasonInput.current?.focus();
  }, [askingReason]);

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      const decision = shortcuts[event.key.toLowerCase()];
      if (!decision || askingReason || !isShortcut(event)) return;

      event.preventDefault();
      if (decision === "wontfix") setAskingReason(true);
      else decide(finding, decision);
    };

    window.addEventListener("keydown", handleShortcut);

    return () => window.removeEventListener("keydown", handleShortcut);
  }, [finding, decide, askingReason]);

  const cancelReason = () => {
    setAskingReason(false);
    setReason("");
  };

  const confirmWontfix = (event: FormEvent) => {
    event.preventDefault();
    if (!reason.trim()) return;

    decide(finding, "wontfix", reason.trim());
    cancelReason();
  };

  return (
    <aside className="min-w-0 border-l border-l-[#252a32] px-8 py-[22px] max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:border-t-[#252a32] max-[700px]:px-[16px] max-[700px]:py-[20px] [&_>_p:not(.detail-author)]:leading-normal [&_>_p:not(.detail-author)]:text-[#b2bfd0] [&_h3]:mx-0 [&_h3]:mt-[24px] [&_h3]:mb-[9px] [&_h3]:text-[14px] [&_h3]:font-semibold">
      <div className="flex flex-wrap items-center gap-[10px] [&_strong]:text-[16px]">
        <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
          {finding.label}
        </span>
        <strong>{finding.title}</strong>
        <SeverityBadge value={finding.severity} />
        <span className="ml-auto inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
          ● {findingStatusLabel[finding.status]}
        </span>
      </div>
      <p className="detail-author mt-[18px] flex flex-wrap items-center gap-[6px] text-[#aebbd0]">
        Found by <Person actorId={finding.found_by_actor_id} />{" "}
        <span className="muted text-[#a2aec2]">
          {" "}
          · &nbsp; {finding.issue_key} round {finding.round} · {formatDate(finding.created_at)}
        </span>
      </p>
      {original && (
        <p className="muted text-[#a2aec2]">
          Duplicate of round {original.round} {original.label}, “{original.title}”. It is decided
          and fixed through the original.
        </p>
      )}
      <h3>Problem</h3>
      <p className="whitespace-pre-wrap">{finding.problem}</p>
      {finding.fix && (
        <>
          <h3>Suggested fix</h3>
          <p className="whitespace-pre-wrap">{finding.fix}</p>
        </>
      )}
      {(location || finding.evidence) && (
        <>
          <h3>Evidence</h3>
          <div className="rounded-[6px] border border-[#303640] p-[10px] text-[13px] text-[#b0c8f5]">
            {location && (
              <div>
                <Icon name="file-text" /> &nbsp; {location}
              </div>
            )}
            {finding.evidence && <p className="mt-[6px] whitespace-pre-wrap">{finding.evidence}</p>}
          </div>
        </>
      )}
      <h3>Discussion</h3>
      <Discussion issueId={finding.issue_id} findingId={finding.id} />
      <h3>Decision</h3>
      {finding.decided_at && (
        <p className="muted mb-[10px] text-[#a2aec2]">
          {findingStatusLabel[finding.status]} · decided by{" "}
          {actor(finding.decided_by_actor_id)?.display_name ?? "Unknown"} on{" "}
          {formatDate(finding.decided_at)}
          {finding.decision_reason && `: ${finding.decision_reason}`}
        </p>
      )}
      <div className="flex gap-[10px] [&_button]:flex-1 [&_button]:rounded-full [&_button]:border [&_button]:border-[#343b49] [&_button]:bg-[#141820] [&_button]:p-[10px] [&_button:first-child]:border-[#5278ff] [&_button:first-child]:bg-[#19284e] [&_kbd]:float-right">
        <button onClick={() => decide(finding, "fix")}>
          <Icon name="check" /> &nbsp; Fix <kbd>F</kbd>
        </button>
        <button onClick={() => setAskingReason(true)}>
          <Icon name="circle-slash" /> &nbsp; Won't fix <kbd>W</kbd>
        </button>
        <button onClick={() => decide(finding, "later")}>
          <Icon name="clock" /> &nbsp; Later <kbd>L</kbd>
        </button>
      </div>
      {askingReason && (
        <form
          onSubmit={confirmWontfix}
          onKeyDown={(event) => {
            if (event.key === "Escape") cancelReason();
          }}
        >
          <label>
            Reason
            <Input
              ref={reasonInput}
              required
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </label>
          <button className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]">
            Confirm won't fix
          </button>{" "}
          <button
            type="button"
            className="text-button border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
            onClick={cancelReason}
          >
            Cancel
          </button>
        </form>
      )}
    </aside>
  );
}
