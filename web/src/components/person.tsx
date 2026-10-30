import { Avatar } from "./avatar";

/** Shows a person or agent as an avatar followed by their name. */
export function Person({ name }: { name: string }) {
  return (
    <span className="person-inline inline-flex items-center gap-2 whitespace-nowrap [&_.agent-icon]:h-[26px] [&_.agent-icon]:w-[26px] [&_.avatar]:h-[26px] [&_.avatar]:w-[26px] [&_.avatar]:basis-[26px] [&_.avatar]:text-[9px]">
      <Avatar name={name} />
      {name}
    </span>
  );
}
