import { useProject } from "../hooks/use-project";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { PageHeader } from "../components/page-header";
import type { MilestoneStatus } from "../lib/types";
import { issuesQuery } from "../queries/issues";
import { milestonesQuery } from "../queries/project";

const statusLabel: Record<MilestoneStatus, string> = {
  planned: "Planned",
  active: "Active",
  done: "Done",
};
import { loadQuery } from "../queries/options";

export const Route = createFileRoute("/$projectSlug/milestones")({
  staticData: { section: "milestones" },
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, milestonesQuery),
      loadQuery(context.queryClient, issuesQuery),
    ]);
  },
  component: MilestonesPage,
});

/** The project's milestones in order; active ones show how many of their issues are finished. */
function MilestonesPage() {
  const project = useProject();
  const { data: milestones } = useSuspenseQuery(milestonesQuery);
  const { data: issues } = useSuspenseQuery(issuesQuery);

  return (
    <>
      <PageHeader
        crumb={project.name}
        title="Milestones"
        description="Track the work that matters for your project."
      />
      <div className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        {milestones
          .filter((milestone) => milestone.project_id === project.id)
          .sort((a, b) => a.position - b.position)
          .map((milestone) => {
            const milestoneIssues = issues.filter((issue) => issue.milestone_id === milestone.id);
            const finished = milestoneIssues.filter(
              (issue) => issue.status === "done" || issue.status === "canceled",
            ).length;
            const progress = milestoneIssues.length ? (finished / milestoneIssues.length) * 100 : 0;

            return (
              <section
                className="rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]"
                key={milestone.id}
              >
                <div className="mb-[20px] flex items-start justify-between gap-[20px] [&_p]:mt-[3px]">
                  <div>
                    <h2>{milestone.name}</h2>
                    <p className="muted text-[#a2aec2]">
                      {statusLabel[milestone.status]} · {milestoneIssues.length}{" "}
                      {milestoneIssues.length === 1 ? "issue" : "issues"} · {finished} finished
                    </p>
                  </div>
                  <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
                    {statusLabel[milestone.status]}
                  </span>
                </div>
                {milestone.status === "active" && (
                  <>
                    <div
                      className="mx-0 my-[18px] h-[5px] rounded-[5px] bg-[#232a34] [&_span]:block [&_span]:h-full [&_span]:bg-[#557dff]"
                      role="progressbar"
                      aria-label={`${milestone.name} progress`}
                      aria-valuenow={Math.round(progress)}
                    >
                      <span style={{ width: `${progress}%` }} />
                    </div>
                    <Link
                      to="/$projectSlug/issues"
                      params={{ projectSlug: project.slug }}
                      search={{ milestone: milestone.id }}
                      className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
                    >
                      View issues →
                    </Link>
                  </>
                )}
              </section>
            );
          })}
      </div>
    </>
  );
}
