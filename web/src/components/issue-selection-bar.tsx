import { useEffect, useRef, useState } from "react";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { IssueStatus } from "../lib/types";
import type { Project } from "../queries/resources";
import { Icon } from "./icon";

type IssueSelectionBarProps = {
  count: number;
  projects: Project[];
  onMove: (status: IssueStatus) => void;
  onMoveToProject: (project: Project) => void;
  onClear: () => void;
};

/** A floating bar over the issue list that sets the status of every selected issue at once. */
export function IssueSelectionBar({
  count,
  projects,
  onMove,
  onMoveToProject,
  onClear,
}: IssueSelectionBarProps) {
  const [projectsOpen, setProjectsOpen] = useState(false);
  const picker = useRef<HTMLDivElement>(null);

  // The project list closes on a press outside it or on Escape, without leaving selection mode.
  useEffect(() => {
    if (!projectsOpen) return;
    const outside = (event: PointerEvent) => {
      if (!picker.current?.contains(event.target as Node)) setProjectsOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setProjectsOpen(false);
    };
    window.addEventListener("pointerdown", outside, true);
    window.addEventListener("keydown", escape, true);
    return () => {
      window.removeEventListener("pointerdown", outside, true);
      window.removeEventListener("keydown", escape, true);
    };
  }, [projectsOpen]);

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
      className="fixed bottom-6 left-1/2 z-30 flex max-w-[calc(100vw-32px)] -translate-x-1/2 items-center gap-2 rounded-xl border border-[#303744] bg-[#171a20] px-3 py-2 shadow-[0_12px_32px_rgba(0,0,0,0.5)]"
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
      <div ref={picker} className="relative shrink-0">
        <button
          type="button"
          aria-haspopup="menu"
          aria-expanded={projectsOpen}
          className="flex items-center gap-1.5 rounded-md border-0 bg-transparent px-2 py-1 text-[13px] whitespace-nowrap text-[#bfcbdc] hover:bg-white/10 hover:text-white aria-expanded:bg-white/10"
          onClick={() => setProjectsOpen((open) => !open)}
        >
          <Icon name="folder" className="text-[#95a6bd]" />
          Move to project
        </button>
        {projectsOpen && (
          <div
            role="menu"
            aria-label="Move to project"
            className="absolute right-0 bottom-[calc(100%+12px)] flex max-h-80 w-[220px] flex-col gap-0.5 overflow-y-auto rounded-xl border border-[#303744] bg-[#171a20] p-1.5 shadow-[0_12px_32px_rgba(0,0,0,0.5)]"
          >
            {projects.map((project) => (
              <button
                key={project.id}
                type="button"
                role="menuitem"
                className="flex h-9 items-center gap-3 rounded-lg border-0 bg-transparent px-3 text-left text-[13px] text-[#d7dde8] hover:bg-white/8 focus:outline-none focus-visible:bg-white/8"
                onClick={() => {
                  setProjectsOpen(false);
                  onMoveToProject(project);
                }}
              >
                <Icon name="folder" className="text-[#95a6bd]" />
                <span className="min-w-0 flex-1 truncate">{project.name}</span>
              </button>
            ))}
          </div>
        )}
      </div>
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
