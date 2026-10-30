import { Textarea } from "./input";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import type { ReactNode } from "react";
import { planLines, type PlanLine } from "../lib/checklist";
import type { ChecklistItem, Issue } from "../lib/types";
import { useSetChecklistItem } from "../queries/issues";
import { Icon, type IconName } from "./icon";

type Section = { heading: string; lines: PlanLine[] };

const headingIcons: Record<string, [IconName, string]> = {
  goal: ["target", "issue-icon-goal"],
  approach: ["list", "issue-icon-approach"],
  "edge cases": ["triangle-alert", "issue-icon-warning"],
  checklist: ["square-check", "issue-icon-checklist"],
};

/** Splits plan lines into sections at each "## " heading; lines before the first have none. */
function sections(lines: PlanLine[]): Section[] {
  const result: Section[] = [{ heading: "", lines: [] }];
  for (const line of lines) {
    const heading = line.kind === "text" ? /^##\s+(.+)$/.exec(line.text) : null;
    if (heading) result.push({ heading: heading[1]!.trim(), lines: [] });
    else result.at(-1)!.lines.push(line);
  }
  return result.filter((section) => section.heading || section.lines.some(hasContent));
}

function hasContent(line: PlanLine): boolean {
  return line.kind === "item" || line.text.trim() !== "";
}

/**
 * Shows an issue's plan, with its checklist items as checkboxes. The plan is shown as text, not
 * rendered as HTML, so nothing in it can run; "## " headings become section titles.
 */
export function IssuePlan({ issue }: { issue: Issue }) {
  const client = useQueryClient();
  const [draft, setDraft] = useState<{ plan: string; revision: number } | null>(null);
  const save = useMutation({
    mutationFn: () =>
      apiWrite(`/api/issues/${issue.key}`, "PATCH", {
        plan: draft?.plan,
        plan_revision: draft?.revision,
      }),
    onSuccess: async () => {
      setDraft(null);
      await client.invalidateQueries();
    },
  });
  const editor = draft ? (
    <form
      className="grid gap-3 py-4"
      onSubmit={(event) => {
        event.preventDefault();
        save.mutate();
      }}
    >
      <label>
        Plan
        <Textarea
          aria-label="Edit plan"
          rows={14}
          value={draft.plan}
          onChange={(event) => setDraft({ ...draft, plan: event.target.value })}
        />
      </label>
      {save.error && <p role="alert">{save.error.message}</p>}
      <button disabled={save.isPending}>Save plan</button>
      <button type="button" onClick={() => setDraft(null)}>
        Cancel
      </button>
    </form>
  ) : (
    <button
      className="mt-4"
      onClick={() => {
        save.reset();
        setDraft({ plan: issue.plan ?? "", revision: issue.plan_revision });
      }}
    >
      Edit plan
    </button>
  );
  const setChecklistItem = useSetChecklistItem();
  const plan = issue.plan ?? "";

  if (!plan.trim()) {
    return (
      <>
        {editor}
        <p className="mt-[24px] text-[#a2aec2]">No plan added yet.</p>
      </>
    );
  }

  const renderLines = (lines: PlanLine[]): ReactNode[] =>
    lines.map((line, position) =>
      line.kind === "item" ? (
        <ChecklistLine
          key={`item-${line.item.index}`}
          item={line.item}
          onChange={(checked) => setChecklistItem(issue, line.item, checked)}
        />
      ) : (
        <p key={`text-${position}`} className="whitespace-pre-wrap">
          {line.text}
        </p>
      ),
    );

  return (
    <div className="max-w-[850px] [&_h2]:mb-[8px] [&_p]:leading-[1.55] [&_p]:text-[#b4c1d3] [&_section]:mt-[24px] [&_section]:grid [&_section]:grid-cols-[38px_1fr] [&_section]:gap-[10px]">
      {editor}
      {sections(planLines(plan)).map((section, position) => {
        const [icon, color] = headingIcons[section.heading.toLowerCase()] ?? [
          "file-text",
          "issue-icon-review",
        ];
        return (
          <section key={`${section.heading}-${position}`}>
            {section.heading ? (
              <Icon name={icon} className={`plan-icon ${color} text-[24px]`} />
            ) : (
              <span />
            )}
            <div>
              {section.heading && <h2>{section.heading}</h2>}
              {renderLines(section.lines.filter(hasContent))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function ChecklistLine({
  item,
  onChange,
}: {
  item: ChecklistItem;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="mx-0 my-[6px] flex items-center gap-[9px] font-normal text-[#b4c1d3] [&_input]:h-[15px] [&_input]:w-[15px] [&_input]:accent-[#4c75ff]">
      <input
        type="checkbox"
        checked={item.checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      {item.text}
    </label>
  );
}
