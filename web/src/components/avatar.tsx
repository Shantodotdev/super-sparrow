import { Icon } from "./icon";

const agentIcons: Record<string, string> = {
  Codex: "/brands/codex.png",
  "Claude Code": "/brands/claude.svg",
};

/** Shows the brand mark of an agent, or initials for a person. */
export function Avatar({ name }: { name: string }) {
  if (name === "Unassigned") return <Icon name="circle-user-round" />;

  const agentIcon = agentIcons[name];
  if (agentIcon) return <img className="agent-icon" src={agentIcon} alt="" />;

  return <span className="avatar">{name.slice(0, 2).toUpperCase()}</span>;
}
