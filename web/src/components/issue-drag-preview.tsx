import { useEffect, useRef } from "react";
import type { Issue } from "../lib/types";

type IssueDragPreviewProps = {
  issue: Issue;
  origin: { x: number; y: number };
};

const cursorOffset = 12;

/** Draws the dragged issue as a raised, highlighted capsule that follows the cursor closely. */
export function IssueDragPreview({ issue, origin }: IssueDragPreviewProps) {
  const element = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const place = (x: number, y: number) => {
      if (!element.current) return;
      element.current.style.transform = `translate3d(${x + cursorOffset}px, ${y + cursorOffset}px, 0) scale(1.05)`;
    };
    // dragover reports the live pointer position; some browsers report 0,0 on the final events.
    const follow = (event: globalThis.DragEvent) => {
      if (event.clientX === 0 && event.clientY === 0) return;
      place(event.clientX, event.clientY);
    };

    place(origin.x, origin.y);
    document.addEventListener("dragover", follow);

    return () => document.removeEventListener("dragover", follow);
  }, [origin]);

  return (
    <div
      ref={element}
      aria-hidden="true"
      className="pointer-events-none fixed top-0 left-0 z-50 flex max-w-120 origin-top-left items-center gap-[10px] rounded-[10px] border border-[#5873c7] bg-[#1d2536] px-4 py-[9px] text-[13px] font-medium whitespace-nowrap text-[#f2f5fb] shadow-[0_14px_36px_rgba(0,0,0,0.6),0_0_0_1px_rgba(88,115,199,0.25)] will-change-transform"
    >
      <span className="text-[#9db0d6]">{issue.key}</span>
      <span className="truncate">{issue.title}</span>
    </div>
  );
}
