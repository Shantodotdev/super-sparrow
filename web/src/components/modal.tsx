import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";

type ModalProps = {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  className?: string;
  overlayClassName?: string;
  label?: string;
  labelledBy?: string;
};

const exitDuration = 210;

/** Presents a dialog with animated transitions and restores focus when it closes. */
export function Modal({
  open,
  onClose,
  children,
  className = "",
  overlayClassName = "",
  label,
  labelledBy,
}: ModalProps) {
  const [present, setPresent] = useState(open);
  const dialog = useRef<HTMLDivElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);

  useLayoutEffect(() => {
    if (open) {
      if (!present) previousFocus.current = document.activeElement as HTMLElement | null;
      setPresent(true);
      return;
    }
    if (!present) return;
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const timer = window.setTimeout(
      () => {
        setPresent(false);
        previousFocus.current?.focus();
        previousFocus.current = null;
      },
      reducedMotion ? 0 : exitDuration,
    );
    return () => window.clearTimeout(timer);
  }, [open, present]);

  useEffect(() => {
    if (!open || !present) return;
    if (dialog.current?.contains(document.activeElement)) return;
    const target = dialog.current?.querySelector<HTMLElement>("[autofocus]") ?? dialog.current;
    target?.focus();
  }, [open, present]);

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      onClose();
    }
    if (event.key !== "Tab") return;
    const controls = Array.from(
      dialog.current?.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
      ) ?? [],
    ).filter((element) => element.getClientRects().length > 0);
    if (!controls.length) {
      event.preventDefault();
      dialog.current?.focus();
      return;
    }
    const first = controls[0]!;
    const last = controls[controls.length - 1]!;
    if (
      event.shiftKey &&
      (document.activeElement === first || document.activeElement === dialog.current)
    ) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  if (!present) return null;
  return (
    <div
      className={`overlay modal-overlay ${open ? "is-entering" : "is-exiting"} ${overlayClassName}`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && open) onClose();
      }}
    >
      <div
        ref={dialog}
        className={`modal-surface ${className}`}
        role="dialog"
        aria-modal="true"
        aria-label={label}
        aria-labelledby={labelledBy}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
      >
        {children}
      </div>
    </div>
  );
}
