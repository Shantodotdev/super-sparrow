import type { Actor, AgentToken } from "../lib/types";
import { at } from "./project";

function person(id: string, name: string, role: Actor["role"]): Actor {
  return {
    id: `person-${id}`,
    kind: "human",
    display_name: name,
    email: `${id}@supersparrow.com`,
    role,
    disabled_at: null,
    created_at: at("2024-11-06"),
  };
}

/** An agent is named after its tool; owner_actor_id says whose agent it is. */
function agent(owner: Actor, agentName: string, name: string): Actor {
  return {
    id: `agent-${owner.id.replace("person-", "")}-${agentName}`,
    kind: "agent",
    display_name: name,
    agent_name: agentName,
    owner_actor_id: owner.id,
    disabled_at: null,
    created_at: at("2024-11-06"),
  };
}

export const people = {
  shanto: person("shanto", "Shanto", "owner"),
  priya: person("priya", "Priya", "member"),
  alex: person("alex", "Alex", "member"),
};

export const agents = {
  shantoClaude: agent(people.shanto, "claude-code", "Claude Code"),
  shantoCodex: agent(people.shanto, "codex", "Codex"),
  alexClaude: agent(people.alex, "claude-code", "Claude Code"),
};

/** The person the preview is signed in as. */
export const currentPerson = people.shanto;

export const seedActors: Actor[] = [...Object.values(people), ...Object.values(agents)];

/** The tools a person can create an agent token for, by agent name. */
export const agentTools = [
  { agentName: "claude-code", label: "Claude Code" },
  { agentName: "codex", label: "Codex" },
];

function token(agentActor: Actor, label: string, hint: string, lastUsed: string | null): AgentToken {
  return {
    id: `token-${agentActor.id}`,
    agent_actor_id: agentActor.id,
    owner_actor_id: agentActor.owner_actor_id!,
    agent_name: agentActor.agent_name!,
    label,
    display_hint: hint,
    last_used_at: lastUsed,
    expires_at: at("2025-02-04"),
    created_at: at("2024-11-06"),
  };
}

export const seedTokens: AgentToken[] = [
  token(agents.shantoClaude, "laptop", "ssp_…4f2a", at("2024-11-08", "11:28")),
  token(agents.shantoCodex, "laptop", "ssp_…9c01", at("2024-11-08", "11:34")),
  token(agents.alexClaude, "desktop", "ssp_…77be", null),
];
