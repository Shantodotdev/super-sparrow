import { useEffect, useRef, useState, type MouseEvent, type PointerEvent } from "react";
import type { Issue, IssueStatus } from "../lib/types";

/** How far the pointer must move before a press on an issue becomes a drag rather than a click. */
const dragThreshold = 5;

type Press = { issue: Issue; x: number; y: number; pointerId: number };

type IssueDrag = {
  /** The issue being dragged, once the pointer has moved past the threshold. */
  dragged: Issue | undefined;
  /** The status group under the pointer while dragging. */
  dropTarget: IssueStatus | undefined;
  /** Where the drag started, for placing the preview on its first frame. */
  origin: { x: number; y: number };
  /** Props for a draggable issue row or card. */
  dragProps: (issue: Issue) => {
    draggable: false;
    onPointerDown: (event: PointerEvent<HTMLElement>) => void;
    onClickCapture: (event: MouseEvent<HTMLElement>) => void;
  };
};

/** Reads the status of the drop zone under a point. Drop zones carry `data-drop-status`. */
function statusAt(x: number, y: number): IssueStatus | undefined {
  const zone = document.elementFromPoint(x, y)?.closest<HTMLElement>("[data-drop-status]");
  return zone?.dataset.dropStatus as IssueStatus | undefined;
}

/**
 * Drags issues between status groups with pointer events. It avoids the browser's native drag and
 * drop, which crashes WebKitGTK in the Linux desktop app. A press that moves less than a few
 * pixels stays a click and opens the issue; Escape cancels a drag.
 */
export function useIssueDrag(onDrop: (issue: Issue, status: IssueStatus) => void): IssueDrag {
  const press = useRef<Press | undefined>(undefined);
  const draggingRef = useRef(false);
  const dropTargetRef = useRef<IssueStatus | undefined>(undefined);
  const suppressClick = useRef(false);
  const onDropRef = useRef(onDrop);
  const [dragged, setDragged] = useState<Issue | undefined>();
  const [dropTarget, setDropTarget] = useState<IssueStatus | undefined>();
  const [origin, setOrigin] = useState({ x: 0, y: 0 });

  onDropRef.current = onDrop;

  useEffect(() => {
    const setTarget = (status: IssueStatus | undefined) => {
      if (dropTargetRef.current === status) return;
      dropTargetRef.current = status;
      setDropTarget(status);
    };

    const reset = () => {
      press.current = undefined;
      draggingRef.current = false;
      document.body.style.userSelect = "";
      setTarget(undefined);
      setDragged(undefined);
    };

    const move = (event: globalThis.PointerEvent) => {
      const current = press.current;
      if (!current || event.pointerId !== current.pointerId) return;

      // Start the drag only once the pointer has clearly moved, so plain clicks still open issues.
      if (!draggingRef.current) {
        if (Math.hypot(event.clientX - current.x, event.clientY - current.y) < dragThreshold)
          return;
        draggingRef.current = true;
        document.body.style.userSelect = "none";
        window.getSelection()?.removeAllRanges();
        setOrigin({ x: event.clientX, y: event.clientY });
        setDragged(current.issue);
      }

      setTarget(statusAt(event.clientX, event.clientY));
    };

    const up = (event: globalThis.PointerEvent) => {
      const current = press.current;
      if (!current || event.pointerId !== current.pointerId) return;

      if (draggingRef.current) {
        // The click that follows this release would open the issue; swallow it once.
        suppressClick.current = true;
        window.setTimeout(() => (suppressClick.current = false), 0);
        const status = statusAt(event.clientX, event.clientY);
        if (status && status !== current.issue.status) onDropRef.current(current.issue, status);
      }
      reset();
    };

    const cancel = () => {
      if (press.current) reset();
    };

    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape" && draggingRef.current) reset();
    };

    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", cancel);
    window.addEventListener("blur", cancel);
    window.addEventListener("keydown", key);

    return () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", cancel);
      window.removeEventListener("blur", cancel);
      window.removeEventListener("keydown", key);
      document.body.style.userSelect = "";
    };
  }, []);

  const dragProps = (issue: Issue) => ({
    draggable: false as const,
    onPointerDown: (event: PointerEvent<HTMLElement>) => {
      if (event.button !== 0 || event.pointerType === "touch") return;
      press.current = { issue, x: event.clientX, y: event.clientY, pointerId: event.pointerId };
    },
    onClickCapture: (event: MouseEvent<HTMLElement>) => {
      if (!suppressClick.current) return;
      event.preventDefault();
      event.stopPropagation();
    },
  });

  return { dragged, dropTarget, origin, dragProps };
}
