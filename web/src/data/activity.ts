import type { ActivityDay } from "../lib/types";

export const activityActors = ["Shanto", "Claude Code", "Codex"];

export const seedActivity: ActivityDay[] = [
  {
    label: "Today",
    events: [
      {
        time: "11:28 AM",
        actor: "Claude Code",
        action: "started a code review on",
        issue: "SP-142",
        detail: "Reviewing the sign-in flow and its edge cases.",
      },
      {
        time: "11:31 AM",
        actor: "Claude Code",
        action: "added a finding to",
        issue: "SP-142",
        detail: "Missing validation for branch name.",
      },
      {
        time: "11:34 AM",
        actor: "Codex",
        action: "added a finding to",
        issue: "SP-142",
        detail: "Session can expire during sign-in.",
      },
      {
        time: "11:37 AM",
        actor: "Shanto",
        action: "decided to fix a finding on",
        issue: "SP-142",
        detail: "Let's fix this and add tests.",
      },
      {
        time: "11:52 AM",
        actor: "Codex",
        action: "marked a finding fixed on",
        issue: "SP-142",
        detail: "Implemented the fix and added tests.",
      },
    ],
  },
  {
    label: "Yesterday",
    events: [
      {
        time: "4:18 PM",
        actor: "Shanto",
        action: "created",
        issue: "SP-142",
        detail: "Added the initial plan.",
      },
    ],
  },
];
