import { useEffect } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Navigate, Outlet, useLocation } from "@tanstack/react-router";
import { projectsQuery } from "../queries/project";
import { NotFoundState } from "../components/not-found-state";

const legacyRoutes = {
  findings: "/$projectSlug/findings",
  milestones: "/$projectSlug/milestones",
} as const;

export const Route = createFileRoute("/$projectSlug")({ component: ProjectLayout });

/** Resolves stable project slugs and redirects old ID links without dropping their suffix or filters. */
function ProjectLayout() {
  const { projectSlug } = Route.useParams();
  const location = useLocation();
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const oldProject = projects.find((project) => project.id === projectSlug);
  useEffect(() => {
    if (oldProject?.slug) {
      const suffix = location.pathname.slice(projectSlug.length + 1);
      window.location.replace(
        `/${oldProject.slug}${suffix}${location.searchStr}${location.hash ? `#${location.hash}` : ""}`,
      );
    }
  }, [oldProject, projectSlug, location.pathname, location.searchStr, location.hash]);

  if (oldProject) return <p role="status">Opening project…</p>;
  const legacy = legacyRoutes[projectSlug as keyof typeof legacyRoutes];
  if (legacy && projects[0])
    return <Navigate to={legacy} params={{ projectSlug: projects[0].slug }} replace />;
  if (!projects.some((project) => project.slug === projectSlug))
    return (
      <NotFoundState
        title="Project not found"
        description="This project does not exist or has been deleted."
      />
    );
  return <Outlet />;
}
