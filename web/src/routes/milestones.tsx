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
      <div className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        {milestones.map((milestone) => (
          <section
            className="rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]"
            key={milestone.name}
          >
            <div className="mb-[20px] flex items-start justify-between gap-[20px] [&_p]:mt-[3px]">
              <div>
                <h2>{milestone.name}</h2>
                <p className="muted text-[#a2aec2]">
                  {milestone.state} · {milestone.issueCount}{" "}
                  {milestone.issueCount === 1 ? "issue" : "issues"}
                </p>
              </div>
              <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
                {milestone.state}
              </span>
            </div>
            {milestone.summary && <p className="muted text-[#a2aec2]">{milestone.summary}</p>}
            {milestone.state === "Active" && (
              <>
                <div className="mx-0 my-[18px] h-[5px] rounded-[5px] bg-[#232a34] [&_span]:block [&_span]:h-full [&_span]:w-[45%] [&_span]:bg-[#557dff]">
                  <span />
                </div>
                <Link
                  to="/issues"
                  className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
                >
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
