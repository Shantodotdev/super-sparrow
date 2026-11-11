import { useSuspenseQuery } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { useProjectSlugs } from "../hooks/use-project";
import {
  issuePriorities,
  issuePriorityColor,
  issuePriorityIcon,
  issuePriorityLabel,
  issueStatusLabel,
  issueStatuses,
  statusIcon,
  statusIconColor,
} from "../lib/issue-status";
import type { Issue, IssuePriority } from "../lib/types";
import { issuesQuery, useQuickPatchIssue } from "../queries/issues";
import { actorsQuery, labelsQuery, milestonesQuery } from "../queries/project";
import { Avatar } from "./avatar";
import { Icon } from "./icon";
import { LabelDot } from "./label-dot";
import { useToast } from "./toast";

type MenuTarget = { issue: Issue; x: number; y: number };

const itemClass =
  "flex h-9 w-full items-center gap-3 rounded-[7px] border-0 bg-transparent px-3 transition-colors duration-100 text-left text-[13px] text-[#d7dde8] hover:bg-white/8 focus:outline-none focus-visible:bg-white/8 aria-expanded:bg-white/8 disabled:cursor-default disabled:text-[#6f7a8b] disabled:hover:bg-transparent";
const panelClass =
  "z-100 flex w-[240px] flex-col gap-0.5 rounded-[12px] border border-[#2c323c] bg-[#1a1d23] p-1.5 shadow-[0_12px_32px_rgba(0,0,0,0.5)]";

/** Opens a Linear-style menu of an issue's properties on right-click. */
export function useIssueContextMenu() {
  const [target, setTarget] = useState<MenuTarget>();

  const open = (issue: Issue) => (event: MouseEvent) => {
    event.preventDefault();
    setTarget({ issue, x: event.clientX, y: event.clientY });
  };

  const menu = target
    ? createPortal(
        <IssueContextMenu
          key={`${target.issue.id}:${target.x}:${target.y}`}
          target={target}
          onClose={() => setTarget(undefined)}
        />,
        document.body,
      )
    : null;

  return { open, menu, activeId: target?.issue.id };
}

/** Lists the properties of one issue as pickers; each choice is saved at once. */
function IssueContextMenu({
  target,
  onClose: dismiss,
}: {
  target: MenuTarget;
  onClose: () => void;
}) {
  const [closing, setClosing] = useState(false);
  const closeTimer = useRef<number | undefined>(undefined);
  const dismissRef = useRef(dismiss);
  dismissRef.current = dismiss;

  useEffect(() => () => window.clearTimeout(closeTimer.current), []);

  // Plays the exit animation before the menu is removed.
  const onClose = useCallback(() => {
    if (closeTimer.current !== undefined) return;
    setClosing(true);
    closeTimer.current = window.setTimeout(() => dismissRef.current(), 110);
  }, []);

  const { data: issues } = useSuspenseQuery(issuesQuery);
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const { data: milestones } = useSuspenseQuery(milestonesQuery);
  const projectSlug = useProjectSlugs();
  const patchIssue = useQuickPatchIssue();
  const { notify } = useToast();
  const root = useRef<HTMLDivElement>(null);
  const [openSub, setOpenSub] = useState<string>();

  // Follow optimistic edits so checkmarks update while the menu stays open.
  const issue = issues.find((item) => item.id === target.issue.id) ?? target.issue;
  const priority: IssuePriority = issue.priority ?? "";
  const projectLabels = labels.filter((label) => label.project_id === issue.project_id);
  const projectMilestones = milestones.filter((item) => item.project_id === issue.project_id);
  const people = actors.filter(
    (actor) =>
      actor.kind === "human" && (!actor.disabled_at || actor.id === issue.assignee_actor_id),
  );
  const assignee = people.find((actor) => actor.id === issue.assignee_actor_id);
  const milestone = projectMilestones.find((item) => item.id === issue.milestone_id);

  // Place the menu at the pointer, kept inside the viewport.
  useLayoutEffect(() => {
    const element = root.current;
    if (!element) return;

    const edge = 8;
    const bounds = element.getBoundingClientRect();
    element.style.left = `${Math.max(edge, Math.min(target.x, window.innerWidth - bounds.width - edge))}px`;
    element.style.top = `${Math.max(edge, Math.min(target.y, window.innerHeight - bounds.height - edge))}px`;
    element.focus({ preventScroll: true });
  }, [target.x, target.y]);

  // Dismiss on any interaction outside the menu.
  useEffect(() => {
    const outside = (event: Event) => {
      if (root.current?.contains(event.target as Node)) return;
      onClose();
      if (event.type !== "pointerdown") return;

      // The click that closes the menu must not also open the issue under the pointer.
      const swallow = (click: Event) => {
        click.preventDefault();
        click.stopPropagation();
      };
      window.addEventListener("click", swallow, { capture: true, once: true });
      window.addEventListener(
        "pointerup",
        () => window.setTimeout(() => window.removeEventListener("click", swallow, true), 0),
        { capture: true, once: true },
      );
    };
    window.addEventListener("pointerdown", outside, true);
    window.addEventListener("contextmenu", outside, true);
    window.addEventListener("scroll", outside, true);
    window.addEventListener("wheel", outside, { capture: true, passive: true });
    window.addEventListener("resize", onClose);
    window.addEventListener("blur", onClose);

    return () => {
      window.removeEventListener("pointerdown", outside, true);
      window.removeEventListener("contextmenu", outside, true);
      window.removeEventListener("scroll", outside, true);
      window.removeEventListener("wheel", outside, true);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("blur", onClose);
    };
  }, [onClose]);

  const choose = (change: Partial<Issue>, keepOpen = false) => {
    patchIssue(issue, change);
    if (!keepOpen) onClose();
  };

  const copy = (text: string, message: string) => {
    void navigator.clipboard.writeText(text).then(
      () => notify(message),
      () => notify("Could not copy to the clipboard"),
    );
    onClose();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const current = (event.target as HTMLElement).closest<HTMLElement>("[role=menu]");
    if (!current) return;

    const items = [
      ...(root.current?.querySelectorAll<HTMLElement>("[data-menu-item]") ?? []),
    ].filter((item) => item.closest("[role=menu]") === current && !item.hasAttribute("disabled"));
    const index = items.indexOf(event.target as HTMLElement);

    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      items[(index + step + items.length) % items.length]?.focus();
    } else if (event.key === "ArrowRight" && (event.target as HTMLElement).dataset.parent) {
      event.preventDefault();
      setOpenSub((event.target as HTMLElement).dataset.parent);
      requestAnimationFrame(() =>
        (event.target as HTMLElement).nextElementSibling
          ?.querySelector<HTMLElement>("[data-menu-item]")
          ?.focus(),
      );
    } else if (event.key === "ArrowLeft" && current.dataset.sub) {
      event.preventDefault();
      const parent = current.previousElementSibling as HTMLElement | null;
      setOpenSub(undefined);
      parent?.focus();
    } else if (event.key === "Escape") {
      event.preventDefault();
      if (current.dataset.sub) {
        (current.previousElementSibling as HTMLElement | null)?.focus();
        setOpenSub(undefined);
      } else {
        onClose();
      }
    }
  };

  const link = `${window.location.origin}/${projectSlug(issue.project_id)}/issues/${issue.key}`;

  return (
    <div
      ref={root}
      role="menu"
      tabIndex={-1}
      aria-label={`Actions for ${issue.key}`}
      className={`context-menu fixed outline-none ${panelClass}`}
      data-closing={closing || undefined}
      style={{ left: target.x, top: target.y }}
      onKeyDown={onKeyDown}
      onContextMenu={(event) => event.preventDefault()}
    >
      <Parent
        id="status"
        label="Status"
        icon={<Icon name={statusIcon[issue.status]} className={statusIconColor[issue.status]} />}
        openSub={openSub}
        setOpenSub={setOpenSub}
      >
        {issueStatuses.map((status) => (
          <Item
            key={status}
            label={issueStatusLabel[status]}
            icon={<Icon name={statusIcon[status]} className={statusIconColor[status]} />}
            checked={issue.status === status}
            onSelect={() => choose({ status })}
          />
        ))}
      </Parent>
      <Parent
        id="priority"
        label="Priority"
        icon={<Icon name={issuePriorityIcon[priority]} className={issuePriorityColor[priority]} />}
        openSub={openSub}
        setOpenSub={setOpenSub}
      >
        {(["", ...issuePriorities] as IssuePriority[]).map((value) => (
          <Item
            key={value || "none"}
            label={issuePriorityLabel[value]}
            icon={<Icon name={issuePriorityIcon[value]} className={issuePriorityColor[value]} />}
            checked={priority === value}
            onSelect={() => choose({ priority: value })}
          />
        ))}
      </Parent>
      <Parent
        id="assignee"
        label="Assignee"
        icon={<Icon name="circle-user-round" className="text-[#95a6bd]" />}
        detail={assignee?.display_name}
        openSub={openSub}
        setOpenSub={setOpenSub}
      >
        <Item
          label="Unassigned"
          icon={<Icon name="circle-user-round" className="text-[#95a6bd]" />}
          checked={!issue.assignee_actor_id}
          onSelect={() => choose({ assignee_actor_id: "" })}
        />
        {people.map((actor) => (
          <Item
            key={actor.id}
            label={actor.display_name}
            icon={
              <span className="flex shrink-0 [&_.avatar]:size-5 [&_.avatar]:basis-5 [&_.avatar]:text-[9px]">
                <Avatar actor={actor} />
              </span>
            }
            checked={issue.assignee_actor_id === actor.id}
            onSelect={() => choose({ assignee_actor_id: actor.id })}
          />
        ))}
      </Parent>
      <Parent
        id="labels"
        label="Labels"
        icon={<Icon name="tag" className="text-[#95a6bd]" />}
        detail={issue.label_ids.length ? String(issue.label_ids.length) : undefined}
        openSub={openSub}
        setOpenSub={setOpenSub}
      >
        {projectLabels.length === 0 && <Item label="No labels" disabled />}
        {projectLabels.map((label) => {
          const checked = issue.label_ids.includes(label.id);

          return (
            <Item
              key={label.id}
              label={label.name}
              icon={<LabelDot color={label.color} />}
              checked={checked}
              onSelect={() =>
                choose(
                  {
                    label_ids: checked
                      ? issue.label_ids.filter((id) => id !== label.id)
                      : [...issue.label_ids, label.id],
                  },
                  true,
                )
              }
            />
          );
        })}
      </Parent>
      <Parent
        id="milestone"
        label="Milestone"
        icon={<Icon name="target" className="text-[#95a6bd]" />}
        detail={milestone?.name}
        openSub={openSub}
        setOpenSub={setOpenSub}
      >
        <Item
          label="No milestone"
          icon={<Icon name="target" className="text-[#95a6bd]" />}
          checked={!issue.milestone_id}
          onSelect={() => choose({ milestone_id: "" })}
        />
        {projectMilestones.map((item) => (
          <Item
            key={item.id}
            label={item.name}
            icon={<Icon name="target" className="text-[#95a6bd]" />}
            checked={issue.milestone_id === item.id}
            onSelect={() => choose({ milestone_id: item.id })}
          />
        ))}
      </Parent>
      <div role="separator" className="mx-1 my-1 h-px shrink-0 bg-[#2c323c]" />
      <Item
        label="Copy issue ID"
        icon={<Icon name="copy" className="text-[#95a6bd]" />}
        onSelect={() => copy(issue.key, `Copied ${issue.key}`)}
        onHover={() => setOpenSub(undefined)}
      />
      <Item
        label="Copy issue link"
        icon={<Icon name="link" className="text-[#95a6bd]" />}
        onSelect={() => copy(link, "Copied issue link")}
        onHover={() => setOpenSub(undefined)}
      />
    </div>
  );
}

type ItemProps = {
  label: string;
  icon?: ReactNode;
  checked?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
  onHover?: () => void;
};

/** One choice in a menu; a checkmark shows the current value. */
function Item({ label, icon, checked, disabled, onSelect, onHover }: ItemProps) {
  return (
    <button
      type="button"
      role="menuitem"
      data-menu-item
      className={itemClass}
      disabled={disabled}
      onClick={onSelect}
      onMouseEnter={(event) => {
        onHover?.();
        if (!disabled) event.currentTarget.focus({ preventScroll: true });
      }}
    >
      {icon}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {checked && <Icon name="check" className="size-4 text-[#9fb4d9]" />}
    </button>
  );
}

type ParentProps = {
  id: string;
  label: string;
  icon: ReactNode;
  detail?: string;
  openSub: string | undefined;
  setOpenSub: (id: string | undefined) => void;
  children: ReactNode;
};

/** A row that opens a submenu of choices beside it while hovered or focused. */
function Parent({ id, label, icon, detail, openSub, setOpenSub, children }: ParentProps) {
  const open = openSub === id;

  return (
    <div className="relative" onMouseEnter={() => setOpenSub(id)}>
      <button
        type="button"
        role="menuitem"
        aria-haspopup="menu"
        aria-expanded={open}
        data-menu-item
        data-parent={id}
        className={itemClass}
        onClick={() => setOpenSub(id)}
        onFocus={() => setOpenSub(id)}
      >
        {icon}
        <span className="min-w-0 flex-1 truncate">{label}</span>
        {detail && (
          <span className="max-w-[80px] truncate text-[12px] text-[#7f8b9d]">{detail}</span>
        )}
        <Icon name="chevron-right" className="size-4 text-[#7f8b9d]" />
      </button>
      {open && <Submenu>{children}</Submenu>}
    </div>
  );
}

/** Opens to the right of its row, flipping left or sliding up when the viewport is too small. */
function Submenu({ children }: { children: ReactNode }) {
  const element = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const menu = element.current;
    if (!menu) return;

    const edge = 8;
    const bounds = menu.getBoundingClientRect();
    if (bounds.right > window.innerWidth - edge) {
      menu.style.left = "auto";
      menu.style.right = "calc(100% + 2px)";
    }
    if (bounds.bottom > window.innerHeight - edge) {
      menu.style.top = `${-7 - (bounds.bottom - window.innerHeight + edge)}px`;
    }
  }, []);

  return (
    <div
      ref={element}
      role="menu"
      data-sub="true"
      className={`context-submenu absolute -top-2 left-[calc(100%+2px)] max-h-[320px] scrollbar-thin [scrollbar-color:#465062_transparent] overflow-y-auto ${panelClass}`}
    >
      {children}
    </div>
  );
}
