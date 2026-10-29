export type NavSection = "issues" | "activity" | "milestones" | "findings" | "settings";

export type IssueStatus = "In Review" | "In Progress" | "Todo" | "Backlog" | "Done" | "Canceled";

export type Issue = {
  key: string;
  title: string;
  status: IssueStatus;
  labels: string[];
  assignee: string;
  findings: number;
  milestone: string;
  updated: string;
  plan?: string;
};

export type FindingSeverity = "Blocking" | "Should fix" | "Nit";

export type Finding = {
  id: string;
  issue: string;
  round: number;
  title: string;
  severity: FindingSeverity;
  author: string;
  status: string;
  problem: string;
  file: string;
};

export type ReviewRound = {
  issue: string;
  number: number;
  from: string;
  to: string;
  date: string;
  author: string;
  summary: string;
};

export type ActivityEvent = {
  time: string;
  actor: string;
  action: string;
  issue: string;
  detail: string;
};

export type ActivityDay = {
  label: string;
  events: ActivityEvent[];
};

export type Milestone = {
  name: string;
  state: "Active" | "Planned";
  issueCount: number;
  summary?: string;
};

export type AgentName = "Claude Code" | "Codex";

export type TeamMember = {
  name: string;
  email: string;
  role: "Owner" | "Member";
  agents: AgentName[];
  joined: string;
};

export type AgentToken = {
  id: string;
  label: string;
  owner: string;
  agent: AgentName;
  expires: string;
  lastUsed: string;
};
