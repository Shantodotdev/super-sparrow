import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { PageHeader } from "../components/page-header";
import { milestonesQuery } from "../queries/milestones";

export const Route = createFileRoute("/milestones")({
  staticData: { section: "milestones" },
  loader: async ({ context }) => {
    await context.queryClient.query({ ...milestonesQuery, staleTime: "static" });
  },
  component: MilestonesPage,
});

/** The project's milestones; active ones show progress and a link to their issues. */
function MilestonesPage() {
  const { data: milestones } = useSuspenseQuery(milestonesQuery);

  return (
    <>
      <PageHeader
        crumb="Projects / Sparrow"
        title="Milestones"
        description="Track the work that matters for your project."
      />
      <div className="settings-content">
        {milestones.map((milestone) => (
          <section className="panel" key={milestone.name}>
            <div className="panel-heading">
              <div>
                <h2>{milestone.name}</h2>
                <p className="muted">
                  {milestone.state} · {milestone.issueCount}{" "}
                  {milestone.issueCount === 1 ? "issue" : "issues"}
                </p>
              </div>
              <span className="badge">{milestone.state}</span>
            </div>
            {milestone.summary && <p className="muted">{milestone.summary}</p>}
            {milestone.state === "Active" && (
              <>
                <div className="progress">
                  <span />
                </div>
                <Link to="/issues" className="text-link">
                  View issues →
                </Link>
              </>
            )}
          </section>
        ))}
      </div>
    </>
  );
}
