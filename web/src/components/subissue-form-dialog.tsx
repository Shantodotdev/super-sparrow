import { useId, type FormEvent, type ReactNode, type Ref } from "react";
import { createPortal } from "react-dom";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { IssueStatus } from "../lib/types";
import { Icon } from "./icon";
import { Input } from "./input";
import { MarkdownEditor } from "./markdown-editor";
import { Modal } from "./modal";
import { Select } from "./select";

type SubissueFormDialogProps = {
  open: boolean;
  onClose: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  heading: string;
  context: ReactNode;
  title: string;
  onTitleChange: (title: string) => void;
  titleLabel: string;
  titleRef?: Ref<HTMLInputElement>;
  description: string;
  onDescriptionChange: (description: string) => void;
  descriptionLabel: string;
  descriptionPlaceholder: string;
  status: IssueStatus;
  onStatusChange: (status: IssueStatus) => void;
  statusLabel: string;
  error?: string;
  pending: boolean;
  submitDisabled: boolean;
  submitLabel: string;
  createMore?: { checked: boolean; onChange: (checked: boolean) => void };
};

/** Uses the existing New Issue form layout for subissues without changing the issue form. */
export function SubissueFormDialog({
  open,
  onClose,
  onSubmit,
  heading,
  context,
  title,
  onTitleChange,
  titleLabel,
  titleRef,
  description,
  onDescriptionChange,
  descriptionLabel,
  descriptionPlaceholder,
  status,
  onStatusChange,
  statusLabel,
  error,
  pending,
  submitDisabled,
  submitLabel,
  createMore,
}: SubissueFormDialogProps) {
  const headingId = useId();

  return createPortal(
    <Modal
      open={open}
      onClose={() => {
        if (!pending) onClose();
      }}
      className="w-[min(100%,780px)] rounded-[16px] border border-[#343740] bg-[#191a1e] shadow-[0_24px_80px_#000b] [&_form]:px-[18px] [&_form]:py-4 max-[600px]:[&_form]:p-4"
      overlayClassName="items-start overflow-y-auto pt-[min(15vh,_130px)] max-[600px]:py-[20px] max-[600px]:px-[12px]"
      labelledBy={headingId}
    >
      <form onSubmit={onSubmit}>
        {error && (
          <p role="alert" className="mb-3 text-sm text-[#ff9999]">
            {error}
          </p>
        )}
        <div className="flex items-center gap-[8px] text-[#a5a8b1] [&_>_.icon]:h-4 [&_>_.icon]:w-4 [&_h2]:m-0 [&_h2]:text-[14px] [&_h2]:font-medium [&_h2]:text-[#e7e8ec]">
          <span className="inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] [&_.icon]:h-4 [&_.icon]:w-4">
            {context}
          </span>
          <Icon name="chevron-right" />
          <h2 id={headingId}>{heading}</h2>
          <button
            type="button"
            className="ml-auto grid place-items-center rounded-lg border-0 bg-transparent p-[6px] text-[#9a9da6] hover:bg-[#303238] hover:text-white"
            aria-label="Close"
            disabled={pending}
            onClick={onClose}
          >
            <Icon name="circle-x" />
          </button>
        </div>
        <div inert={pending}>
          <Input
            ref={titleRef}
            autoFocus
            required
            maxLength={200}
            aria-label={titleLabel}
            className="mt-[26px] block h-auto w-full rounded-none border-0 border-b border-[#3a3d46] bg-transparent p-0 text-[19px] font-semibold text-[#f1f2f4] placeholder:text-[#777a84]"
            value={title}
            onChange={(event) => onTitleChange(event.target.value)}
            placeholder={titleLabel}
            disabled={pending}
          />
          <MarkdownEditor
            label={descriptionLabel}
            placeholder={descriptionPlaceholder}
            value={description}
            onChange={onDescriptionChange}
            className="mt-4 [&_.tiptap]:max-h-[38dvh]! [&_.tiptap]:min-h-[120px]! [&_.tiptap]:overflow-y-auto! [&_.tiptap]:text-[#f1f2f4]!"
          />
          <div className="flex flex-wrap gap-[6px] [&_button]:min-h-10 [&_button]:px-[11px] [&_button]:py-[6px] [&_button]:text-[13px] **:[[role=listbox]]:max-h-(--subissue-menu-height)">
            <Select
              variant="chip"
              label={statusLabel}
              value={status}
              onChange={(value) => onStatusChange(value as IssueStatus)}
              options={issueStatuses.map((value) => ({
                value,
                label: issueStatusLabel[value],
                icon: statusIcon[value],
                iconClassName: statusIconColor[value],
              }))}
            />
          </div>
        </div>
        <div className="mt-[22px] flex items-center justify-end gap-4">
          {createMore && (
            <label className="m-0 flex cursor-pointer items-center gap-[8px] text-[12px] font-normal text-[#a4a6ae] focus-within:text-[#d2d9e6]">
              <input
                type="checkbox"
                role="switch"
                className="peer sr-only"
                checked={createMore.checked}
                onChange={(event) => createMore.onChange(event.target.checked)}
                disabled={pending}
              />
              <span
                aria-hidden="true"
                className={`relative inline-flex h-[18px] w-[32px] shrink-0 rounded-full transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-[#4f73f0]/60 ${createMore.checked ? "bg-[#2455eb]" : "bg-[#3a404b]"}`}
              >
                <span
                  className={`absolute top-[2px] left-[2px] size-4 rounded-full bg-white shadow transition-transform ${createMore.checked ? "translate-x-4" : ""}`}
                />
              </span>
              <span>Create more</span>
            </label>
          )}
          <button
            type="submit"
            className="rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]"
            disabled={pending || submitDisabled}
          >
            {submitLabel}
          </button>
        </div>
      </form>
    </Modal>,
    document.body,
  );
}
