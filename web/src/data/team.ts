import type { AgentName, AgentToken, TeamMember } from "../lib/types";

export const agentNames: AgentName[] = ["Claude Code", "Codex"];

export const seedMembers: TeamMember[] = [
  {
    name: "Shanto",
    email: "shanto@supersparrow.com",
    role: "Owner",
    agents: ["Claude Code", "Codex"],
    joined: "Nov 6, 2024",
  },
  {
    name: "Alex",
    email: "alex@supersparrow.com",
    role: "Member",
    agents: ["Claude Code"],
    joined: "Nov 6, 2024",
  },
  {
    name: "Jamie",
    email: "jamie@supersparrow.com",
    role: "Member",
    agents: ["Codex"],
    joined: "Nov 6, 2024",
  },
];

export const seedTokens: AgentToken[] = [
  {
    id: "token-1",
    label: "Claude Code · laptop",
    owner: "Shanto",
    agent: "Claude Code",
    expires: "Dec 5, 2024",
    lastUsed: "Nov 6, 2024",
  },
  {
    id: "token-2",
    label: "Codex · laptop",
    owner: "Shanto",
    agent: "Codex",
    expires: "Dec 5, 2024",
    lastUsed: "Nov 6, 2024",
  },
];
