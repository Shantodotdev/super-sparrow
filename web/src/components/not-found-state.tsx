import { Link } from "@tanstack/react-router";

type NotFoundStateProps = { title: string; description: string };

/** An empty state with a way back to the issue list. */
export function NotFoundState({ title, description }: NotFoundStateProps) {
  return (
    <div className="mx-auto my-40 max-w-120 p-[24px] text-center text-[#93a0b2] [&_h2]:mx-0 [&_h2]:mt-[18px] [&_h2]:mb-2 [&_h2]:text-[#e9edf5]">
      <h2>{title}</h2>
      <p>{description}</p>
      <Link to="/issues" className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]">
        Back to issues
      </Link>
    </div>
  );
}
