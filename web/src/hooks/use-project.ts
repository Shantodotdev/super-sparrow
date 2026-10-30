import { useParams } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { projectsQuery } from "../queries/project";

/** Resolves the project named by the current route, never a query-string selection. */
export function useProject() {
  const { projectSlug } = useParams({ strict: false });
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const project = projects.find((item) => item.slug === projectSlug);
  if (!project) throw new Error("Project not found");
  return project;
}

/** Resolves issue project IDs to URL slugs, retaining old ID links for stale cache entries. */
export function useProjectSlugs() {
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const slugs = new Map(projects.map((project) => [project.id, project.slug]));
  return (id: string) => slugs.get(id) ?? id;
}
