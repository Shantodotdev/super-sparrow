import { useKeyboardShortcuts } from "./keyboard-shortcuts-provider";
import { matchesChord } from "../lib/shortcuts";
import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { GripHorizontal, X } from "lucide-react";
import {
  issuePriorities,
  issuePriorityLabel,
  issueStatusLabel,
  issueStatuses,
} from "../lib/issue-status";
import { statusIcon, statusIconColor } from "../lib/issue-status";
import type { IssuePriority, IssueStatus } from "../lib/types";
import { useCreateIssue } from "../queries/issues";
import { useAttachmentUploads } from "../queries/attachments";
import { useFileDrop } from "../hooks/use-file-drop";
import { actorsQuery, projectsQuery } from "../queries/project";
import { projectLabelsQuery, projectMilestonesQuery } from "../queries/project-resources";
import { FloatingMenu } from "./floating-menu";
import { Icon } from "./icon";
import { LabelDot } from "./label-dot";
import { AttachmentButtons, UploadFailures, UploadList } from "./attachment-queue";
import { MarkdownEditor } from "./markdown-editor";
import { Select } from "./select";
import { useCurrentProjectSlug } from "./shortcuts";
import { useToast } from "./toast";

type QuickCaptureProps = {
  open: boolean;
  onClose: () => void;
  /**
   * Fills its own window, as the desktop app's capture window does, instead of floating over a
   * page. The window itself is then what the person moves.
   */
  standalone?: boolean;
};

type Position = { x: number; y: number };

const positionKey = "sparrow:quick-capture-position";
const panelWidth = 560;

const priorityOptions = [
  {
    value: "",
    label: "No priority",
    icon: "circle-dashed" as const,
    iconClassName: "text-[#a9b6c8]",
  },
  ...issuePriorities.map((priority: IssuePriority) => ({
    value: priority,
    label: issuePriorityLabel[priority],
    icon:
      priority === "urgent" || priority === "high"
        ? ("triangle-alert" as const)
        : ("circle-dot" as const),
    iconClassName:
      priority === "urgent"
        ? "text-[#f2a9ae]"
        : priority === "high"
          ? "text-[#f2ad67]"
          : priority === "medium"
            ? "text-[#f2cf87]"
            : "text-[#a7c0ff]",
  })),
];

/**
 * A movable panel for writing down a problem the moment it is noticed: an issue with its status,
 * priority, assignee, milestone, labels, and files. It stays open while the person works
 * elsewhere on the page and closes only when they create the issue, press the close button, or
 * press the capture shortcut again. Its draft survives being closed and reopened. Files upload
 * once the issue exists; if some fail, the issue stays and they can be retried.
 */
export function QuickCapture({ open, onClose, standalone = false }: QuickCaptureProps) {
  const { notify } = useToast();
  const createIssue = useCreateIssue();
  const uploads = useAttachmentUploads();
  const rememberedSlug = useCurrentProjectSlug();
  const projects = useQuery(projectsQuery);
  const people = useQuery({ ...actorsQuery, enabled: open });

  const { bindings } = useKeyboardShortcuts();
  const [createdKey, setCreatedKey] = useState("");
  const [projectSlug, setProjectSlug] = useState<string | undefined>(undefined);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [status, setStatus] = useState<IssueStatus>("backlog");
  const [priority, setPriority] = useState<IssuePriority>("");
  const [assigneeId, setAssigneeId] = useState("");
  const [milestoneId, setMilestoneId] = useState("");
  const [labelIds, setLabelIds] = useState<string[]>([]);
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [position, setPosition] = useState<Position | null>(null);
  const titleInput = useRef<HTMLInputElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const labelTrigger = useRef<HTMLButtonElement>(null);
  const drag = useRef<{ dx: number; dy: number } | null>(null);

  const project =
    projects.data?.find((item) => item.slug === (projectSlug ?? rememberedSlug)) ??
    projects.data?.[0];
  const labels = useQuery({
    ...projectLabelsQuery(project?.key ?? ""),
    enabled: open && !!project,
  });
  const milestones = useQuery({
    ...projectMilestonesQuery(project?.key ?? ""),
    enabled: open && !!project,
  });

  // Each opening focuses the title, so the person can start typing at once.
  useEffect(() => {
    if (open) requestAnimationFrame(() => titleInput.current?.focus());
  }, [open]);

  // The personal close binding closes the open panel, even while typing in it.
  useEffect(() => {
    if (!open) return;
    const close = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat || !matchesChord(event, bindings.closeCapture))
        return;
      event.preventDefault();
      onClose();
    };
    window.addEventListener("keydown", close, true);
    return () => window.removeEventListener("keydown", close, true);
  }, [open, onClose, bindings.closeCapture]);

  // The panel opens where it was last left, or near the top right of the window.
  useEffect(() => {
    if (standalone || position) return;
    let saved: Position | null = null;
    try {
      const raw = window.localStorage.getItem(positionKey);
      if (raw) saved = JSON.parse(raw) as Position;
    } catch {
      // A missing or blocked position falls back to the default place.
    }
    setPosition(clamp(saved ?? { x: window.innerWidth - panelWidth - 32, y: 72 }, panel.current));
  }, [standalone, position]);

  // Keeps the panel reachable when the window shrinks.
  useEffect(() => {
    if (standalone) return;
    const keepInside = () => setPosition((current) => current && clamp(current, panel.current));
    window.addEventListener("resize", keepInside);
    return () => window.removeEventListener("resize", keepInside);
  }, [standalone]);

  const switchProject = (slug: string) => {
    setProjectSlug(slug);
    setMilestoneId("");
    setLabelIds([]);
  };

  const reset = () => {
    setTitle("");
    setBody("");
    setStatus("backlog");
    setPriority("");
    setAssigneeId("");
    setMilestoneId("");
    setLabelIds([]);
    setError("");
  };

  const addFiles = (files: File[]) => {
    if (files.length === 0 || pending) return;
    const { rejected } = uploads.add(files);
    if (rejected.length > 0) setError(rejected.join(" "));
  };
  const { dragging, handlers: dropHandlers } = useFileDrop(addFiles, !open || !!createdKey);

  const finishUploads = () => {
    uploads.clear();
    setCreatedKey("");
    onClose();
  };

  const submit = async () => {
    if (!project || !title.trim() || pending || createdKey) return;
    setPending(true);
    setError("");
    try {
      const { issue } = await createIssue({
        title: title.trim(),
        plan: body.trim(),
        status,
        priority,
        projectKey: project.key,
        assigneeId,
        milestoneId,
        labelIds,
      });
      notify(`Created ${issue.key}`);

      // Files upload to the new issue; when some fail the issue stays and they can be retried.
      const files = uploads.jobs;
      const failed = files.length > 0 ? await uploads.start(issue.key) : [];
      for (const job of files) {
        if (!failed.some((item) => item.id === job.id)) uploads.remove(job.id);
      }
      reset();
      if (failed.length > 0) {
        setCreatedKey(issue.key);
        return;
      }
      onClose();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Could not save");
    } finally {
      setPending(false);
    }
  };

  const startDrag = (event: ReactPointerEvent<HTMLElement>) => {
    if (standalone || event.button !== 0) return;
    if ((event.target as HTMLElement).closest("button, input, select, a")) return;
    const box = panel.current?.getBoundingClientRect();
    if (!box) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { dx: event.clientX - box.left, dy: event.clientY - box.top };
  };
  const moveDrag = (event: ReactPointerEvent<HTMLElement>) => {
    if (!drag.current) return;
    setPosition(
      clamp(
        { x: event.clientX - drag.current.dx, y: event.clientY - drag.current.dy },
        panel.current,
      ),
    );
  };
  const endDrag = () => {
    if (!drag.current) return;
    drag.current = null;
    try {
      if (position) window.localStorage.setItem(positionKey, JSON.stringify(position));
    } catch {
      // Remembering the place is a convenience; blocked storage forgets it.
    }
  };

  const assignable = (people.data ?? []).filter(
    (actor) => actor.kind !== "system" && !actor.disabled_at,
  );
  const projectLabels = labels.data ?? [];
  const selectedLabels = projectLabels.filter((label) => labelIds.includes(label.id));

  return (
    <div
      ref={panel}
      hidden={!open}
      role={open ? "dialog" : undefined}
      aria-modal="false"
      aria-label="Quick capture"
      className={
        standalone
          ? `flex h-dvh w-full flex-col overflow-hidden rounded-2xl border border-[#262a32] bg-[#0d0e11] text-[#e7e8ec] ${dragging ? "ring-1 ring-[#426fff] ring-inset" : ""}`
          : `fixed z-60 flex max-h-[calc(100dvh-24px)] min-h-[300px] w-[min(560px,calc(100vw-24px))] min-w-[340px] resize flex-col overflow-hidden rounded-2xl border border-[#3a3e48] bg-[#15171b]/98 text-[#e7e8ec] shadow-[0_24px_80px_#000c,0_0_0_1px_#0006] backdrop-blur ${dragging ? "border-[#426fff]" : ""}`
      }
      style={standalone || !position ? undefined : { left: position.x, top: position.y }}
      {...dropHandlers}
      onKeyDown={(event) => {
        if (matchesChord(event, bindings.submit)) {
          event.preventDefault();
          void submit();
        }
      }}
    >
      <header
        className={`flex h-11 shrink-0 items-center justify-between px-3 ${standalone ? "" : "cursor-grab active:cursor-grabbing"}`}
        onPointerDown={startDrag}
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        data-tauri-drag-region={standalone ? "" : undefined}
      >
        <GripHorizontal aria-hidden="true" className="pointer-events-none size-4 text-[#4a4d57]" />
        <button
          type="button"
          aria-label="Close quick capture"
          className="grid size-7 place-items-center rounded-full border-0 bg-[#1a1c21] text-[#9a9da6] transition-colors hover:bg-[#2c3038] hover:text-white"
          onClick={onClose}
        >
          <X aria-hidden="true" className="size-3.5" strokeWidth={2.25} />
        </button>
      </header>

      {createdKey && (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          <UploadFailures issueKey={createdKey} uploads={uploads} onDone={finishUploads} />
        </div>
      )}

      <div
        hidden={!!createdKey}
        className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-5 pt-2 pb-3"
      >
        {error && (
          <p role="alert" className="m-0 text-[13px] text-[#f2a9ae]">
            {error}
          </p>
        )}
        <input
          ref={titleInput}
          id="quick-capture-title"
          aria-label="Issue title"
          className="h-10 w-full rounded-none border-0 bg-transparent p-0 text-[20px] leading-10 font-semibold text-[#f1f2f4] caret-[#7896ff] outline-none placeholder:text-[#5f6370] focus-visible:outline-none"
          placeholder="Issue title"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
        />
        <MarkdownEditor
          label="Issue description"
          placeholder="Add a description…"
          value={body}
          onChange={setBody}
          className="[&_.tiptap]:min-h-[110px]! [&_.tiptap]:text-[14px]! [&_.tiptap]:text-[#e4e6ea]!"
        />
      </div>

      {uploads.jobs.length > 0 && (
        <div hidden={!!createdKey} className="max-h-[132px] shrink-0 overflow-y-auto px-4 pt-2">
          <UploadList jobs={uploads.jobs} onRemove={(job) => uploads.remove(job.id)} showDone />
        </div>
      )}

      <div
        hidden={!!createdKey}
        className="flex shrink-0 flex-wrap items-center gap-[6px] px-4 pt-2 [&_button]:min-h-9 [&_button]:text-[13px]"
      >
        <Select
          variant="chip"
          icon="folder"
          label="Project"
          value={project?.slug ?? ""}
          onChange={switchProject}
          options={(projects.data ?? []).map((item) => ({ value: item.slug, label: item.name }))}
        />
        <Select
          variant="chip"
          label="Status"
          value={status}
          onChange={(value) => setStatus(value as IssueStatus)}
          options={issueStatuses.map((value) => ({
            value,
            label: issueStatusLabel[value],
            icon: statusIcon[value],
            iconClassName: statusIconColor[value],
          }))}
        />
        <Select
          variant="chip"
          label="Priority"
          value={priority}
          onChange={(value) => setPriority(value as IssuePriority)}
          options={priorityOptions}
        />
        <Select
          variant="chip"
          icon="circle-user-round"
          label="Assignee"
          value={assigneeId}
          onChange={setAssigneeId}
          options={[
            { value: "", label: "Assignee" },
            ...assignable.map((actor) => ({ value: actor.id, label: actor.display_name })),
          ]}
        />
        <Select
          variant="chip"
          icon="target"
          label="Milestone"
          value={milestoneId}
          onChange={setMilestoneId}
          options={[
            { value: "", label: "Milestone" },
            ...(milestones.data ?? []).map((milestone) => ({
              value: milestone.id,
              label: milestone.name,
            })),
          ]}
        />
        <button
          ref={labelTrigger}
          type="button"
          aria-haspopup="true"
          aria-expanded={labelsOpen}
          className="inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] whitespace-nowrap text-[#d6d7dc] hover:border-[#484b54] hover:bg-[#383b42]"
          onClick={() => setLabelsOpen((current) => !current)}
        >
          <Icon name="list-filter" className="size-4" />
          {selectedLabels.length ? `Labels · ${selectedLabels.length}` : "Labels"}
        </button>
        {labelsOpen && (
          <FloatingMenu
            anchor={labelTrigger}
            minWidth={220}
            onDismiss={() => setLabelsOpen(false)}
            aria-label="Choose labels"
            className="max-h-[260px] overflow-y-auto rounded-[12px] border border-[#303744] bg-[#171b21] p-1.5 text-[#d7dfeb] shadow-[0_12px_28px_#0009]"
          >
            {projectLabels.length === 0 && (
              <p className="m-0 px-3 py-2 text-[13px] text-[#8e9bac]">
                This project has no labels.
              </p>
            )}
            {projectLabels.map((label) => {
              const selected = labelIds.includes(label.id);
              return (
                <button
                  key={label.id}
                  type="button"
                  aria-pressed={selected}
                  className="flex min-h-9 w-full items-center justify-between gap-3 rounded-[5px] border-0 bg-transparent px-[9px] py-2 text-left text-sm text-[#bfcbdc] hover:bg-[#283140]"
                  onClick={() =>
                    setLabelIds((current) =>
                      selected ? current.filter((id) => id !== label.id) : [...current, label.id],
                    )
                  }
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <LabelDot color={label.color} />
                    <span className="truncate">{label.name}</span>
                  </span>
                  {selected && <Icon name="check" className="size-4 text-[#7896ff]" />}
                </button>
              );
            })}
          </FloatingMenu>
        )}
      </div>

      <footer
        hidden={!!createdKey}
        className="flex shrink-0 items-center justify-between gap-3 px-4 pt-2 pb-4 [&_button]:min-h-9 [&_button]:text-[13px]"
      >
        <div className="flex items-center gap-[6px]">
          <AttachmentButtons onFiles={addFiles} onError={setError} disabled={pending} />
        </div>
        <button
          type="button"
          className="rounded-full border border-[#3264ff] bg-[#2455eb] px-4 py-2 text-[13px] font-[550] text-white hover:bg-[#3566f6] disabled:opacity-50"
          disabled={pending || !title.trim() || !project}
          onClick={() => void submit()}
        >
          {pending ? "Saving…" : "Create issue"}
        </button>
      </footer>
    </div>
  );
}

/** Keeps the panel's top-left corner where the whole header stays on screen. */
function clamp(position: Position, element: HTMLElement | null): Position {
  const width = element?.offsetWidth || panelWidth;
  const margin = 8;
  return {
    x: Math.min(Math.max(margin, position.x), Math.max(margin, window.innerWidth - width - margin)),
    y: Math.min(Math.max(margin, position.y), Math.max(margin, window.innerHeight - 56)),
  };
}
