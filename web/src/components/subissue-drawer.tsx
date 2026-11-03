import { useState } from "react";
import { createPortal } from "react-dom";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { Issue, IssueStatus, Subissue } from "../lib/types";
import { useSaveSubissue } from "../queries/subissues";
import { Icon } from "./icon";
import { Input } from "./input";
import { LabelChips } from "./label-chips";
import { LabelSelector } from "./label-selector";
import { Modal } from "./modal";
import { Person } from "./person";
import { Select } from "./select";
import { SubissueDescription } from "./subissue-description";
import { useToast } from "./toast";

type SubissueDrawerProps = {
  issue: Issue;
  subissue?: Subissue;
  open: boolean;
  onClose: () => void;
};

/** Opens a lightweight subissue in a full-height panel with the main issue's inline editing behavior. */
export function SubissueDrawer({ issue, subissue, open, onClose }: SubissueDrawerProps) {
  const close = () => {
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
    onClose();
  };

  return createPortal(
    <Modal
      variant="drawer"
      open={open}
      onClose={close}
      label={subissue ? `${subissue.key} — ${subissue.title}` : "Subissue"}
      className="flex h-dvh w-[min(100%,720px)] min-w-0 flex-col border-l border-[#303641] bg-[#101216] shadow-[-20px_0_60px_#0006]"
    >
      {subissue && (
        <SubissueDetail key={subissue.id} issue={issue} subissue={subissue} onClose={close} />
      )}
    </Modal>,
    document.body,
  );
}

/** Saves the title on blur and status immediately, without sending unrelated fields. */
function SubissueDetail({
  issue,
  subissue,
  onClose,
}: {
  issue: Issue;
  subissue: Subissue;
  onClose: () => void;
}) {
  const draftKey = `sparrow:subissue-title-draft:${subissue.id}`;
  const [title, setTitle] = useState<string | null>(() => {
    try {
      return sessionStorage.getItem(draftKey);
    } catch {
      return null;
    }
  });
  const save = useSaveSubissue(issue.key);
  const { notify } = useToast();

  const updateTitle = (value: string) => {
    setTitle(value);
    try {
      sessionStorage.setItem(draftKey, value);
    } catch {
      /* Editing remains available when browser storage is unavailable. */
    }
  };

  const clearTitleDraft = (saved: string | null) => {
    try {
      if (sessionStorage.getItem(draftKey) === saved) sessionStorage.removeItem(draftKey);
    } catch {
      /* A storage failure must not turn a committed save into an error. */
    }
  };

  const saveStatus = async (status: IssueStatus) => {
    try {
      await save.mutateAsync({ id: subissue.id, status });
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not save the subissue status");
    }
  };

  const saveTitle = async () => {
    if (title === null || title === subissue.title) {
      clearTitleDraft(title);
      setTitle(null);
      return;
    }
    try {
      await save.mutateAsync({ id: subissue.id, title });
      clearTitleDraft(title);
      setTitle(null);
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not save the subissue title");
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-8 py-7 max-[600px]:px-5">
      <div className="flex shrink-0 items-start gap-3">
        <h1 className="min-w-0 flex-1">
          <Input
            autoFocus
            aria-label="Subissue title"
            placeholder="Subissue title"
            maxLength={200}
            className="h-auto w-full rounded-none border-0 bg-transparent p-0 text-[28px] font-semibold text-[#f2f5fc]"
            value={title ?? subissue.title}
            disabled={save.isPending}
            onChange={(event) => updateTitle(event.target.value)}
            onBlur={() => void saveTitle()}
            onKeyDown={(event) => {
              if (event.key === "Enter") event.currentTarget.blur();
            }}
          />
        </h1>
        <button
          type="button"
          aria-label="Close"
          className="grid size-8 shrink-0 place-items-center rounded-lg border-0 bg-transparent p-0 text-[#9a9da6] hover:bg-[#303238] hover:text-white"
          onClick={onClose}
        >
          <Icon name="circle-x" />
        </button>
      </div>
      <div className="mt-4 flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 text-[#a9b8ca]">
        <span className="inline-flex items-center gap-2">
          <Icon name={statusIcon[subissue.status]} className={statusIconColor[subissue.status]} />
          {subissue.key}
        </span>
        <Select
          variant="chip"
          label="Subissue status"
          value={subissue.status}
          onChange={(value) => void saveStatus(value as IssueStatus)}
          options={issueStatuses.map((status) => ({
            value: status,
            label: issueStatusLabel[status],
            icon: statusIcon[status],
            iconClassName: statusIconColor[status],
          }))}
        />
      </div>
      <div className="mt-5 grid shrink-0 grid-cols-[96px_minmax(0,1fr)] items-center gap-x-4 gap-y-3 text-[13px] text-[#8fa0b7]">
        <span>Created by</span>
        <span className="text-[#c9d3e1]">
          <Person actorId={subissue.created_by_actor_id} />
        </span>
        <span className="self-start pt-[9px]">Labels</span>
        <div className="flex min-w-0 flex-col gap-2 [&_button[aria-expanded]]:min-h-[34px] [&_button[aria-expanded]]:text-[13px]">
          {subissue.label_ids.length > 0 && <LabelChips labelIds={subissue.label_ids} />}
          <LabelSelector
            projectId={issue.project_id}
            selected={subissue.label_ids}
            onChange={(labelIds) => save.mutateAsync({ id: subissue.id, label_ids: labelIds })}
          />
        </div>
      </div>
      {save.error && (
        <p role="alert" className="mt-3 text-sm text-[#f2a9ae]">
          {save.error.message}
        </p>
      )}
      <SubissueDescription issue={issue} subissue={subissue} />
    </div>
  );
}
