import type { IconName } from "../components/icon";
import type { IssuePriority, IssueStatus } from "./types";

/** Issue statuses in the order the list and board show them. */
export const issueStatuses: IssueStatus[] = [
  "in_review",
  "in_progress",
  "todo",
  "backlog",
  "done",
  "canceled",
];

export const issueStatusLabel: Record<IssueStatus, string> = {
  in_review: "In Review",
  in_progress: "In Progress",
  todo: "Todo",
  backlog: "Backlog",
  done: "Done",
  canceled: "Canceled",
};

export const statusIcon: Record<IssueStatus, IconName> = {
  in_review: "circle-dot",
  in_progress: "circle-dot",
  todo: "circle",
  backlog: "circle-dashed",
  done: "circle-check",
  canceled: "circle-x",
};

export const statusIconColor: Record<IssueStatus, string> = {
  in_review: "text-[#a574ff]",
  in_progress: "text-[#4c7aff]",
  todo: "text-[#cbd5e1]",
  backlog: "text-[#a9b6c8]",
  done: "text-[#44ce91]",
  canceled: "text-[#f08080]",
};

export const issuePriorities: IssuePriority[] = ["urgent", "high", "medium", "low"];

export const issuePriorityLabel: Record<IssuePriority, string> = {
  urgent: "Urgent",
  high: "High",
  medium: "Medium",
  low: "Low",
};
