import { useEffect } from "react";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { IssueStatus } from "../lib/types";
import { Icon } from "./icon";

type IssueSelectionBarProps = {
  count: number;
  onMove: (status: IssueStatus) => void;
  onClear: () => void;
};

/** A floating bar over the issue list that sets the status of every selected issue at once. */
export function IssueSelectionBar({ count, onMove, onClear }: IssueSelectionBarProps) {
  // Escape leaves selection mode, unless a dialog or menu has claimed the key.
  useEffect(() => {
    const clear = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      if (document.querySelector('[role="dialog"]:not([hidden]), [role="menu"]')) return;
      onClear();
    };
    window.addEventListener("keydown", clear);
    return () => window.removeEventListener("keydown", clear);
  }, [onClear]);

  return (
    <div
      role="toolbar"
      aria-label="Selected issues"
      className="fixed bottom-6 left-1/2 z-30 flex max-w-[calc(100vw-32px)] -translate-x-1/2 items-center gap-2 overflow-x-auto rounded-xl border border-[#303744] bg-[#171a20] px-3 py-2 shadow-[0_12px_32px_rgba(0,0,0,0.5)]"
    >
      <span className="pr-1 text-[13px] font-medium whitespace-nowrap text-[#dce2ec]">
        {count} selected
      </span>
      <span aria-hidden="true" className="h-5 w-px shrink-0 bg-[#303744]" />
      {issueStatuses.map((status) => (
        <button
          key={status}
          type="button"
          title={`Move to ${issueStatusLabel[status]}`}
          className="flex items-center gap-1.5 rounded-md border-0 bg-transparent px-2 py-1 text-[13px] whitespace-nowrap text-[#bfcbdc] hover:bg-white/10 hover:text-white"
          onClick={() => onMove(status)}
        >
          <Icon name={statusIcon[status]} className={statusIconColor[status]} />
          {issueStatusLabel[status]}
        </button>
      ))}
      <span aria-hidden="true" className="h-5 w-px shrink-0 bg-[#303744]" />
      <button
        type="button"
        aria-label="Clear selection"
        title="Clear selection (Esc)"
        className="grid size-7 shrink-0 place-items-center rounded-md border-0 bg-transparent text-[#9ba9bc] hover:bg-white/10 hover:text-white"
        onClick={onClear}
      >
        <Icon name="x" />
      </button>
    </div>
  );
}
