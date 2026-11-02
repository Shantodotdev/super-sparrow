import { reviewActorName } from "../lib/review";
import { useActor } from "../queries/project";
import { Avatar } from "./avatar";

/** Shows the reviewer's tool or human name without an owner's name in the visible label. */
export function ReviewPerson({ actorId }: { actorId: string }) {
  const actor = useActor()(actorId);

  return (
    <span
      title={actor?.kind === "agent" ? "AI agent" : undefined}
      className="inline-flex items-center gap-2 [&_.agent-icon]:size-5 [&_.avatar]:size-5 [&_.avatar]:basis-5 [&_.avatar]:text-[9px]"
    >
      <Avatar actor={actor} />
      <span>{reviewActorName(actor)}</span>
    </span>
  );
}
