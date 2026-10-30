import { createFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createFileRoute("/$projectSlug/")({ component: ProjectIndex });

/** Opens a project's issue list when its base URL is visited. */
function ProjectIndex() {
  const { projectSlug } = Route.useParams();
  return <Navigate to="/$projectSlug/issues" params={{ projectSlug }} replace />;
}
