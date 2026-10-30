import { Icon } from "./icon";
import { Modal } from "./modal";
import { ProjectForm } from "./project-form";

type NewProjectModalProps = { open: boolean; onClose: () => void };

/** Presents project creation in the shared dialog and closes after a successful save. */
export function NewProjectModal({ open, onClose }: NewProjectModalProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy="new-project-heading"
      className="w-[min(100%,520px)] rounded-2xl border border-[#303641] bg-[#10141c] p-7 shadow-[0_24px_80px_#0009]"
    >
      <div className="mb-4 flex items-center justify-between gap-4">
        <h2 id="new-project-heading">New project</h2>
        <button
          type="button"
          aria-label="Close"
          onClick={onClose}
          className="grid place-items-center rounded-lg border-0 bg-transparent p-2 text-[#9ba9be] hover:bg-[#20252d] hover:text-white"
        >
          <Icon name="circle-x" />
        </button>
      </div>
      <ProjectForm onCreated={onClose} />
    </Modal>
  );
}
