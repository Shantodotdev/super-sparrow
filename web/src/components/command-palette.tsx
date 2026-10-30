import { useQuery } from "@tanstack/react-query";
import { projectsQuery } from "../queries/project";
import { Link, useParams } from "@tanstack/react-router";
import { paletteLinks } from "../data/navigation";
import { Modal } from "./modal";

type CommandPaletteProps = { open: boolean; onClose: () => void };

/** A quick-navigation dialog; choosing a destination closes it. */
export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const { projectSlug } = useParams({ strict: false });
  const { data: projects } = useQuery(projectsQuery);
  const selectedSlug = projectSlug ?? projects?.[0]?.slug;
  return (
    <Modal
      open={open}
      onClose={onClose}
      className="w-[min(100%,510px)] rounded-[10px] border border-[#303641] bg-[#12151a] p-[16px] shadow-[0_20px_70px_#000a] [&_a]:flex [&_a]:w-full [&_a]:justify-between [&_a]:rounded-[5px] [&_a]:border-0 [&_a]:bg-none [&_a]:p-[12px] [&_a]:text-left [&_a_span]:text-[#8290a5] [&_a:hover]:bg-[#202631]"
      label="Navigation"
    >
      <div className="flex justify-between p-[10px] text-[#94a1b3]">
        Go to <kbd>esc</kbd>
      </div>
      {paletteLinks.map((link) => (
        <Link key={link.to} to={link.to} params={{ projectSlug: selectedSlug! }} onClick={onClose}>
          {link.label}
          <span>↗</span>
        </Link>
      ))}
    </Modal>
  );
}
