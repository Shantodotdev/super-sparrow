import type { ChecklistItem } from "./types";

// Mirrors server/internal/checklist: a bullet or number, a box, and the item's text. Items inside
// fenced code blocks are examples, not tasks.
const itemPattern = /^(\s*(?:[-*+]|\d{1,9}[.)])\s+\[)([ xX])(\]\s+)(.*)$/;

/** One line of a plan: plain text, or a checklist item. */
export type PlanLine = { kind: "text"; text: string } | { kind: "item"; item: ChecklistItem };

/** Splits a plan into its lines, numbering checklist items from 0 as the server does. */
export function planLines(plan: string): PlanLine[] {
  const lines: PlanLine[] = [];
  let fence = "";
  let index = 0;

  for (const line of plan.split("\n")) {
    const trimmed = line.trim();
    if (fence) {
      if (trimmed.startsWith(fence)) fence = "";
    } else if (trimmed.startsWith("```") || trimmed.startsWith("~~~")) {
      fence = trimmed.slice(0, 3);
    } else {
      const match = itemPattern.exec(line.replace(/\r$/, ""));
      if (match) {
        lines.push({
          kind: "item",
          item: { index: index++, text: match[4]!.trim(), checked: match[2] !== " " },
        });
        continue;
      }
    }
    lines.push({ kind: "text", text: line });
  }

  return lines;
}

/** Returns the plan's checklist items. */
export function checklistItems(plan: string): ChecklistItem[] {
  return planLines(plan).flatMap((line) => (line.kind === "item" ? [line.item] : []));
}

/** Returns the plan with the item at index ticked or unticked, and every other line unchanged. */
export function setChecked(plan: string, index: number, checked: boolean): string {
  let fence = "";
  let current = 0;

  return plan
    .split("\n")
    .map((line) => {
      const trimmed = line.trim();
      if (fence) {
        if (trimmed.startsWith(fence)) fence = "";
        return line;
      }
      if (trimmed.startsWith("```") || trimmed.startsWith("~~~")) {
        fence = trimmed.slice(0, 3);
        return line;
      }

      const match = itemPattern.exec(line);
      if (!match || current++ !== index) return line;
      return `${match[1]}${checked ? "x" : " "}${match[3]}${match[4]}`;
    })
    .join("\n");
}
