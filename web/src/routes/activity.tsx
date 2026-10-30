import { useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Filter } from "../components/filter";
import { PageHeader } from "../components/page-header";
import { Person } from "../components/person";
import { activityActors } from "../data/activity";
import { activityQuery } from "../queries/activity";

export const Route = createFileRoute("/activity")({
  staticData: { section: "activity" },
  loader: async ({ context }) => {
    await context.queryClient.query({ ...activityQuery, staleTime: "static" });
  },
  component: ActivityPage,
});

/** A timeline of what people and agents did, grouped by day and filterable by actor. */
function ActivityPage() {
  const { data: days } = useSuspenseQuery(activityQuery);
  const [actor, setActor] = useState("All");

  return (
    <>
      <PageHeader
        crumb="Activity"
        title="Activity"
        description="Track what’s happening across your projects, issues, and agents."
      />
      <div className="px-[32px] py-[25px] [&_h2]:mx-0 [&_h2]:my-[25px]">
        <div className="mx-0 mt-0 mb-[20px] flex flex-wrap items-center gap-[12px]">
          <Filter name="Actor" value={actor} onChange={setActor} options={activityActors} />
          <div className="rounded-[6px] border border-[#292e38] p-[12px]">
            Project &nbsp; Sparrow
          </div>
        </div>
        {days.map((day) => {
          const events = day.events.filter((event) => actor === "All" || event.actor === actor);
          if (!events.length) return null;

          return (
            <section key={day.label}>
              <h2>{day.label}</h2>
              <div className="timeline">
                {events.map((event) => (
                  <div
                    className="grid grid-cols-[100px_14px_180px_1fr] items-start gap-[18px] px-0 py-[15px] max-[700px]:grid-cols-[65px_10px_1fr] max-[700px]:gap-[8px] max-[700px]:[&_>_div:last-child]:col-3 [&_p]:mt-[6px] [&_time]:text-[#95a6bd]"
                    key={event.time}
                  >
                    <time>{event.time}</time>
                    <div className="mt-[5px] h-[10px] w-[10px] rounded-full bg-[#77889f]" />
                    <Person name={event.actor} />
                    <div>
                      <span>{event.action} </span>
                      <Link
                        to="/$issue"
                        params={{ issue: event.issue }}
                        className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
                      >
                        {event.issue}
                      </Link>
                      <p className="muted text-[#a2aec2]">{event.detail}</p>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </>
  );
}
