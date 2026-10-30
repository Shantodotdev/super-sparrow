import { labelColors } from "../lib/label-colors";
import { LabelDot } from "./label-dot";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import { labelsQuery } from "../queries/project";
import { statusIcon, statusIconColor } from "../lib/issue-status";
import { Input, Textarea } from "./input";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Icon } from "./icon";
import { Select } from "./select";
import { Modal } from "./modal";
import type { Project } from "../queries/resources";
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
  project: Project;
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
  project,
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
  const projectKey = project.key;
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [assigneeId, setAssigneeId] = useState("");
  const [milestoneId, setMilestoneId] = useState("");
  const [selectedLabels, setSelectedLabels] = useState<string[]>([]);
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [labelSearch, setLabelSearch] = useState("");
  const [labelDraftName, setLabelDraftName] = useState<string | null>(null);
  const client = useQueryClient();
  const createLabel = useMutation({
    mutationFn: ({ name, color }: { name: string; color: string }) =>
      apiWrite<{ label: Label }>(
        `/api/projects/${encodeURIComponent(project.key)}/labels`,
        "POST",
        { name, color },
      ),
    onSuccess: async ({ label }) => {
      client.setQueryData(labelsQuery.queryKey, (current) => [
        ...(current ?? []).filter((item) => item.id !== label.id),
        label,
      ]);
      setSelectedLabels((current) =>
        current.includes(label.id) ? current : [...current, label.id],
      );
      setLabelSearch("");
      setLabelDraftName(null);
      setLabelsOpen(false);
      labelTrigger.current?.focus();
      await client.invalidateQueries({ queryKey: labelsQuery.queryKey });
    },
  });

  const [createMore, setCreateMore] = useState(false);
  const labelMenu = useRef<HTMLDivElement>(null);
  const labelTrigger = useRef<HTMLButtonElement>(null);
  const colorPicker = useRef<HTMLDivElement>(null);
  const titleInput = useRef<HTMLInputElement>(null);
  const projectLabels = labels.filter((l) => l.project_id === project.id);
  const labelNames = new Map(projectLabels.map((label) => [label.id, label.name]));
  const searchedName = labelSearch.trim();
  const matchingLabels = projectLabels.filter((label) =>
    label.name.toLowerCase().includes(searchedName.toLowerCase()),
  );
  const exactLabel = projectLabels.find(
    (label) => label.name.toLowerCase() === searchedName.toLowerCase(),
  );
  const canCreateLabel = searchedName !== "" && !exactLabel;

  const toggleLabel = (id: string) =>
    setSelectedLabels((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id],
    );
  const addLabel = () => {
    if (createLabel.isPending || pending) return;
    if (exactLabel) {
      toggleLabel(exactLabel.id);
      setLabelSearch("");
    } else if (searchedName) {
      setLabelDraftName(searchedName);
      createLabel.reset();
    }
  };

  useEffect(() => {
    if (!open) return;
    setTitle("");
    setPlan("");
    setStatus("backlog");

    setAssigneeId("");
    setMilestoneId("");
    setSelectedLabels([]);
    setLabelsOpen(false);
    setLabelSearch("");
    setLabelDraftName(null);
    createLabel.reset();
    setCreateMore(false);
  }, [open]);

  useEffect(() => {
    if (labelDraftName) colorPicker.current?.querySelector<HTMLButtonElement>("button")?.focus();
  }, [labelDraftName]);

  useEffect(() => {
    if (!labelsOpen) return;
    const dismiss = (event: PointerEvent) => {
      if (!labelMenu.current?.contains(event.target as Node)) {
        setLabelsOpen(false);
        setLabelDraftName(null);
      }
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [labelsOpen]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!title.trim() || !projectKey || pending || createLabel.isPending || labelDraftName) return;
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
        {error && <p role="alert">{error}</p>}
        <div className="flex items-center gap-[8px] text-[#a5a8b1] [&_>_.icon]:h-4 [&_>_.icon]:w-4 [&_h2]:m-0 [&_h2]:text-[14px] [&_h2]:font-medium [&_h2]:text-[#e7e8ec]">
          <span className="inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] [&_.icon]:h-4 [&_.icon]:w-4">
            <Icon name="folder" /> {project.name}
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
        <Input
          ref={titleInput}
          autoFocus
          required
          aria-label="Issue title"
          className="mt-[26px] block h-auto w-full rounded-none border-0 border-b border-transparent bg-transparent p-0 text-[19px] font-semibold text-[#f1f2f4] placeholder:text-[#777a84]"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder="Issue title"
        />
        <Textarea
          aria-label="Description or plan"
          className="mt-4 block h-auto min-h-[100px] w-full resize-y rounded-none border-0 border-b border-transparent bg-transparent p-0 text-[14px] leading-[1.6] text-[#f1f2f4] placeholder:text-[#777a84]"
          value={plan}
          onChange={(event) => setPlan(event.target.value)}
          placeholder="Add a description or plan..."
        />
        <div className="flex flex-wrap gap-[6px]">
          <Select
            variant="chip"
            label="Status"
            value={status}
            onChange={(value) => setStatus(value as IssueStatus)}
            options={statuses.map((option) => ({
              ...option,
              icon: statusIcon[option.value as IssueStatus],
              iconClassName: statusIconColor[option.value as IssueStatus],
            }))}
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
              ...milestones.filter((m) => m.projectId === project.id),
            ]}
          />
          <div
            className="relative"
            ref={labelMenu}
            onKeyDown={(event) => {
              if (event.key === "Escape" && labelsOpen) {
                event.stopPropagation();
                if (labelDraftName) {
                  setLabelDraftName(null);
                  createLabel.reset();
                } else setLabelsOpen(false);
              }
            }}
          >
            <button
              className="inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] hover:border-[#484b54] hover:bg-[#383b42] aria-expanded:border-[#484b54] aria-expanded:bg-[#383b42] [&_.icon]:h-4 [&_.icon]:w-4"
              type="button"
              ref={labelTrigger}
              aria-label="Labels"
              aria-expanded={labelsOpen}
              aria-haspopup="true"
              onClick={() => {
                setLabelsOpen(!labelsOpen);
                setLabelDraftName(null);
              }}
            >
              <Icon name="list-filter" />{" "}
              {selectedLabels.length ? `Labels · ${selectedLabels.length}` : "Labels"}
            </button>
            {labelsOpen && (
              <div
                className="absolute top-[calc(100%+6px)] left-0 z-30 w-104 max-w-[calc(100vw-40px)] animate-[select-in_120ms_ease-out] overflow-hidden rounded-[12px] border border-[#303744] bg-[#171b21] shadow-[0_12px_28px_#0009,0_2px_8px_#0006] motion-reduce:animate-none"
                aria-label="Choose labels"
              >
                {labelDraftName ? (
                  <div className="p-3" aria-label="Choose label color">
                    <div className="mb-3 flex items-center gap-2 text-sm text-[#c7cfdb]">
                      <button
                        type="button"
                        aria-label="Back to labels"
                        className="rounded border-0 bg-transparent p-1 hover:bg-[#283140]"
                        disabled={createLabel.isPending}
                        onClick={() => {
                          setLabelDraftName(null);
                          createLabel.reset();
                        }}
                      >
                        <Icon name="chevron-right" className="size-4 rotate-180" />
                      </button>
                      <span>Choose a color</span>
                      <span className="ml-auto truncate text-[#95a6bd]">{labelDraftName}</span>
                    </div>
                    <div ref={colorPicker} className="grid grid-cols-4 gap-2">
                      {labelColors.map(({ name, value }) => (
                        <button
                          key={value}
                          type="button"
                          aria-label={name}
                          title={name}
                          className="grid place-items-center gap-1.5 rounded-lg border border-[#303744] bg-[#101216] px-2 py-2.5 text-xs text-[#bfcbdc] transition-colors hover:border-[#6486ff] hover:bg-[#283140] disabled:opacity-50"
                          disabled={createLabel.isPending || pending}
                          onClick={() => createLabel.mutate({ name: labelDraftName, color: value })}
                        >
                          <span
                            className="size-5 rounded-full"
                            style={{ backgroundColor: value }}
                          />
                          {name}
                        </button>
                      ))}
                    </div>
                    {createLabel.error && (
                      <p role="alert" className="mt-3 text-xs text-[#ff9999]">
                        {createLabel.error.message}
                      </p>
                    )}
                  </div>
                ) : (
                  <>
                    <div className="border-b border-[#303744] px-4 py-1">
                      <Input
                        autoFocus
                        aria-label="Search or create labels"
                        placeholder="Search labels…"
                        maxLength={50}
                        className="h-9 rounded-none border-0 bg-transparent p-0 text-sm placeholder:text-[#8999ad]"
                        value={labelSearch}
                        disabled={createLabel.isPending || pending}
                        onChange={(event) => {
                          setLabelSearch(event.target.value);
                          if (createLabel.error) createLabel.reset();
                        }}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") {
                            event.preventDefault();
                            event.stopPropagation();
                            addLabel();
                          }
                        }}
                      />
                    </div>
                    <div className="max-h-[240px] overflow-y-auto p-1.5">
                      {matchingLabels.map(({ id, name, color }) => (
                        <button
                          type="button"
                          key={id}
                          aria-pressed={selectedLabels.includes(id)}
                          className="flex min-h-9 w-full items-center justify-between gap-3 rounded-[5px] border-0 bg-transparent px-[9px] py-2 text-left text-sm text-[#bfcbdc] transition-colors hover:bg-[#283140] focus-visible:bg-[#283140] focus-visible:outline-none"
                          onClick={() => toggleLabel(id)}
                        >
                          <span className="flex min-w-0 items-center gap-2">
                            <LabelDot color={color} />
                            <span className="truncate">{name}</span>
                          </span>
                          {selectedLabels.includes(id) && (
                            <Icon name="check" className="size-4 text-[#7896ff]" />
                          )}
                        </button>
                      ))}
                      {canCreateLabel && (
                        <button
                          type="button"
                          aria-label="Create new label"
                          className="flex min-h-9 w-full items-center gap-2.5 rounded-[5px] border-0 bg-[#283140] px-[9px] py-2 text-left text-sm text-[#f2f5fc] transition-colors hover:bg-[#283140] focus-visible:bg-[#283140] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
                          disabled={createLabel.isPending || pending}
                          onClick={addLabel}
                        >
                          <Icon name="plus" className="size-4 text-[#7896ff]" />
                          <span className="shrink-0">Create new label:</span>
                          <span className="truncate text-[#95a6bd]">
                            &quot;{searchedName}&quot;
                          </span>
                        </button>
                      )}
                      {createLabel.error && (
                        <p role="alert" className="px-3 py-2 text-xs text-[#ff9999]">
                          {createLabel.error.message}
                        </p>
                      )}
                    </div>
                  </>
                )}
              </div>
            )}
          </div>
        </div>
        {selectedLabels.length > 0 && (
          <div className="mt-[10px] flex flex-wrap gap-[5px] [&_button]:inline-flex [&_button]:items-center [&_button]:gap-1.5 [&_button]:rounded-full [&_button]:border [&_button]:border-[#3b424e] [&_button]:bg-[#252b35] [&_button]:px-[8px] [&_button]:py-[3px] [&_button]:text-[12px] [&_button]:text-[#becbe0]">
            {selectedLabels.map((id) => (
              <button
                type="button"
                key={id}
                onClick={() =>
                  setSelectedLabels((current) => current.filter((item) => item !== id))
                }
                aria-label={`Remove ${labelNames.get(id)} label`}
              >
                <LabelDot
                  color={projectLabels.find((label) => label.id === id)?.color ?? "#a7c0ff"}
                />{" "}
                {labelNames.get(id)}{" "}
                <span aria-hidden="true" className="ml-1 text-[#8e9bac]">
                  ×
                </span>
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
            disabled={
              pending || createLabel.isPending || !!labelDraftName || !title.trim() || !projectKey
            }
          >
            Create issue
          </button>
        </div>
      </form>
    </Modal>
  );
}
