import type { Milestone } from "../lib/types";

export const seedMilestones: Milestone[] = [
  {
    name: "Launch v1.0",
    state: "Active",
    issueCount: 9,
    summary: "The core Super Sparrow workflow, from planning through review.",
  },
  { name: "Later", state: "Planned", issueCount: 1 },
];
