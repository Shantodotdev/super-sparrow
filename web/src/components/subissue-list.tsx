import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { issueStatusLabel, statusIcon, statusIconColor } from "../lib/issue-status";
import type { Issue, IssueStatus, Subissue } from "../lib/types";
import { subissuesQuery, useSaveSubissue, type SubissueChanges } from "../queries/subissues";
import { Icon } from "./icon";
import { SubissueFormDialog } from "./subissue-form-dialog";

/** Shows an issue's lightweight children with the same edit dialog in the body and sidebar. */
export function SubissueList({ issue, compact = false }: { issue: Issue; compact?: boolean }) {
  const { data: subissues } = useSuspenseQuery(subissuesQuery(issue.key));
  const [editing, setEditing] = useState<Subissue | "new" | null>(null);
  const [open, setOpen] = useState(false);
  const completed = subissues.filter((item) => item.status === "done").length;

  return (
    <section
      className={compact ? "" : "mt-8 mb-8 w-full border-t border-[#252b34] pt-[22px]"}
      aria-label={compact ? "Subissues sidebar" : "Subissues"}
    >
      <div className="mb-[10px] flex items-center justify-between gap-3">
        <div className="flex items-center gap-[10px]">
          <strong className="text-[14px] font-semibold">Subissues</strong>
          <span className="text-[12px] text-[#8fa0b7]">
            {completed}/{subissues.length}
          </span>
        </div>
        <button
          type="button"
          aria-label="Add subissue"
          title="Add subissue"
          onClick={() => {
            setEditing("new");
            setOpen(true);
          }}
          className="grid size-7 place-items-center rounded-[6px] border-0 bg-transparent p-0 text-[#9a9da6] transition-colors hover:bg-[#303238] hover:text-white"
        >
          <Icon name="plus" className="size-4" />
        </button>
      </div>
      {subissues.length === 0 && (
        <p className="muted mt-[3px] text-[12px] text-[#a2aec2]">No subissues yet.</p>
      )}
      <ul className="m-0 list-none p-0">
        {subissues.map((subissue) => (
          <li key={subissue.id}>
            <button
              type="button"
              aria-label={`${subissue.key}: ${subissue.title}, ${issueStatusLabel[subissue.status]}`}
              className={
                compact
                  ? "my-[6px] flex w-full items-center gap-[12px] rounded-[6px] border border-[#272d35] bg-[#111419] p-[10px] text-left transition-colors hover:border-[#3a424f] hover:bg-[#18191a]"
                  : "grid min-h-[45px] w-full grid-cols-[20px_max-content_minmax(0,1fr)] items-center gap-[10px] rounded-[10px] border-0 bg-transparent px-[12px] py-[6px] text-left transition-colors hover:bg-[#18191a]"
              }
              onClick={() => {
                setEditing(subissue);
                setOpen(true);
              }}
            >
              <Icon
                name={statusIcon[subissue.status]}
                className={`size-4 shrink-0 ${statusIconColor[subissue.status]}`}
              />
              {compact ? (
                <>
                  <span className="shrink-0 text-[12px] text-[#8fa0b7]">{subissue.key}</span>
                  <span className="min-w-0 flex-1 truncate text-[13px] font-medium text-[#eef0f5]">
                    {subissue.title}
                  </span>
                </>
              ) : (
                <>
                  <span className="text-[13px] text-[#aebed4]">{subissue.key}</span>
                  <span className="min-w-0 truncate text-[14px] text-[#eef0f5]">
                    {subissue.title}
                  </span>
                </>
              )}
            </button>
          </li>
        ))}
      </ul>
      <SubissueEditor
        issue={issue}
        subissue={editing === "new" || editing === null ? undefined : editing}
        open={open}
        onClose={() => setOpen(false)}
      />
    </section>
  );
}

/** Edits a draft explicitly, preserving it on errors including concurrent description changes. */
function SubissueEditor({
  issue,
  subissue,
  open,
  onClose,
}: {
  issue: Issue;
  subissue?: Subissue;
  open: boolean;
  onClose: () => void;
}) {
  const titleInput = useRef<HTMLInputElement>(null);
  const [createMore, setCreateMore] = useState(false);
  const [title, setTitle] = useState(subissue?.title ?? "");
  const [description, setDescription] = useState(subissue?.description ?? "");
  const [status, setStatus] = useState<IssueStatus>(subissue?.status ?? "backlog");
  const save = useSaveSubissue(issue.key);
  useEffect(() => {
    if (!open) return;
    setTitle(subissue?.title ?? "");
    setDescription(subissue?.description ?? "");
    setStatus(subissue?.status ?? "backlog");
    setCreateMore(false);
    save.reset();
  }, [open, subissue, save.reset]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (save.isPending) return;
    const changes: SubissueChanges = subissue
      ? { id: subissue.id }
      : { title, description, status };
    if (subissue) {
      if (title !== subissue.title) changes.title = title;
      if (status !== subissue.status) changes.status = status;
      if (description !== subissue.description) {
        changes.description = description;
        changes.description_revision = subissue.description_revision;
      }
      if (
        changes.title === undefined &&
        changes.status === undefined &&
        changes.description === undefined
      ) {
        onClose();
        return;
      }
    }

    try {
      await save.mutateAsync(changes);
      if (!subissue && createMore) {
        setTitle("");
        setDescription("");
        setStatus("backlog");
        titleInput.current?.focus();
      } else {
        onClose();
      }
    } catch {
      // The mutation displays the error and keeps the draft for recovery.
    }
  };

  return (
    <SubissueFormDialog
      open={open}
      onClose={onClose}
      onSubmit={submit}
      heading={subissue ? subissue.key : "New subissue"}
      context={
        <>
          <Icon name={statusIcon[issue.status]} className={statusIconColor[issue.status]} />{" "}
          {issue.key}
        </>
      }
      title={title}
      onTitleChange={setTitle}
      titleLabel="Subissue title"
      titleRef={titleInput}
      description={description}
      onDescriptionChange={setDescription}
      descriptionLabel="Subissue description"
      descriptionPlaceholder="Add a description or plan…"
      status={status}
      onStatusChange={setStatus}
      statusLabel="Subissue status"
      error={save.error?.message}
      pending={save.isPending}
      submitDisabled={!title.trim()}
      submitLabel={save.isPending ? "Saving…" : subissue ? "Save subissue" : "Create subissue"}
      createMore={subissue ? undefined : { checked: createMore, onChange: setCreateMore }}
    />
  );
}
