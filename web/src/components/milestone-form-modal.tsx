import { useEffect, useId, useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import type { Milestone } from "../lib/types";
import { useSaveMilestone } from "../queries/milestones";
import { Icon } from "./icon";
import { Input } from "./input";
import { Modal } from "./modal";

/** Creates or edits a milestone, retaining the form and server error when a save fails. */
export function MilestoneFormModal({
  projectKey,
  formKey,
  milestone,
  open,
  onClose,
}: {
  projectKey: string;
  formKey: number;
  milestone?: Milestone;
  open: boolean;
  onClose: () => void;
}) {
  const headingId = useId();
  const save = useSaveMilestone(projectKey);
  const close = () => {
    if (!save.isPending) onClose();
  };
  const { reset } = save;
  useEffect(() => reset(), [formKey, reset]);

  return createPortal(
    <Modal
      open={open}
      onClose={close}
      labelledBy={headingId}
      className="w-[min(100%,520px)] rounded-2xl border border-[#303641] bg-[#15171b] p-6 shadow-[0_24px_80px_#0009]"
    >
      <div className="mb-6 flex items-center justify-between gap-4">
        <h2 id={headingId} className="flex items-center gap-3 text-lg font-semibold">
          <Icon name="target" className="text-[#9ba9be]" />
          {milestone ? "Edit milestone" : "New milestone"}
        </h2>
        <button
          type="button"
          aria-label="Close"
          disabled={save.isPending}
          onClick={close}
          className="rounded-lg border-0 bg-transparent p-2 text-[#9ba9be] hover:bg-[#262b33] hover:text-white"
        >
          <Icon name="circle-x" />
        </button>
      </div>
      <MilestoneForm key={formKey} milestone={milestone} save={save} onClose={onClose} />
    </Modal>,
    document.body,
  );
}

/** Resets the draft for each opening without remounting the dialog that restores focus. */
function MilestoneForm({
  milestone,
  save,
  onClose,
}: {
  milestone?: Milestone;
  save: ReturnType<typeof useSaveMilestone>;
  onClose: () => void;
}) {
  const [name, setName] = useState(milestone?.name ?? "");
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (save.isPending || !name.trim()) return;
    save.mutate({ id: milestone?.id, name: name.trim() }, { onSuccess: onClose });
  };

  return (
    <form onSubmit={submit} className="grid gap-5">
      <label className="m-0 grid gap-2 text-sm text-[#cbd4e4]">
        Milestone name
        <Input
          autoFocus
          required
          maxLength={100}
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="e.g. Phase 01 · Foundation"
          disabled={save.isPending}
        />
      </label>
      {save.error && (
        <p role="alert" className="text-sm text-[#ff9999]">
          {save.error.message}
        </p>
      )}
      <div className="mt-2 flex justify-end gap-3">
        <button
          type="button"
          disabled={save.isPending}
          onClick={onClose}
          className="rounded-full border border-[#303641] bg-transparent px-4 py-2 text-[#b8c2d2] hover:bg-[#222730]"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={save.isPending || !name.trim()}
          className="rounded-full border border-[#5f7fff] bg-[#2455eb] px-4 py-2 font-medium text-white hover:bg-[#3566f6] disabled:opacity-50"
        >
          {save.isPending ? "Saving…" : milestone ? "Save changes" : "Create milestone"}
        </button>
      </div>
    </form>
  );
}
