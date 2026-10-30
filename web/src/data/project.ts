import type { Label, Milestone } from "../lib/types";

/** The one project the preview shows. */
export const project = { id: "project-sp", key: "SP", name: "Sparrow" };

/** Returns an API timestamp for a time on a date of the preview, such as ("2024-11-06", "11:28"). */
export function at(date: string, time = "09:00"): string {
  return `${date}T${time}:00.000Z`;
}

function label(name: string): Label {
  return { id: `label-${name.toLowerCase()}`, project_id: project.id, name, created_at: at("2024-10-28") };
}

export const seedLabels: Label[] = [
  "accessibility",
  "auth",
  "backend",
  "bug",
  "database",
  "documentation",
  "feature",
  "frontend",
  "Observability",
  "performance",
  "realtime",
  "research",
  "security",
  "UI",
  "UX",
].map(label);

export const milestones = {
  launch: {
    id: "milestone-launch",
    project_id: project.id,
    name: "Launch v1.0",
    position: 1,
    status: "active",
    created_at: at("2024-10-28"),
    updated_at: at("2024-11-01"),
  },
  later: {
    id: "milestone-later",
    project_id: project.id,
    name: "Later",
    position: 2,
    status: "planned",
    created_at: at("2024-10-28"),
    updated_at: at("2024-10-28"),
  },
} satisfies Record<string, Milestone>;

export const seedMilestones: Milestone[] = Object.values(milestones);
