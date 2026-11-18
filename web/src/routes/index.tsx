import { createFileRoute, Navigate } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { readLastProject } from "../lib/last-project";
import { projectsQuery } from "../queries/project";

export const Route = createFileRoute("/")({ component: ProjectHome });

/** Opens the project last visited, or the first one; the shell handles an empty team. */
function ProjectHome() {
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const lastSlug = readLastProject();
  const project = projects.find((item) => item.slug === lastSlug) ?? projects[0];
  return project ? (
    <Navigate to="/$projectSlug/issues" params={{ projectSlug: project.slug }} replace />
  ) : null;
}
