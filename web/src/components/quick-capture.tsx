import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bookmark, Minus, Send, SlidersHorizontal, X } from "lucide-react";
import { splitCapture } from "../lib/capture-text";
import {
  issuePriorities,
  issuePriorityLabel,
  issueStatusLabel,
  issueStatuses,
  statusIcon,
  statusIconColor,
} from "../lib/issue-status";
import type { IssuePriority, IssueStatus } from "../lib/types";
import { useCaptureShortcut } from "../hooks/use-capture-shortcut";
import { useCreateContext } from "../queries/contexts";
import { useCreateIssue } from "../queries/issues";
import { actorsQuery, projectsQuery } from "../queries/project";
import { projectLabelsQuery, projectMilestonesQuery } from "../queries/project-resources";
import { Avatar } from "./avatar";
import { FloatingMenu } from "./floating-menu";
import { LabelDot } from "./label-dot";
import { MarkdownEditor, appendImage } from "./markdown-editor";
import { ScreenshotButton } from "./screenshot-button";
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
const panelWidth = 480;

const priorityColor: Record<IssuePriority, string> = {
  "": "text-[#a9b6c8]",
  urgent: "text-[#f2a9ae]",
  high: "text-[#f2ad67]",
  medium: "text-[#f2cf87]",
  low: "text-[#a7c0ff]",
};

const priorityOptions = [
  {
    value: "",
    label: "No priority",
    icon: "circle-dashed" as const,
    iconClassName: priorityColor[""],
  },
  ...issuePriorities.map((priority) => ({
    value: priority,
    label: issuePriorityLabel[priority],
    icon:
      priority === "urgent" || priority === "high"
        ? ("triangle-alert" as const)
        : ("circle-dot" as const),
    iconClassName: priorityColor[priority],
  })),
];

const iconButton =
  "grid size-7 place-items-center rounded-full border-0 bg-transparent text-[#8a8e98] hover:bg-[#24272d] hover:text-white";

/**
 * A compact, movable panel for writing down a problem the moment it is noticed: a title, details
 * with screenshots inline, priority and assignee at hand, and status, milestone, and labels under
 * More. It saves as an issue or as a context. The panel stays open while the person works
 * elsewhere and closes only when they save, press its close button, or press the capture shortcut
 * again. Its draft survives being closed.
 */
export function QuickCapture({ open, onClose, standalone = false }: QuickCaptureProps) {
  const { notify } = useToast();
  const createIssue = useCreateIssue();
  const createContext = useCreateContext();
  const rememberedSlug = useCurrentProjectSlug();
  const shortcut = useCaptureShortcut();
  const projects = useQuery(projectsQuery);
  const people = useQuery({ ...actorsQuery, enabled: open });

  const [projectSlug, setProjectSlug] = useState<string | undefined>(undefined);
  const [title, setTitle] = useState("");
  const [details, setDetails] = useState("");
  const [status, setStatus] = useState<IssueStatus>("backlog");
  const [priority, setPriority] = useState<IssuePriority>("");
  const [assigneeId, setAssigneeId] = useState("");
  const [milestoneId, setMilestoneId] = useState("");
  const [labelIds, setLabelIds] = useState<string[]>([]);
  const [moreOpen, setMoreOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [pending, setPending] = useState<"issue" | "context" | null>(null);
  const [error, setError] = useState("");
  const [position, setPosition] = useState<Position | null>(null);
  const panel = useRef<HTMLDivElement>(null);
  const titleInput = useRef<HTMLInputElement>(null);
  const moreTrigger = useRef<HTMLButtonElement>(null);
  const moreMenu = useRef<HTMLDivElement>(null);
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

  // A typed title wins; without one, the first line of the details becomes the title.
  const fromDetails = splitCapture(details);
  const finalTitle = title.trim() || fromDetails.title;
  const finalBody = title.trim() ? details.trim() : fromDetails.body;

  // Each opening expands the panel and puts the cursor in the title.
  useEffect(() => {
    if (!open) return;
    setCollapsed(false);
    requestAnimationFrame(() => titleInput.current?.focus());
  }, [open]);

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

  // The More menu closes on a click outside it; its own dropdowns open inside it.
  useEffect(() => {
    if (!moreOpen) return;
    const dismiss = (event: PointerEvent) => {
      const target = event.target as Node;
      if (moreMenu.current?.contains(target) || moreTrigger.current?.contains(target)) return;
      setMoreOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [moreOpen]);

  const switchProject = (slug: string) => {
    setProjectSlug(slug);
    setMilestoneId("");
    setLabelIds([]);
  };

  const reset = () => {
    setTitle("");
    setDetails("");
    setStatus("backlog");
    setPriority("");
    setAssigneeId("");
    setMilestoneId("");
    setLabelIds([]);
    setMoreOpen(false);
    setError("");
  };

  const save = async (kind: "issue" | "context") => {
    if (!project || !finalTitle || pending) return;
    setPending(kind);
    setError("");
    try {
      if (kind === "issue") {
        const { issue } = await createIssue({
          title: finalTitle,
          plan: finalBody,
          status,
          priority,
          projectKey: project.key,
          assigneeId,
          milestoneId,
          labelIds,
        });
        notify(`Created ${issue.key}`);
      } else {
        const created = await createContext({
          projectKey: project.key,
          title: finalTitle,
          body: finalBody,
        });
        notify(`Added context ${created.context.key}`);
      }
      reset();
      onClose();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Could not save");
    } finally {
      setPending(null);
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
  // How many of the fields under More differ from a new issue's defaults.
  const moreSet =
    (status === "backlog" ? 0 : 1) + (milestoneId ? 1 : 0) + (labelIds.length ? 1 : 0);
  const canSave = !!project && !!finalTitle && !pending;

  return (
    <div
      ref={panel}
      hidden={!open}
      role="dialog"
      aria-modal="false"
      aria-labelledby="quick-capture-heading"
      className={
        standalone
          ? "flex h-dvh w-full flex-col overflow-hidden rounded-[22px] border border-[#2a2d34] bg-[#141619] text-[#e7e8ec]"
          : `fixed z-60 flex max-h-[calc(100dvh-24px)] w-[min(480px,calc(100vw-24px))] min-w-[340px] flex-col overflow-hidden rounded-[22px] border border-[#2a2d34] bg-[#141619]/98 text-[#e7e8ec] shadow-[0_24px_64px_#000b] backdrop-blur ${collapsed ? "" : "resize"}`
      }
      style={standalone || !position ? undefined : { left: position.x, top: position.y }}
      onKeyDown={(event) => {
        if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
          event.preventDefault();
          void save("issue");
        }
      }}
    >
      <header
        className={`flex shrink-0 items-center gap-2 px-4 pt-3 ${collapsed ? "pb-3" : "pb-2"} ${standalone ? "" : "cursor-grab active:cursor-grabbing"}`}
        onPointerDown={startDrag}
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        data-tauri-drag-region={standalone ? "" : undefined}
      >
        <img src="/logo.png" alt="" className="pointer-events-none size-5 rounded" />
        <h2
          id="quick-capture-heading"
          className="pointer-events-none m-0 text-[13px] font-medium text-[#c9ccd3]"
        >
          Super Sparrow
        </h2>
        {(projects.data?.length ?? 0) > 1 && (
          <div className="[&_button]:min-h-[22px]! [&_button]:py-0! [&_button]:text-[11px]!">
            <Select
              variant="chip"
              icon="folder"
              label="Project"
              value={project?.slug ?? ""}
              onChange={switchProject}
              options={(projects.data ?? []).map((item) => ({
                value: item.slug,
                label: item.name,
              }))}
            />
          </div>
        )}
        <span className="ml-auto" />
        {shortcut && (
          <kbd className="pointer-events-none font-sans text-[10px] tracking-wide text-[#5f636d]">
            {shortcut}
          </kbd>
        )}
        {!standalone && (
          <button
            type="button"
            aria-label={collapsed ? "Expand quick capture" : "Minimize quick capture"}
            aria-expanded={!collapsed}
            className={iconButton}
            onClick={() => setCollapsed((current) => !current)}
          >
            <Minus aria-hidden="true" className="size-4" />
          </button>
        )}
        <button
          type="button"
          aria-label="Close quick capture"
          className={iconButton}
          onClick={onClose}
        >
          <X aria-hidden="true" className="size-4" />
        </button>
      </header>

      <div hidden={collapsed} className="flex min-h-0 flex-1 flex-col gap-2.5 px-4 pb-4">
        {error && (
          <p role="alert" className="m-0 text-[12px] text-[#f2a9ae]">
            {error}
          </p>
        )}

        <div className="flex min-h-0 flex-1 flex-col rounded-[18px] bg-[#1b1d22] ring-1 ring-transparent transition-shadow focus-within:ring-[#3b5bd6]/60">
          <input
            ref={titleInput}
            id="quick-capture-title"
            aria-label="Title"
            className="w-full border-0 bg-transparent px-4 pt-3 pb-1 text-[15px] font-semibold text-[#f1f2f4] outline-none placeholder:font-medium placeholder:text-[#62666f]"
            placeholder="Title"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
          <div
            id="quick-capture-description"
            className="min-h-0 flex-1 overflow-y-auto px-4 pb-2.5"
          >
            <MarkdownEditor
              label="Details"
              placeholder="Add details, paste or capture screenshots…"
              value={details}
              onChange={setDetails}
              projectKey={project?.key}
              onImageError={setError}
              className="[&_.tiptap]:max-h-[180px] [&_.tiptap]:min-h-[52px]! [&_.tiptap]:text-[13px]! [&_.tiptap]:leading-relaxed! [&_.tiptap]:text-[#c9ccd3]! [&_img]:max-h-40 [&_img]:rounded-xl [&_p]:my-0.5!"
            />
          </div>
        </div>

        <div className="flex items-center gap-1 [&_.custom-select_button]:min-h-7! [&_.custom-select_button]:border-transparent! [&_.custom-select_button]:bg-transparent! [&_.custom-select_button]:py-0! [&_.custom-select_button]:text-[12px] [&_.custom-select_button]:text-[#a9adb6]! [&_.custom-select_button:hover]:bg-[#22252b]! [&_.custom-select_button[aria-expanded=true]]:bg-[#22252b]!">
          <ScreenshotButton
            compact
            projectKey={project?.key ?? ""}
            onCaptured={(attachment) =>
              setDetails((current) => appendImage(current, attachment.url))
            }
            onError={setError}
            className="size-7! rounded-full! border-transparent! bg-transparent! text-[#a9adb6]! hover:bg-[#22252b]!"
          />
          <span aria-hidden="true" className="mx-1 h-3.5 w-px bg-[#2a2d34]" />
          <Select
            variant="chip"
            label="Priority"
            value={priority}
            onChange={(value) => setPriority(value as IssuePriority)}
            options={priorityOptions}
          />
          <Select
            variant="chip"
            label="Assignee"
            icon="circle-user-round"
            value={assigneeId}
            onChange={setAssigneeId}
            options={[
              { value: "", label: "Assign" },
              ...assignable.map((actor) => ({
                value: actor.id,
                label: actor.display_name,
                leading: (
                  <span className="flex shrink-0 [&_.agent-icon]:size-4 [&_.avatar]:size-4 [&_.avatar]:basis-4 [&_.avatar]:text-[8px]">
                    <Avatar actor={actor} />
                  </span>
                ),
              })),
            ]}
          />
          <button
            ref={moreTrigger}
            type="button"
            aria-haspopup="true"
            aria-expanded={moreOpen}
            className="ml-auto inline-flex min-h-7 items-center gap-1.5 rounded-full border-0 bg-transparent px-2.5 text-[12px] whitespace-nowrap text-[#a9adb6] hover:bg-[#22252b] aria-expanded:bg-[#22252b]"
            onClick={() => setMoreOpen((current) => !current)}
          >
            <SlidersHorizontal aria-hidden="true" className="size-3.5 text-[#a9adb6]" />
            More
            {moreSet > 0 && (
              <span className="grid size-4 place-items-center rounded-full bg-[#2455eb] text-[10px] font-semibold text-white">
                {moreSet}
              </span>
            )}
          </button>
          {moreOpen && (
            <FloatingMenu
              anchor={moreTrigger}
              minWidth={250}
              onDismiss={() => setMoreOpen(false)}
              aria-label="More issue fields"
              className="w-[260px] rounded-[18px] border border-[#2a2d34] bg-[#16181c] p-3 text-[#d7dfeb] shadow-[0_12px_28px_#0009] [&_.custom-select]:w-full [&_.custom-select_button]:min-h-8! [&_.custom-select_button]:w-full! [&_.custom-select_button]:text-[12px]"
            >
              <div ref={moreMenu} className="grid gap-2.5">
                <MoreField label="Status">
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
                </MoreField>
                <MoreField label="Milestone">
                  <Select
                    variant="chip"
                    icon="target"
                    label="Milestone"
                    value={milestoneId}
                    onChange={setMilestoneId}
                    options={[
                      { value: "", label: "No milestone" },
                      ...(milestones.data ?? []).map((milestone) => ({
                        value: milestone.id,
                        label: milestone.name,
                      })),
                    ]}
                  />
                </MoreField>
                <MoreField label="Labels">
                  {projectLabels.length === 0 ? (
                    <p className="m-0 text-[12px] text-[#8e9bac]">This project has no labels.</p>
                  ) : (
                    <div className="flex max-h-[96px] flex-wrap gap-1 overflow-y-auto">
                      {projectLabels.map((label) => {
                        const selected = labelIds.includes(label.id);
                        return (
                          <button
                            key={label.id}
                            type="button"
                            aria-pressed={selected}
                            className="inline-flex items-center gap-1.5 rounded-full border border-[#303642] bg-[#101216] px-2 py-0.5 text-[11px] text-[#a9b6c8] hover:bg-[#20242c] aria-pressed:border-[#6685ee] aria-pressed:bg-[#202b45] aria-pressed:text-[#eaf0ff]"
                            onClick={() =>
                              setLabelIds((current) =>
                                selected
                                  ? current.filter((id) => id !== label.id)
                                  : [...current, label.id],
                              )
                            }
                          >
                            <LabelDot color={label.color} />
                            {label.name}
                          </button>
                        );
                      })}
                    </div>
                  )}
                </MoreField>
              </div>
            </FloatingMenu>
          )}
        </div>

        <footer className="grid grid-cols-2 gap-2">
          <button
            type="button"
            className="inline-flex min-h-9 items-center justify-center gap-2 rounded-full border-0 bg-[#1f2228] text-[13px] font-medium text-[#c9ccd3] hover:bg-[#262a31] disabled:opacity-40"
            disabled={!canSave}
            onClick={() => void save("context")}
            title="Save as a context for coding agents"
          >
            <Bookmark aria-hidden="true" className="size-4 text-[#8ea6ff]" />
            {pending === "context" ? "Adding…" : "Add to Context"}
          </button>
          <button
            type="button"
            className="inline-flex min-h-9 items-center justify-center gap-2 rounded-full border-0 bg-[#2455eb] text-[13px] font-semibold text-white hover:bg-[#3566f6] disabled:opacity-40"
            disabled={!canSave}
            onClick={() => void save("issue")}
            title="Ctrl+Enter"
          >
            <Send aria-hidden="true" className="size-4" />
            {pending === "issue" ? "Creating…" : "Create Issue"}
          </button>
        </footer>
      </div>
    </div>
  );
}

/** One labelled field in the More menu. */
function MoreField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-1">
      <span className="text-[11px] font-medium tracking-wide text-[#8290a5] uppercase">
        {label}
      </span>
      {children}
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
