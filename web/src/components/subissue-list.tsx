import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { issueStatusLabel, statusIcon, statusIconColor } from "../lib/issue-status";
import type { Issue, IssueStatus } from "../lib/types";
import { subissuesQuery, useSaveSubissue } from "../queries/subissues";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { SubissueDrawer } from "./subissue-drawer";
import { SubissueFormDialog } from "./subissue-form-dialog";

/** Shows lightweight children with inline editing in a side drawer from either list. */
export function SubissueList({ issue, compact = false }: { issue: Issue; compact?: boolean }) {
  const { data: subissues } = useSuspenseQuery(subissuesQuery(issue.key));
  const [creating, setCreating] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const selected = subissues.find((item) => item.id === selectedId);
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
          onClick={() => setCreating(true)}
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
                  : "grid min-h-[45px] w-full grid-cols-[20px_max-content_minmax(0,1fr)_auto] items-center gap-[10px] rounded-[10px] border-0 bg-transparent px-[12px] py-[6px] text-left transition-colors hover:bg-[#18191a]"
              }
              onClick={() => {
                setSelectedId(subissue.id);
                setDrawerOpen(true);
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
                  <span className="max-w-[260px] justify-self-end [&>span]:flex-nowrap [&>span]:overflow-hidden">
                    <LabelChips labelIds={subissue.label_ids} />
                  </span>
                </>
              )}
            </button>
          </li>
        ))}
      </ul>
      <SubissueCreator issue={issue} open={creating} onClose={() => setCreating(false)} />
      <SubissueDrawer
        issue={issue}
        subissue={selected}
        open={drawerOpen && !!selected}
        onClose={() => setDrawerOpen(false)}
      />
    </section>
  );
}

/** Creates a subissue without losing its draft when a request fails. */
function SubissueCreator({
  issue,
  open,
  onClose,
}: {
  issue: Issue;
  open: boolean;
  onClose: () => void;
}) {
  const titleInput = useRef<HTMLInputElement>(null);
  const [createMore, setCreateMore] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [status, setStatus] = useState<IssueStatus>("backlog");
  const [labelIds, setLabelIds] = useState<string[]>([]);
  const save = useSaveSubissue(issue.key);
  useEffect(() => {
    if (!open) return;
    setTitle("");
    setDescription("");
    setStatus("backlog");
    setLabelIds([]);
    setCreateMore(false);
    save.reset();
  }, [open, save.reset]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (save.isPending) return;
    try {
      await save.mutateAsync({ title, description, status, label_ids: labelIds });
      if (createMore) {
        setTitle("");
        setDescription("");
        setStatus("backlog");
        setLabelIds([]);
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
      heading="New subissue"
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
      labels={{
        projectId: issue.project_id,
        selected: labelIds,
        onChange: async (ids) => setLabelIds(ids),
      }}
      error={save.error?.message}
      pending={save.isPending}
      submitDisabled={!title.trim()}
      submitLabel={save.isPending ? "Saving…" : "Create subissue"}
      createMore={{ checked: createMore, onChange: setCreateMore }}
    />
  );
}
