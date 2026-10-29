import { Link } from "@tanstack/react-router";

type NotFoundStateProps = { title: string; description: string };

/** An empty state with a way back to the issue list. */
export function NotFoundState({ title, description }: NotFoundStateProps) {
  return (
    <div className="empty-state">
      <h2>{title}</h2>
      <p>{description}</p>
      <Link to="/issues" className="text-link">
        Back to issues
      </Link>
    </div>
  );
}
