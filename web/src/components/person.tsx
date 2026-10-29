import { Avatar } from "./avatar";

/** Shows a person or agent as an avatar followed by their name. */
export function Person({ name }: { name: string }) {
  return (
    <span className="person-inline">
      <Avatar name={name} />
      {name}
    </span>
  );
}
