import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useLocation, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { validateIssuesSearch } from "../lib/search";
import {
  isTypingTarget,
  matchesChord,
  shortcutDefinitions,
  formatBinding,
  type ShortcutID,
} from "../lib/shortcuts";
import { useKeyboardShortcuts } from "./keyboard-shortcuts-provider";
import { projectsQuery } from "../queries/project";
import { Modal } from "./modal";

const lastProjectKey = "sparrow:last-project-slug";

/**
 * The project shortcuts and the palette act on: the one in the URL, else the last one visited in
 * this browser, else the first. Visiting a project remembers it for pages outside any project.
 */
export function useCurrentProjectSlug(): string | undefined {
  const { projectSlug: routeSlug } = useParams({ strict: false });
  const { data: projects } = useQuery(projectsQuery);
  const known = projects?.some((project) => project.slug === routeSlug) ?? false;

  useEffect(() => {
    if (!routeSlug || !known) return;

    try {
      window.localStorage.setItem(lastProjectKey, routeSlug);
    } catch {
      // Remembering the project is a convenience; blocked storage falls back to the first project.
    }
  }, [routeSlug, known]);

  if (known) return routeSlug;

  let remembered: string | null = null;
  try {
    remembered = window.localStorage.getItem(lastProjectKey);
  } catch {
    // Blocked storage leaves nothing remembered.
  }

  return projects?.find((project) => project.slug === remembered)?.slug ?? projects?.[0]?.slug;
}

/** Dispatches single chords and two-step sequences outside editors and open dialogs. */
export function useShortcutKeys(handlers: Partial<Record<ShortcutID, () => void>>): void {
  const { bindings, recording } = useKeyboardShortcuts();
  const latest = useRef(handlers);
  latest.current = handlers;
  useEffect(() => {
    let pending: string | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const clear = () => {
      pending = null;
      clearTimeout(timer);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        recording ||
        event.defaultPrevented ||
        event.repeat ||
        event.isComposing ||
        isTypingTarget(event.target) ||
        document.querySelector('[role="dialog"]:not([hidden])')
      ) {
        clear();
        return;
      }
      const entries = Object.keys(latest.current) as ShortcutID[];
      if (pending) {
        const first = pending;
        clear();
        const id = entries.find((id) => {
          const steps = bindings[id].split(" ");
          return steps[0] === first && steps[1] !== undefined && matchesChord(event, steps[1]);
        });
        event.preventDefault();
        if (id) latest.current[id]?.();
        return;
      }
      for (const id of entries) {
        const steps = bindings[id].split(" ");
        if (!matchesChord(event, steps[0] ?? "")) continue;
        event.preventDefault();
        if (steps.length === 2) {
          pending = steps[0]!;
          timer = setTimeout(clear, 1200);
        } else latest.current[id]?.();
        return;
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      clear();
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [bindings, recording]);
}

/** The rows j and k move between: visible issue links on the current page. */
function visibleRows(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-shortcut-row]")).filter(
    (row) => row.getClientRects().length > 0 && !row.closest("[inert]"),
  );
}

/** Moves focus to the next or previous row, starting from the first row when none is focused. */
function moveRowFocus(step: 1 | -1): void {
  const rows = visibleRows();
  if (!rows.length) return;

  const current = rows.findIndex((row) => row === document.activeElement);
  const next = current === -1 ? (step === 1 ? 0 : rows.length - 1) : current + step;
  const target = rows[Math.min(Math.max(next, 0), rows.length - 1)]!;

  target.focus();
  target.scrollIntoView({ block: "nearest" });
}

/**
 * Registers the app-wide shortcuts: go-to sequences, creating an issue, searching, list movement,
 * and the help dialog. `onHelp` opens the shortcut list.
 */
export function useGlobalShortcuts(onHelp: () => void): void {
  const navigate = useNavigate();
  const location = useLocation();
  const projectSlug = useCurrentProjectSlug();
  const segments = location.pathname.split("/").filter(Boolean);
  const onIssueList = segments.length === 2 && segments[1] === "issues";
  const issuesSearch = validateIssuesSearch(useSearch({ strict: false }));
  const onIssuePage = segments.length >= 3 && segments[1] === "issues";

  useShortcutKeys({
    help: onHelp,
    goMine: () => {
      void navigate({ to: "/issues" });
    },
    goActivity: () => {
      void navigate({ to: "/activity" });
    },
    goPeople: () => {
      void navigate({ to: "/settings" });
    },
    goIssues: () => {
      if (projectSlug) void navigate({ to: "/$projectSlug/issues", params: { projectSlug } });
    },
    goMilestones: () => {
      if (projectSlug) void navigate({ to: "/$projectSlug/milestones", params: { projectSlug } });
    },
    newIssue: () => {
      if (!projectSlug) return;

      void navigate({
        to: "/$projectSlug/issues",
        params: { projectSlug },
        search: { new: true },
        replace: onIssueList,
      });
    },
    search: () => {
      document.querySelector<HTMLInputElement>('input[aria-label="Search issues"]')?.focus();
    },
    nextIssue: () => moveRowFocus(1),
    previousIssue: () => moveRowFocus(-1),
    toggleView: () => {
      if (!onIssueList || !projectSlug) return;

      void navigate({
        to: "/$projectSlug/issues",
        params: { projectSlug },
        search: { ...issuesSearch, view: issuesSearch.view ? undefined : "board" },
      });
    },
    up: () => {
      if (!onIssuePage || !projectSlug) return;

      void navigate({ to: "/$projectSlug/issues", params: { projectSlug } });
    },
  });
}

/** Shows the person's current shortcuts and links to their editing page. */
export function ShortcutHelp({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { bindings } = useKeyboardShortcuts();
  return (
    <Modal
      open={open}
      onClose={onClose}
      label="Keyboard shortcuts"
      className="max-h-[76vh] w-[min(100%,640px)] overflow-y-auto rounded-xl border border-[#303641] bg-[#12151a] p-5"
    >
      <div className="mb-4 flex justify-between">
        <strong>Keyboard shortcuts</strong>
        <Link to="/settings/shortcuts" onClick={onClose}>
          Customize shortcuts
        </Link>
      </div>
      {(Object.keys(shortcutDefinitions) as ShortcutID[]).map((id) => (
        <div key={id} className="flex items-center justify-between gap-4 py-2 text-sm">
          <span>{shortcutDefinitions[id].label}</span>
          <kbd>{formatBinding(bindings[id])}</kbd>
        </div>
      ))}
      <p className="mt-4 text-sm text-[#94a1b3]">
        Escape closes dialogs. Tab moves focus. Enter opens a focused link.
      </p>
    </Modal>
  );
}
