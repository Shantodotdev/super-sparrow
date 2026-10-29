import { Link } from "@tanstack/react-router";
import { paletteLinks } from "../data/navigation";
import { Modal } from "./modal";

type CommandPaletteProps = { open: boolean; onClose: () => void };

/** A quick-navigation dialog; choosing a destination closes it. */
export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  return (
    <Modal open={open} onClose={onClose} className="palette" label="Navigation">
      <div className="palette-title">
        Go to <kbd>esc</kbd>
      </div>
      {paletteLinks.map((link) => (
        <Link key={link.to} to={link.to} onClick={onClose}>
          {link.label}
          <span>↗</span>
        </Link>
      ))}
    </Modal>
  );
}
