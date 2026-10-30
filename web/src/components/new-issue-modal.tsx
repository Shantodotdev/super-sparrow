import { useEffect, useRef, useState, type FormEvent } from "react";
import { Icon } from "./icon";
import { Select } from "./select";
import { Modal } from "./modal";
import { useSuspenseQuery } from "@tanstack/react-query";
import { projectsQuery } from "../queries/project";
import type { IssueStatus, Label } from "../lib/types";

/** A new issue's fields; assigneeId and milestoneId are "" for none, as the API takes them. */
export type NewIssueValues = {
  title: string;
  plan: string;
  status: IssueStatus;
  projectKey: string;
  assigneeId: string;
  milestoneId: string;
  labelIds: string[];
};

type Option = { value: string; label: string; projectId?: string };

type NewIssueModalProps = {
  statuses: Option[];
  assignees: Option[];
  milestones: Option[];
  labels: Label[];
  onCreate: (values: NewIssueValues) => Promise<unknown>;
  onClose: () => void;
  open: boolean;
};

/** Collects the fields for a new issue to persist through the Go API. */
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
  const [status, setStatus] = useState<IssueStatus>("backlog");
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const [projectKey, setProjectKey] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [assigneeId, setAssigneeId] = useState("");
  const [milestoneId, setMilestoneId] = useState("");
  const [selectedLabels, setSelectedLabels] = useState<string[]>([]);
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [createMore, setCreateMore] = useState(false);
  const labelMenu = useRef<HTMLDivElement>(null);
  const titleInput = useRef<HTMLInputElement>(null);
  const projectLabels = labels.filter(
    (l) => l.project_id === projects.find((p) => p.key === projectKey)?.id,
  );
  const labelNames = new Map(projectLabels.map((label) => [label.id, label.name]));

  useEffect(() => {
    if (!open) return;
    setTitle("");
    setPlan("");
    setStatus("backlog");

    setAssigneeId("");
    setMilestoneId("");
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

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!title.trim() || !projectKey || pending) return;
    setPending(true);
    setError("");
    try {
      await onCreate({
        title: title.trim(),
        plan: plan.trim(),
        status,
        projectKey,
        assigneeId,
        milestoneId,
        labelIds: selectedLabels,
      });
      if (!createMore) {
        onClose();
        return;
      }
      setTitle("");
      setPlan("");
      setStatus("backlog");

      setAssigneeId("");
      setMilestoneId("");
      setSelectedLabels([]);
      setLabelsOpen(false);
      titleInput.current?.focus();
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not create issue");
    } finally {
      setPending(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      className="w-[min(100%,780px)] rounded-[16px] border border-[#343740] bg-[#191a1e] shadow-[0_24px_80px_#000b] [&_form]:px-[18px] [&_form]:py-4 max-[600px]:[&_form]:p-4"
      overlayClassName="items-start pt-[min(15vh,_130px)] max-[600px]:py-[20px] max-[600px]:px-[12px]"
      labelledBy="new-issue-heading"
    >
      <form onSubmit={submit}>
        <label>
          Project
          <select
            required
            value={projectKey}
            onChange={(event) => {
              setProjectKey(event.target.value);
              setAssigneeId("");
              setMilestoneId("");
              setSelectedLabels([]);
            }}
          >
            <option value="">Choose a project</option>
            {projects.map((p) => (
              <option key={p.key} value={p.key}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        {error && <p role="alert">{error}</p>}
        {pending && <p role="status">Saving…</p>}
        <div className="flex items-center gap-[8px] text-[#a5a8b1] [&_>_.icon]:h-4 [&_>_.icon]:w-4 [&_h2]:m-0 [&_h2]:text-[14px] [&_h2]:font-medium [&_h2]:text-[#e7e8ec]">
          <span className="inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] [&_.icon]:h-4 [&_.icon]:w-4">
            <Icon name="folder" /> Sparrow
          </span>
          <Icon name="chevron-right" />
          <h2 id="new-issue-heading">New issue</h2>
          <button
            type="button"
            className="ml-auto grid place-items-center rounded-lg border-0 bg-transparent p-[6px] text-[#9a9da6] hover:bg-[#303238] hover:text-white"
            aria-label="Close"
            onClick={onClose}
          >
            <Icon name="circle-x" />
          </button>
        </div>
        <input
          ref={titleInput}
          autoFocus
          required
          aria-label="Issue title"
          className="mt-[26px] block h-auto w-full rounded-none border-0 bg-transparent p-0 text-[19px] font-semibold text-[#f1f2f4] outline-none placeholder:text-[#777a84] focus-visible:outline-none"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder="Issue title"
        />
        <textarea
          aria-label="Description or plan"
          className="mt-4 block h-auto min-h-[100px] w-full resize-y rounded-none border-0 bg-transparent p-0 text-[14px] leading-[1.6] text-[#f1f2f4] outline-none placeholder:text-[#777a84] focus-visible:outline-none"
          value={plan}
          onChange={(event) => setPlan(event.target.value)}
          placeholder="Add a description or plan..."
        />
        <div className="flex flex-wrap gap-[6px]">
          <Select
            variant="chip"
            icon="circle-dashed"
            label="Status"
            value={status}
            onChange={(value) => setStatus(value as IssueStatus)}
            options={statuses}
          />
          <Select
            variant="chip"
            icon="circle-user-round"
            label="Assignee"
            value={assigneeId}
            onChange={setAssigneeId}
            options={[{ value: "", label: "Assignee" }, ...assignees]}
          />
          <Select
            variant="chip"
            icon="target"
            label="Milestone"
            value={milestoneId}
            onChange={setMilestoneId}
            options={[
              { value: "", label: "Milestone" },
              ...milestones.filter(
                (m) => m.projectId === projects.find((p) => p.key === projectKey)?.id,
              ),
            ]}
          />
          <div
            className="relative"
            ref={labelMenu}
            onKeyDown={(event) => {
              if (event.key === "Escape" && labelsOpen) {
                event.stopPropagation();
                setLabelsOpen(false);
              }
            }}
          >
            <button
              className="inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] hover:border-[#484b54] hover:bg-[#383b42] aria-expanded:border-[#484b54] aria-expanded:bg-[#383b42] [&_.icon]:h-4 [&_.icon]:w-4"
              type="button"
              aria-expanded={labelsOpen}
              aria-haspopup="true"
              onClick={() => setLabelsOpen(!labelsOpen)}
            >
              <Icon name="list-filter" />{" "}
              {selectedLabels.length ? `Labels · ${selectedLabels.length}` : "Labels"}
            </button>
            {labelsOpen && (
              <div
                className="absolute top-[calc(100%+6px)] left-0 z-30 max-h-[240px] w-[190px] overflow-y-auto rounded-[12px] border border-[#303744] bg-[#171b21] p-[5px] shadow-[0_12px_28px_#0009] [&_.icon]:h-4 [&_.icon]:w-4 [&_.icon]:text-[#7896ff] [&_button]:flex [&_button]:w-full [&_button]:items-center [&_button]:justify-between [&_button]:rounded-[6px] [&_button]:border-0 [&_button]:bg-transparent [&_button]:px-[9px] [&_button]:py-[8px] [&_button]:text-left [&_button]:text-[#c7cfdb] [&_button:focus-visible]:bg-[#283140] [&_button:hover]:bg-[#283140]"
                aria-label="Choose labels"
              >
                {projectLabels.map(({ id, name }) => (
                  <button
                    type="button"
                    key={id}
                    aria-pressed={selectedLabels.includes(id)}
                    onClick={() =>
                      setSelectedLabels((current) =>
                        current.includes(id)
                          ? current.filter((item) => item !== id)
                          : [...current, id],
                      )
                    }
                  >
                    <span>{name}</span>
                    {selectedLabels.includes(id) && <Icon name="check" />}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
        {selectedLabels.length > 0 && (
          <div className="mt-[10px] flex flex-wrap gap-[5px] [&_button]:rounded-full [&_button]:border [&_button]:border-[#3b424e] [&_button]:bg-[#252b35] [&_button]:px-[8px] [&_button]:py-[3px] [&_button]:text-[12px] [&_button]:text-[#becbe0] [&_span]:ml-[3px] [&_span]:text-[#8e9bac]">
            {selectedLabels.map((id) => (
              <button
                type="button"
                key={id}
                onClick={() =>
                  setSelectedLabels((current) => current.filter((item) => item !== id))
                }
                aria-label={`Remove ${labelNames.get(id)} label`}
              >
                {labelNames.get(id)} <span aria-hidden="true">×</span>
              </button>
            ))}
          </div>
        )}
        <div className="mt-[22px] flex items-center justify-end gap-4">
          <label className="m-0 flex cursor-pointer items-center gap-2 text-[12px] font-normal text-[#a4a6ae] [&_input]:h-[15px] [&_input]:w-[15px] [&_input]:accent-[#4f73f0]">
            <input
              type="checkbox"
              checked={createMore}
              onChange={(event) => setCreateMore(event.target.checked)}
            />{" "}
            <span>Create more</span>
          </label>
          <button
            className="rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]"
            disabled={pending || !title.trim() || !projectKey}
          >
            Create issue
          </button>
        </div>
      </form>
    </Modal>
  );
}
