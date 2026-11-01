import { useLayoutEffect, useRef, type HTMLAttributes, type RefObject } from "react";

type FloatingMenuProps = HTMLAttributes<HTMLDivElement> & {
  anchor: RefObject<HTMLElement | null>;
  minWidth: number;
  onDismiss: () => void;
};

/** Places a menu in the browser's top layer so modal overflow and stacking cannot clip it. */
export function FloatingMenu({
  anchor,
  minWidth,
  onDismiss,
  className = "",
  children,
  ...props
}: FloatingMenuProps) {
  const menu = useRef<HTMLDivElement>(null);
  const dismiss = useRef(onDismiss);

  useLayoutEffect(() => {
    dismiss.current = onDismiss;
  }, [onDismiss]);

  useLayoutEffect(() => {
    const element = menu.current;
    const trigger = anchor.current;
    if (!element || !trigger) return;

    const close = () => {
      if (element.matches(":popover-open")) element.hidePopover();
      dismiss.current();
    };

    const position = () => {
      const rect = trigger.getBoundingClientRect();
      const viewport = window.visualViewport;
      const viewportLeft = viewport?.offsetLeft ?? 0;
      const viewportTop = viewport?.offsetTop ?? 0;
      const width = viewport?.width ?? window.innerWidth;
      const height = viewport?.height ?? window.innerHeight;
      const edge = 12;
      const gap = 6;
      if (
        width <= edge * 2 ||
        height <= edge * 2 ||
        rect.width === 0 ||
        rect.height === 0 ||
        rect.bottom <= viewportTop ||
        rect.top >= viewportTop + height ||
        rect.right <= viewportLeft ||
        rect.left >= viewportLeft + width
      ) {
        close();
        return;
      }

      const availableWidth = Math.max(0, width - edge * 2);
      element.style.minWidth = `${Math.min(Math.max(rect.width, minWidth), availableWidth)}px`;
      element.style.maxWidth = `${availableWidth}px`;

      const below = Math.max(0, viewportTop + height - edge - rect.bottom - gap);
      const above = Math.max(0, rect.top - gap - viewportTop - edge);
      const desiredHeight = Math.min(element.scrollHeight + 2, 260);
      const opensAbove = desiredHeight > below && above > below;
      element.style.maxHeight = `${Math.min(260, opensAbove ? above : below)}px`;

      const bounds = element.getBoundingClientRect();
      element.style.left = `${Math.max(viewportLeft + edge, Math.min(rect.left, viewportLeft + width - edge - bounds.width))}px`;
      const top = opensAbove ? rect.top - gap - bounds.height : rect.bottom + gap;
      element.style.top = `${Math.max(viewportTop + edge, Math.min(top, viewportTop + height - edge - bounds.height))}px`;
    };

    element.showPopover();
    position();
    element.querySelector<HTMLElement>("input, [autofocus]")?.focus({ preventScroll: true });
    const observer = new ResizeObserver(position);
    observer.observe(trigger);
    observer.observe(element);
    const visibility = new IntersectionObserver(([entry]) => {
      if (entry && !entry.isIntersecting) close();
    });
    visibility.observe(trigger);
    window.addEventListener("resize", position);
    document.addEventListener("scroll", position, true);
    document.addEventListener("transitionend", position, true);
    window.visualViewport?.addEventListener("resize", position);
    window.visualViewport?.addEventListener("scroll", position);

    return () => {
      observer.disconnect();
      visibility.disconnect();
      window.removeEventListener("resize", position);
      document.removeEventListener("scroll", position, true);
      document.removeEventListener("transitionend", position, true);
      window.visualViewport?.removeEventListener("resize", position);
      window.visualViewport?.removeEventListener("scroll", position);
      if (element.matches(":popover-open")) element.hidePopover();
    };
  }, [anchor, minWidth]);

  return (
    <div
      {...props}
      ref={menu}
      popover="manual"
      className={`fixed inset-auto m-0 overflow-y-auto ${className}`}
    >
      {children}
    </div>
  );
}
