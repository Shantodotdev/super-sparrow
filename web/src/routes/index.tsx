import { createFileRoute, Navigate } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { projectsQuery } from "../queries/project";

export const Route = createFileRoute("/")({ component: ProjectHome });

/** Opens the first available project; the shell handles an empty team. */
function ProjectHome() {
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const project = projects[0];
  return project ? (
    <Navigate to="/$projectSlug/issues" params={{ projectSlug: project.slug }} replace />
  ) : null;
}
