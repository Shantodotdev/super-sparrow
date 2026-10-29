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
      <div className="activity-content">
        <div className="filters">
          <Filter name="Actor" value={actor} onChange={setActor} options={activityActors} />
          <div className="filter-static">Project &nbsp; Sparrow</div>
        </div>
        {days.map((day) => {
          const events = day.events.filter((event) => actor === "All" || event.actor === actor);
          if (!events.length) return null;

          return (
            <section key={day.label}>
              <h2>{day.label}</h2>
              <div className="timeline">
                {events.map((event) => (
                  <div className="timeline-row" key={event.time}>
                    <time>{event.time}</time>
                    <div className="timeline-dot" />
                    <Person name={event.actor} />
                    <div>
                      <span>{event.action} </span>
                      <Link to="/$issue" params={{ issue: event.issue }} className="text-link">
                        {event.issue}
                      </Link>
                      <p className="muted">{event.detail}</p>
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
