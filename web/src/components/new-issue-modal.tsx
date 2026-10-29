import { useEffect, useRef, useState, type FormEvent } from "react";
import { Icon } from "./icon";
import { Select } from "./select";
import { Modal } from "./modal";

export type NewIssueValues = {
  title: string;
  plan: string;
  status: string;
  assignee: string;
  milestone: string;
  labels: string[];
};

type NewIssueModalProps = {
  statuses: string[];
  assignees: string[];
  milestones: string[];
  labels: string[];
  onCreate: (values: NewIssueValues) => void;
  onClose: () => void;
  open: boolean;
};

/** Collects the fields for a new issue in the local frontend preview. */
export function NewIssueModal({
  statuses,
  assignees,
  milestones,
  labels,
  onCreate,
  onClose,
  open,
}: NewIssueModalProps) {
  const [title, setTitle] = useState("");
  const [plan, setPlan] = useState("");
  const [status, setStatus] = useState("Backlog");
  const [assignee, setAssignee] = useState("Unassigned");
  const [milestone, setMilestone] = useState("No milestone");
  const [selectedLabels, setSelectedLabels] = useState<string[]>([]);
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [createMore, setCreateMore] = useState(false);
  const labelMenu = useRef<HTMLDivElement>(null);
  const titleInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setTitle("");
    setPlan("");
    setStatus("Backlog");
    setAssignee("Unassigned");
    setMilestone("No milestone");
    setSelectedLabels([]);
    setLabelsOpen(false);
    setCreateMore(false);
  }, [open]);

  useEffect(() => {
    if (!labelsOpen) return;
    const dismiss = (event: PointerEvent) => {
      if (!labelMenu.current?.contains(event.target as Node)) setLabelsOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [labelsOpen]);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!title.trim()) return;
    onCreate({
      title: title.trim(),
      plan: plan.trim(),
      status,
      assignee,
      milestone,
      labels: selectedLabels,
    });
    if (!createMore) {
      onClose();
      return;
    }
    setTitle("");
    setPlan("");
    setStatus("Backlog");
    setAssignee("Unassigned");
    setMilestone("No milestone");
    setSelectedLabels([]);
    setLabelsOpen(false);
    titleInput.current?.focus();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      className="new-issue-modal"
      overlayClassName="new-issue-overlay"
      labelledBy="new-issue-heading"
    >
      <form onSubmit={submit}>
        <div className="new-issue-header">
          <span className="new-issue-project">
            <Icon name="folder" /> Sparrow
          </span>
          <Icon name="chevron-right" />
          <h2 id="new-issue-heading">New issue</h2>
          <button type="button" className="new-issue-close" aria-label="Close" onClick={onClose}>
            <Icon name="circle-x" />
          </button>
        </div>
        <input
          ref={titleInput}
          autoFocus
          required
          aria-label="Issue title"
          className="new-issue-title"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder="Issue title"
        />
        <textarea
          aria-label="Description or plan"
          className="new-issue-plan"
          value={plan}
          onChange={(event) => setPlan(event.target.value)}
          placeholder="Add a description or plan..."
        />
        <div className="new-issue-properties">
          <Select
            variant="chip"
            icon="circle-dashed"
            label="Status"
            value={status}
            onChange={setStatus}
            options={statuses.map((item) => ({ value: item, label: item }))}
          />
          <Select
            variant="chip"
            icon="circle-user-round"
            label="Assignee"
            value={assignee}
            onChange={setAssignee}
            options={["Unassigned", ...assignees].map((item) => ({
              value: item,
              label: item === "Unassigned" ? "Assignee" : item,
            }))}
          />
          <Select
            variant="chip"
            icon="target"
            label="Milestone"
            value={milestone}
            onChange={setMilestone}
            options={["No milestone", ...milestones].map((item) => ({
              value: item,
              label: item === "No milestone" ? "Milestone" : item,
            }))}
          />
          <div
            className="new-issue-labels"
            ref={labelMenu}
            onKeyDown={(event) => {
              if (event.key === "Escape" && labelsOpen) {
                event.stopPropagation();
                setLabelsOpen(false);
              }
            }}
          >
            <button
              className="new-issue-chip"
              type="button"
              aria-expanded={labelsOpen}
              aria-haspopup="true"
              onClick={() => setLabelsOpen(!labelsOpen)}
            >
              <Icon name="list-filter" />{" "}
              {selectedLabels.length ? `Labels · ${selectedLabels.length}` : "Labels"}
            </button>
            {labelsOpen && (
              <div className="new-issue-label-menu" aria-label="Choose labels">
                {labels.map((label) => (
                  <button
                    type="button"
                    key={label}
                    aria-pressed={selectedLabels.includes(label)}
                    onClick={() =>
                      setSelectedLabels((current) =>
                        current.includes(label)
                          ? current.filter((item) => item !== label)
                          : [...current, label],
                      )
                    }
                  >
                    <span>{label}</span>
                    {selectedLabels.includes(label) && <Icon name="check" />}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
        {selectedLabels.length > 0 && (
          <div className="new-issue-selected-labels">
            {selectedLabels.map((label) => (
              <button
                type="button"
                key={label}
                onClick={() =>
                  setSelectedLabels((current) => current.filter((item) => item !== label))
                }
                aria-label={`Remove ${label} label`}
              >
                {label} <span aria-hidden="true">×</span>
              </button>
            ))}
          </div>
        )}
        <div className="new-issue-footer">
          <label className="new-issue-create-more">
            <input
              type="checkbox"
              checked={createMore}
              onChange={(event) => setCreateMore(event.target.checked)}
            />{" "}
            <span>Create more</span>
          </label>
          <button className="primary" disabled={!title.trim()}>
            Create issue
          </button>
        </div>
      </form>
    </Modal>
  );
}
