import type { Actor } from "../lib/types";
import { useActor } from "../queries/project";
import { Avatar } from "./avatar";

/** Describes an actor in full, such as "Shanto's Claude Code" for an agent. */
export function useActorTitle() {
  const actor = useActor();

  return (subject: Actor | undefined): string => {
    if (!subject) return "Unassigned";
    const owner = subject.kind === "agent" ? actor(subject.owner_actor_id ?? "") : undefined;
    return owner ? `${owner.display_name}'s ${subject.display_name}` : subject.display_name;
  };
}

/**
 * Shows a person or agent as an avatar followed by their name. An agent's owner is in its
 * tooltip, so two people's Codex agents can be told apart.
 */
export function Person({ actorId }: { actorId: string }) {
  const actor = useActor();
  const title = useActorTitle();
  const subject = actor(actorId);

  return (
    <span
      className="person-inline inline-flex items-center gap-2 whitespace-nowrap [&_.agent-icon]:h-[26px] [&_.agent-icon]:w-[26px] [&_.avatar]:h-[26px] [&_.avatar]:w-[26px] [&_.avatar]:basis-[26px] [&_.avatar]:text-[9px]"
      title={title(subject)}
    >
      <Avatar actor={subject} />
      {subject?.display_name ?? "Unassigned"}
    </span>
  );
}
