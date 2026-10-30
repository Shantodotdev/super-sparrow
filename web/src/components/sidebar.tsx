import { useQuery } from "@tanstack/react-query";
import { projectsQuery } from "../queries/project";
import { useState } from "react";
import { useMatches, useParams } from "@tanstack/react-router";
import { Icon } from "./icon";
import { NavItem } from "./nav-item";
import { NewProjectModal } from "./new-project-modal";

/** The main navigation. The highlighted item comes from the `section` in the route's static data. */
export function Sidebar() {
  const { data: projects = [] } = useQuery(projectsQuery);
  const { projectSlug } = useParams({ strict: false });
  const [collapsed, setCollapsed] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);
  const section = useMatches({ select: (matches) => matches.at(-1)?.staticData.section });

  return (
    <>
      <aside className="sticky top-0 left-0 z-1 flex h-dvh w-[274px] shrink-0 basis-[274px] flex-col self-start border-r border-[#20242b] bg-[#07080a] px-[13px] pt-[24px] pb-4 max-[700px]:w-[66px] max-[700px]:basis-[66px] max-[700px]:px-[8px] max-[700px]:py-[20px]">
        <div className="flex items-center justify-center px-[13px] max-[700px]:px-[5px]">
          <img
            src="/logo.png"
            className="h-[64px] w-[64px] flex-none max-[700px]:h-[40px] max-[700px]:w-[40px]"
            alt=""
          />
        </div>
        <nav className="mt-[32px] min-h-0 overflow-y-auto" aria-label="Main navigation">
          <NavItem
            to="/issues"
            icon="circle-user-round"
            label="My issues"
            active={section === "issues" && !projectSlug}
          />
          <NavItem
            to="/activity"
            icon="activity"
            label="Activity"
            active={section === "activity" && !projectSlug}
          />
          <div className="mx-[15px] mt-[25px] mb-[12px] flex items-center justify-between text-[13px] text-[#9ba9be]">
            <span className="max-[700px]:hidden">Projects</span>
            <button
              aria-label="Add project"
              className="border-0 bg-transparent p-1"
              onClick={() => setCreating(true)}
            >
              <Icon name="plus" />
            </button>
          </div>
          {projects.map((project) => {
            const expanded = !collapsed.includes(project.id);
            return (
              <section key={project.id} aria-label={`${project.name} project`}>
                <button
                  className="flex h-[34px] w-full items-center gap-[12px] rounded-[6px] border-0 bg-transparent px-[15px] text-left transition-colors duration-160 hover:bg-[#20252d] max-[700px]:p-[10px] max-[700px]:text-[0px]"
                  aria-expanded={expanded}
                  aria-controls={`project-nav-${project.id}`}
                  onClick={() =>
                    setCollapsed((items) =>
                      expanded ? [...items, project.id] : items.filter((id) => id !== project.id),
                    )
                  }
                >
                  <Icon
                    name="folder"
                    className="project-logo text-[20px] text-[#7890ff] max-[700px]:text-[20px]"
                  />
                  <strong>{project.name}</strong>
                  <Icon
                    name={expanded ? "chevron-down" : "chevron-right"}
                    className="ml-auto text-[#a5b1c2] max-[700px]:hidden"
                  />
                </button>
                {expanded && (
                  <div
                    id={`project-nav-${project.id}`}
                    className="ml-[16px] max-[700px]:ml-0 [&_a]:pl-[20px] max-[700px]:[&_a]:p-0"
                  >
                    <NavItem
                      to="/$projectSlug/issues"
                      params={{ projectSlug: project.slug }}
                      icon="list-todo"
                      label="Issues"
                      active={projectSlug === project.slug && section === "issues"}
                    />
                    <NavItem
                      to="/$projectSlug/milestones"
                      params={{ projectSlug: project.slug }}
                      icon="target"
                      label="Milestones"
                      active={projectSlug === project.slug && section === "milestones"}
                    />
                    <NavItem
                      to="/$projectSlug/findings"
                      params={{ projectSlug: project.slug }}
                      icon="list-filter"
                      label="Open findings"
                      active={projectSlug === project.slug && section === "findings"}
                    />
                  </div>
                )}
              </section>
            );
          })}
        </nav>
        <div className="mt-auto">
          <NavItem
            to="/settings"
            icon="settings"
            label="Settings"
            active={section === "settings"}
          />
        </div>
      </aside>
      <NewProjectModal open={creating} onClose={() => setCreating(false)} />
    </>
  );
}
