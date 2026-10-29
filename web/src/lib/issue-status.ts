import type { IconName } from "../components/icon";
import type { IssueStatus } from "./types";

export const issueStatuses: IssueStatus[] = [
  "In Review",
  "In Progress",
  "Todo",
  "Backlog",
  "Done",
  "Canceled",
];

export const statusIcon: Record<IssueStatus, IconName> = {
  "In Review": "circle-dot",
  "In Progress": "circle-dot",
  Todo: "circle",
  Backlog: "circle-dashed",
  Done: "circle-check",
  Canceled: "circle-x",
};
