import { Icon } from "./icon";

const agentIcons: Record<string, string> = {
  Codex: "/brands/codex.png",
  "Claude Code": "/brands/claude.svg",
};

/** Shows the brand mark of an agent, or initials for a person. */
export function Avatar({ name }: { name: string }) {
  if (name === "Unassigned") return <Icon name="circle-user-round" />;

  const agentIcon = agentIcons[name];
  if (agentIcon) return <img className="agent-icon h-8 w-8" src={agentIcon} alt="" />;

  return (
    <span className="avatar inline-grid h-[36px] w-[36px] flex-[0_0_36px] place-items-center rounded-full bg-[#6941c6] text-[12px] font-semibold text-white">
      {name.slice(0, 2).toUpperCase()}
    </span>
  );
}
