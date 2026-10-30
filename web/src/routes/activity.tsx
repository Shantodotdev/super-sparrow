import { createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { EventLine } from "../components/event-line";
import { Filter, all } from "../components/filter";
import { PageHeader } from "../components/page-header";
import { formatDate } from "../lib/format";
import type { ActivityEvent } from "../lib/types";
import { issuesQuery } from "../queries/issues";
import { activityQuery, actorsQuery } from "../queries/project";

/** The actor the timeline is filtered to, by ID; missing shows everyone. */
type ActivitySearch = { actor?: string };
import { loadQuery } from "../queries/seeded";

export const Route = createFileRoute("/activity")({
  staticData: { section: "activity" },
  validateSearch: (search: Record<string, unknown>): ActivitySearch => ({
    actor: typeof search.actor === "string" && search.actor ? search.actor : undefined,
  }),
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, activityQuery),
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, issuesQuery),
    ]);
  },
  component: ActivityPage,
});

/** Groups events by the day they happened, newest day and newest event first. */
function byDay(events: ActivityEvent[]): { day: string; events: ActivityEvent[] }[] {
  const days = new Map<string, ActivityEvent[]>();
  for (const event of [...events].reverse()) {
    const day = formatDate(event.created_at);
    days.set(day, [...(days.get(day) ?? []), event]);
  }
  return [...days].map(([day, dayEvents]) => ({ day, events: dayEvents }));
}

/** A timeline of what people and agents did, grouped by day and filterable by actor. */
function ActivityPage() {
  const { actor } = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data: events } = useSuspenseQuery(activityQuery);
  const { data: actors } = useSuspenseQuery(actorsQuery);

  const actorOptions = actors
    .filter((item) => events.some((event) => event.actor_id === item.id))
    .map((item) => ({ value: item.id, label: item.display_name }));
  const days = byDay(events.filter((event) => !actor || event.actor_id === actor));

  return (
    <>
      <PageHeader
        crumb="Activity"
        title="Activity"
        description="Track what’s happening across your projects, issues, and agents."
      />
      <div className="px-[32px] py-[25px] [&_h2]:mx-0 [&_h2]:my-[25px]">
        <div className="mx-0 mt-0 mb-[20px] flex flex-wrap items-center gap-[12px]">
          <Filter
            name="Actor"
            value={actor ?? all}
            onChange={(value) =>
              void navigate({ search: { actor: value === all ? undefined : value }, replace: true })
            }
            options={actorOptions}
          />
          <div className="rounded-[6px] border border-[#292e38] p-[12px]">
            Project &nbsp; Sparrow
          </div>
        </div>
        {days.length === 0 && <p className="muted text-[#a2aec2]">No activity yet.</p>}
        {days.map(({ day, events: dayEvents }) => (
          <section key={day}>
            <h2>{day}</h2>
            <div className="timeline">
              {dayEvents.map((event) => (
                <EventLine key={event.id} event={event} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </>
  );
}
