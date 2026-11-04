import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { validateIssuesSearch } from "../lib/search";
import { isTypingTarget, nextGoStep, shortcutGroups, type GoDestination } from "../lib/shortcuts";
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

/** How long the go prefix waits for its destination key. */
const goTimeoutMs = 1200;

/** Keeps modified keys, typing in fields, and open dialogs from triggering plain-key shortcuts. */
function shortcutsBlocked(event: KeyboardEvent): boolean {
  if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey) return true;
  if (isTypingTarget(event.target)) return true;

  return document.querySelector('[role="dialog"]') !== null;
}

/**
 * Runs the handler for a pressed key, such as "c" or "?". Handlers are looked up by lowercase key
 * and skipped while the person types or a dialog is open.
 */
export function useShortcutKeys(bindings: Record<string, () => void>): void {
  const latest = useRef(bindings);
  latest.current = bindings;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (shortcutsBlocked(event)) return;

      const handler = latest.current[event.key.toLowerCase()];
      if (!handler) return;

      event.preventDefault();
      handler();
    };

    window.addEventListener("keydown", onKeyDown);

    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
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
  const goPending = useRef(false);
  const goTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const segments = location.pathname.split("/").filter(Boolean);
  const onIssueList = segments.length === 2 && segments[1] === "issues";
  const issuesSearch = validateIssuesSearch(useSearch({ strict: false }));
  const onIssuePage = segments.length >= 3 && segments[1] === "issues";

  const goTo = (destination: GoDestination) => {
    if (destination === "my-issues") return void navigate({ to: "/issues" });
    if (destination === "activity") return void navigate({ to: "/activity" });
    if (destination === "people") return void navigate({ to: "/settings" });
    if (!projectSlug) return;

    void navigate({
      to: destination === "issues" ? "/$projectSlug/issues" : "/$projectSlug/milestones",
      params: { projectSlug },
    });
  };

  const latest = useRef(goTo);
  latest.current = goTo;

  // The go sequence is its own listener because its second key is only meaningful after the first.
  useEffect(() => {
    const clear = () => {
      goPending.current = false;
      clearTimeout(goTimer.current);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (shortcutsBlocked(event)) return;

      const step = nextGoStep(goPending.current, event.key);
      if (step.kind === "pending") {
        event.preventDefault();
        goPending.current = true;
        clearTimeout(goTimer.current);
        goTimer.current = setTimeout(clear, goTimeoutMs);
        return;
      }

      const wasPending = goPending.current;
      clear();
      if (step.kind === "go") {
        event.preventDefault();
        latest.current(step.destination);
      } else if (wasPending) {
        event.stopImmediatePropagation();
      }
    };

    window.addEventListener("keydown", onKeyDown, true);

    return () => {
      clear();
      window.removeEventListener("keydown", onKeyDown, true);
    };
  }, []);

  useShortcutKeys({
    "?": onHelp,
    c: () => {
      if (!projectSlug) return;

      void navigate({
        to: "/$projectSlug/issues",
        params: { projectSlug },
        search: { new: true },
        replace: onIssueList,
      });
    },
    "/": () => {
      document.querySelector<HTMLInputElement>('input[aria-label="Search issues"]')?.focus();
    },
    j: () => moveRowFocus(1),
    k: () => moveRowFocus(-1),
    v: () => {
      if (!onIssueList || !projectSlug) return;

      void navigate({
        to: "/$projectSlug/issues",
        params: { projectSlug },
        search: { ...issuesSearch, view: issuesSearch.view ? undefined : "board" },
      });
    },
    u: () => {
      if (!onIssuePage || !projectSlug) return;

      void navigate({ to: "/$projectSlug/issues", params: { projectSlug } });
    },
  });
}

/** Lists every keyboard shortcut, grouped by where it applies. */
export function ShortcutHelp({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      label="Keyboard shortcuts"
      className="max-h-[76vh] w-[min(100%,560px)] overflow-y-auto rounded-[10px] border border-[#303641] bg-[#12151a] p-[20px] shadow-[0_20px_70px_#000a]"
    >
      <div className="mb-4 flex justify-between text-[#94a1b3]">
        <strong className="text-[#edf0f6]">Keyboard shortcuts</strong>
        <kbd>esc</kbd>
      </div>
      {shortcutGroups.map((group) => (
        <section key={group.title} className="mb-[16px]" aria-label={group.title}>
          <h2 className="mb-[6px] text-[12px] font-semibold tracking-wide text-[#8290a5] uppercase">
            {group.title}
          </h2>
          <ul className="m-0 list-none p-0">
            {group.shortcuts.map((shortcut) => (
              <li
                key={shortcut.description}
                className="flex items-center justify-between py-[6px] text-[14px]"
              >
                <span>{shortcut.description}</span>
                <span className="flex gap-[6px]">
                  {shortcut.keys.map((key) => (
                    <kbd
                      key={key}
                      className="min-w-[24px] rounded-[5px] border border-[#303641] bg-[#1a1e25] px-2 py-[2px] text-center text-[12px] text-[#dce2ec]"
                    >
                      {key}
                    </kbd>
                  ))}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </Modal>
  );
}
