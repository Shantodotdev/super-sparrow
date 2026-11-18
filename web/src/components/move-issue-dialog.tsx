import { useId, useRef } from "react";
import { createPortal } from "react-dom";
import type { Issue } from "../lib/types";
import type { Project } from "../queries/resources";
import { Modal } from "./modal";

type MoveIssueDialogProps = {
  issues: Issue[];
  target: Project | undefined;
  pending: boolean;
  error: string;
  onConfirm: () => void;
  onClose: () => void;
};

/** Asks for confirmation before an issue moves to another project, saying what moves with it. */
export function MoveIssueDialog({
  issues,
  target,
  pending,
  error,
  onConfirm,
  onClose,
}: MoveIssueDialogProps) {
  const headingId = useId();
  // The dialog fades out after target clears, so it keeps the last project to show meanwhile.
  const lastTarget = useRef(target);
  if (target) lastTarget.current = target;
  const name = lastTarget.current?.name;
  const lastIssues = useRef(issues);
  if (target) lastIssues.current = issues;
  const moving = lastIssues.current;
  const many = moving.length > 1;
  const what = moving.length === 1 ? moving[0]?.key : `${moving.length} issues`;

  return createPortal(
    <Modal
      open={!!target}
      onClose={() => {
        if (!pending) onClose();
      }}
      className="w-[min(100%,520px)] rounded-[16px] border border-[#343740] bg-[#191a1e] p-5 shadow-[0_24px_80px_#000b]"
      overlayClassName="items-start overflow-y-auto pt-[min(20vh,_160px)] max-[600px]:px-[12px]"
      labelledBy={headingId}
    >
      <h2 id={headingId} className="m-0 text-[18px] font-medium text-[#e7e8ec]">
        Move {what} to {name}?
      </h2>
      <p className="mt-3 mb-0 text-[15px] leading-[1.55] text-[#a9b8ca]">
        {many ? "Each issue gets a new key" : "The issue gets a new key"} in {name}, and{" "}
        {many ? "the old keys stop" : "the old key stops"} working. Subissues, reviews, comments,
        and attachments move with {many ? "them" : "it"}.
      </p>
      <ul className="mt-3 mb-0 list-disc space-y-1 pl-5 text-[15px] leading-normal text-[#a9b8ca]">
        <li>Labels move by name; missing ones are created in {name}.</li>
        <li>The milestone moves if {name} has one with the same name, otherwise it is cleared.</li>
        <li>Parent issue links and child issues are unlinked.</li>
      </ul>
      {error && (
        <p role="alert" className="mt-3 mb-0 text-[15px] text-[#ff9999]">
          {error}
        </p>
      )}
      <div className="mt-5 flex justify-end gap-2">
        <button
          type="button"
          className="rounded-lg border border-[#343740] bg-transparent px-4 py-2.5 text-[14px] text-[#c9d3e1] hover:bg-white/5"
          disabled={pending}
          onClick={onClose}
        >
          Cancel
        </button>
        <button
          type="button"
          autoFocus
          className="rounded-lg border-0 bg-[#426fff] px-4 py-2.5 text-[14px] font-medium text-white hover:bg-[#5580ff] disabled:cursor-default disabled:opacity-60"
          disabled={pending}
          onClick={onConfirm}
        >
          {moving.length === 1 ? "Move issue" : `Move ${moving.length} issues`}
        </button>
      </div>
    </Modal>,
    document.body,
  );
}
