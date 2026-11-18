import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import {
  issueStatusLabel,
  issueStatuses,
  statusGroupBackground,
  statusIcon,
  statusIconColor,
} from "../lib/issue-status";
import type { Issue, IssueStatus, Subissue } from "../lib/types";
import { subissuesQuery, useSaveSubissue } from "../queries/subissues";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { SubissueDrawer } from "./subissue-drawer";
import { SubissueFormDialog } from "./subissue-form-dialog";
import { useSubissueContextMenu } from "./subissue-context-menu";
import { IssueDragPreview } from "./issue-drag-preview";
import { useIssueDrag } from "../hooks/use-issue-drag";
import { useDragAutoscroll } from "../hooks/use-drag-autoscroll";
import { useToast } from "./toast";

/** Shows an issue's lightweight children, each opening in a side drawer for editing. */
export function SubissueList({
  issue,
  creating,
  onCreatingChange,
}: {
  issue: Issue;
  creating: boolean;
  onCreatingChange: (creating: boolean) => void;
}) {
  const { data: subissues } = useSuspenseQuery(subissuesQuery(issue.key));
  const contextMenu = useSubissueContextMenu(issue);
  const [collapsed, setCollapsed] = useState<IssueStatus[]>([]);
  const sectionRef = useRef<HTMLElement>(null);
  const move = useSaveSubissue(issue.key);
  const { notify } = useToast();
  const { dragged, dropTarget, origin, dragProps } = useIssueDrag<Subissue>((subissue, status) =>
    move.mutate({ id: subissue.id, status }, { onError: (error) => notify(error.message) }),
  );

  useDragAutoscroll(!!dragged, sectionRef, 0);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const selected = subissues.find((item) => item.id === selectedId);
  const completed = subissues.filter((item) => item.status === "done").length;

  return (
    <section ref={sectionRef} className="my-[20px] w-full" aria-label="Subissues">
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
          onClick={() => onCreatingChange(true)}
          className="grid size-7 place-items-center rounded-[6px] border-0 bg-transparent p-0 text-[#9a9da6] transition-colors hover:bg-[#303238] hover:text-white"
        >
          <Icon name="plus" className="size-4" />
        </button>
      </div>
      {subissues.length === 0 && (
        <p className="muted mt-[3px] text-[12px] text-[#a2aec2]">No subissues yet.</p>
      )}
      {issueStatuses.map((status) => {
        const rows = subissues.filter((subissue) => subissue.status === status);
        if (!rows.length && !dragged) return null;

        const isCollapsed = collapsed.includes(status);

        return (
          <section key={status} className="relative [&+section]:mt-[8px]" data-drop-status={status}>
            <span
              aria-hidden="true"
              className={`pointer-events-none absolute inset-0 z-2 rounded-[8px] border-2 transition-[border-color,background-color] duration-150 motion-reduce:transition-none ${
                dropTarget === status && dragged?.status !== status
                  ? "border-[#5873c7] bg-[#5873c7]/5"
                  : "border-transparent"
              }`}
            />
            <button
              type="button"
              className={`group/toggle flex w-full items-center gap-[10px] rounded-[8px] border-0 px-[12px] py-[8px] text-left ${statusGroupBackground[status]}`}
              aria-expanded={!isCollapsed}
              onClick={() =>
                setCollapsed((current) =>
                  current.includes(status)
                    ? current.filter((item) => item !== status)
                    : [...current, status],
                )
              }
            >
              <Icon
                name="chevron-down"
                className="transition-transform duration-180 ease-[ease] group-aria-[expanded=false]/toggle:-rotate-90 motion-reduce:transition-none"
              />
              <Icon name={statusIcon[status]} className={statusIconColor[status]} />
              <strong>{issueStatusLabel[status]}</strong>
              <span className="text-[13px] font-medium text-[#9ba9bc]">{rows.length}</span>
            </button>
            {!isCollapsed && (
              <ul className="m-0 list-none p-0 pl-[24px]">
                {rows.map((subissue) => (
                  <li key={subissue.id}>
                    <button
                      type="button"
                      aria-label={`${subissue.key}: ${subissue.title}, ${issueStatusLabel[subissue.status]}`}
                      className={`grid min-h-[45px] w-full grid-cols-[20px_max-content_minmax(0,1fr)_auto] items-center gap-[10px] rounded-[10px] border-0 px-[12px] py-[6px] text-left transition-colors ${
                        dragged
                          ? dragged.id === subissue.id
                            ? "bg-[#1e222a]"
                            : dropTarget === status
                              ? "bg-transparent opacity-35"
                              : "bg-transparent"
                          : contextMenu.activeId === subissue.id
                            ? "bg-[#18191a]"
                            : `bg-transparent ${contextMenu.activeId ? "" : "hover:bg-[#18191a]"}`
                      }`}
                      {...dragProps(subissue)}
                      onContextMenu={contextMenu.open(subissue)}
                      onClick={() => {
                        setSelectedId(subissue.id);
                        setDrawerOpen(true);
                      }}
                    >
                      <Icon
                        name={statusIcon[subissue.status]}
                        className={`size-4 shrink-0 ${statusIconColor[subissue.status]}`}
                      />
                      <span className="text-[13px] text-[#aebed4]">{subissue.key}</span>
                      <span className="min-w-0 truncate text-[14px] text-[#eef0f5]">
                        {subissue.title}
                      </span>
                      <span className="max-w-[260px] justify-self-end [&>span]:flex-nowrap [&>span]:overflow-hidden">
                        <LabelChips labelIds={subissue.label_ids} />
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        );
      })}
      {contextMenu.menu}
      {dragged && <IssueDragPreview issue={dragged} origin={origin} />}
      <SubissueCreator issue={issue} open={creating} onClose={() => onCreatingChange(false)} />
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
