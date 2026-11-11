import { useEffect, type RefObject } from "react";

const edgeSize = 90;
const maxSpeed = 1400;
const smoothing = 0.18;

/** Finds the nearest ancestor that scrolls vertically, falling back to the page. */
function scrollParent(start: HTMLElement): HTMLElement {
  for (let node = start.parentElement; node; node = node.parentElement) {
    const { overflowY } = getComputedStyle(node);
    if ((overflowY === "auto" || overflowY === "scroll") && node.scrollHeight > node.clientHeight) {
      return node;
    }
  }

  return (document.scrollingElement ?? document.documentElement) as HTMLElement;
}

/**
 * Scrolls the page while an issue is dragged near its top or bottom edge. The top edge sits below
 * the sticky toolbar, so dragging up into the header scrolls up. Speed rises with how deep the
 * pointer goes into the edge and eases in and out, so scrolling starts and stops smoothly.
 */
export function useDragAutoscroll(
  active: boolean,
  anchor: RefObject<HTMLElement | null>,
  topInset: number,
): void {
  useEffect(() => {
    if (!active || !anchor.current) return;

    const scroller = scrollParent(anchor.current);
    const isPage = scroller === document.scrollingElement || scroller === document.documentElement;
    let pointerY: number | undefined;
    let velocity = 0;
    let last = performance.now();
    let frame = 0;

    const track = (event: DragEvent) => {
      if (event.clientY !== 0) pointerY = event.clientY;
    };

    const targetVelocity = (): number => {
      if (pointerY === undefined) return 0;

      const bounds = isPage
        ? { top: 0, bottom: window.innerHeight }
        : scroller.getBoundingClientRect();
      const topEdge = bounds.top + topInset + edgeSize;
      const bottomEdge = bounds.bottom - edgeSize;

      if (pointerY < topEdge) {
        const depth = Math.min(1, (topEdge - pointerY) / (edgeSize + topInset));
        return -maxSpeed * depth * depth;
      }
      if (pointerY > bottomEdge) {
        const depth = Math.min(1, (pointerY - bottomEdge) / edgeSize);
        return maxSpeed * depth * depth;
      }

      return 0;
    };

    const step = (now: number) => {
      const seconds = Math.min(0.05, (now - last) / 1000);
      last = now;
      velocity += (targetVelocity() - velocity) * smoothing;
      if (Math.abs(velocity) > 1) scroller.scrollTop += velocity * seconds;
      frame = requestAnimationFrame(step);
    };

    document.addEventListener("dragover", track);
    frame = requestAnimationFrame(step);

    return () => {
      document.removeEventListener("dragover", track);
      cancelAnimationFrame(frame);
    };
  }, [active, anchor, topInset]);
}
