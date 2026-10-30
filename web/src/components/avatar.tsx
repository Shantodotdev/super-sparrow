import type { Actor } from "../lib/types";
import { Icon } from "./icon";

const agentIcons: Record<string, string> = {
  codex: "/brands/codex.png",
  "claude-code": "/brands/claude.svg",
};

/**
 * Shows an agent's tool logo, a person's initials, or an empty figure when nobody is given. The
 * logo comes from the agent's agent_name, so a person named after a tool never gets its logo.
 */
export function Avatar({ actor }: { actor: Actor | undefined }) {
  if (!actor) return <Icon name="circle-user-round" />;

  const agentIcon = actor.kind === "agent" ? agentIcons[actor.agent_name ?? ""] : undefined;
  if (agentIcon) return <img className="agent-icon h-8 w-8" src={agentIcon} alt="" />;

  return (
    <span className="avatar inline-grid h-[36px] w-[36px] flex-[0_0_36px] place-items-center rounded-full bg-[#6941c6] text-[12px] font-semibold text-white">
      {actor.display_name.slice(0, 2).toUpperCase()}
    </span>
  );
}
