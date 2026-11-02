import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Filter, all } from "../components/filter";
import { Icon } from "../components/icon";
import { Input } from "../components/input";
import { MilestoneCard } from "../components/milestone-card";
import { MilestoneFormModal } from "../components/milestone-form-modal";
import { useProject } from "../hooks/use-project";
import {
  milestoneProgressById,
  milestoneStatuses,
  milestoneStatusLabel,
  validateMilestonesSearch,
  type MilestonesSearch,
} from "../lib/milestones";
import { projectsQuery } from "../queries/project";
import { projectIssuesQuery, projectMilestonesQuery } from "../queries/project-resources";
import { loadQuery } from "../queries/options";

export const Route = createFileRoute("/$projectSlug/milestones")({
  staticData: { section: "milestones" },
  validateSearch: validateMilestonesSearch,
  loader: async ({ context, params }) => {
    const projects = await loadQuery(context.queryClient, projectsQuery);
    const project = projects.find((item) => item.slug === params.projectSlug);
    if (!project) return;
    void context.queryClient.prefetchQuery(projectMilestonesQuery(project.key));
    void context.queryClient.prefetchQuery(projectIssuesQuery(project.key));
  },
  component: MilestonesPage,
});

/** Shows searchable project milestones with live issue progress and persisted milestone edits. */
function MilestonesPage() {
  const project = useProject();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const milestones = useQuery(projectMilestonesQuery(project.key));
  const issues = useQuery(projectIssuesQuery(project.key));
  const [form, setForm] = useState<{ id?: string; open: boolean; key: number }>({
    open: false,
    key: 0,
  });
  const projectMilestones = (milestones.data ?? [])
    .filter((milestone) => milestone.project_id === project.id)
    .sort((a, b) => a.position - b.position);
  const query = search.q?.trim().toLocaleLowerCase() ?? "";
  const filtered = projectMilestones.filter(
    (milestone) =>
      (!search.status || milestone.status === search.status) &&
      milestone.name.toLocaleLowerCase().includes(query),
  );
  const progress = issues.data
    ? milestoneProgressById(issues.data.filter((issue) => issue.project_id === project.id))
    : undefined;
  const setFilter = (name: keyof MilestonesSearch, value: string) =>
    void navigate({
      search: (previous) => ({ ...previous, [name]: value === all || !value ? undefined : value }),
      replace: true,
    });
  const openForm = (id?: string) =>
    setForm((previous) => ({ id, open: true, key: previous.key + 1 }));

  return (
    <>
      <div className="sticky top-0 z-2 bg-[#08090b] px-[26px] pt-[24px] pb-[20px] max-[700px]:px-4 max-[700px]:pt-4">
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative max-w-[360px] min-w-[180px] flex-[1_1_220px]">
            <Icon
              name="search"
              className="pointer-events-none absolute top-1/2 left-4 -translate-y-1/2 text-[#8999ad]"
            />
            <Input
              className="w-full rounded-full pl-12"
              placeholder="Search milestones..."
              value={search.q ?? ""}
              onChange={(event) => setFilter("q", event.target.value)}
              aria-label="Search milestones"
            />
          </div>
          <Filter
            name="Status"
            value={search.status ?? all}
            onChange={(value) => setFilter("status", value)}
            options={milestoneStatuses.map((value) => ({
              value,
              label: milestoneStatusLabel[value],
            }))}
          />
          <button
            type="button"
            className="ml-auto inline-flex shrink-0 items-center gap-2 rounded-full border border-[#5f7fff] bg-[#2455eb] px-[15px] py-[8px] font-[550] whitespace-nowrap text-white shadow-[0_4px_14px_#2455eb38,inset_0_1px_#ffffff24] transition-colors hover:border-[#89a0ff] hover:bg-[#3566f6] motion-reduce:transition-none"
            onClick={() => openForm()}
          >
            <Icon name="plus" /> New milestone
          </button>
        </div>
      </div>
      <div className="grid gap-3 px-[26px] pb-8 max-[700px]:px-4">
        {(milestones.error || issues.error) && (
          <div
            role="alert"
            className="flex flex-wrap items-center gap-3 rounded-lg border border-[#683a3a] bg-[#281b1e] p-4 text-sm text-[#ffb1b1]"
          >
            <p>
              {milestones.error
                ? "Could not refresh milestones."
                : "Could not refresh issue progress."}
            </p>
            <button
              type="button"
              className="ml-auto rounded-md border border-[#683a3a] px-3 py-1 hover:bg-[#3b2529]"
              onClick={() => {
                void milestones.refetch();
                void issues.refetch();
              }}
            >
              Try again
            </button>
          </div>
        )}
        {!milestones.data && !milestones.error && (
          <p role="status" className="py-12 text-center text-sm text-[#9ba9be]">
            Loading milestones…
          </p>
        )}
        {milestones.data && !filtered.length && (
          <div className="grid justify-items-center gap-3 rounded-xl border border-dashed border-[#2b3039] px-6 py-16 text-center">
            <Icon name="target" className="mb-1 size-8 text-[#78869a]" />
            <h2 className="text-base font-semibold">
              {projectMilestones.length
                ? "No matching milestones"
                : "Your next milestone starts here"}
            </h2>
            <p className="text-sm text-[#9ba9be]">
              {projectMilestones.length
                ? "Try another search or clear your filters."
                : "Create a milestone, then assign issues to track your progress."}
            </p>
            {projectMilestones.length > 0 ? (
              <button
                type="button"
                className="mt-1 text-sm text-[#8ba7ff] hover:text-[#c3d1ff]"
                onClick={() => void navigate({ search: {}, replace: true })}
              >
                Clear filters
              </button>
            ) : (
              <button
                type="button"
                className="mt-1 text-sm text-[#8ba7ff] hover:text-[#c3d1ff]"
                onClick={() => openForm()}
              >
                Create milestone →
              </button>
            )}
          </div>
        )}
        {filtered.map((milestone) => (
          <MilestoneCard
            key={milestone.id}
            milestone={milestone}
            projectSlug={project.slug}
            progress={
              progress
                ? (progress.get(milestone.id) ?? {
                    total: 0,
                    done: 0,
                    canceled: 0,
                    active: 0,
                    remaining: 0,
                  })
                : undefined
            }
            progressError={!!issues.error}
            onEdit={() => openForm(milestone.id)}
          />
        ))}
      </div>
      <MilestoneFormModal
        key={project.id}
        formKey={form.key}
        projectKey={project.key}
        milestone={projectMilestones.find((milestone) => milestone.id === form.id)}
        open={form.open}
        onClose={() => setForm((previous) => ({ ...previous, open: false }))}
      />
    </>
  );
}
