import { Link } from "@tanstack/react-router";
import { Pencil } from "lucide-react";
import { milestoneStatusLabel, type MilestoneProgress } from "../lib/milestones";
import type { Milestone, MilestoneStatus } from "../lib/types";
import { Icon, type IconName } from "./icon";

const statusStyle: Record<MilestoneStatus, string> = {
  planned: "border-[#343b46] bg-[#20242b] text-[#aebbd0]",
  active: "border-[#304d9b] bg-[#172442] text-[#83a4ff]",
  done: "border-[#226449] bg-[#142e25] text-[#65dca4]",
};
const statusIcon: Record<MilestoneStatus, IconName> = {
  planned: "circle-dashed",
  active: "circle-dot",
  done: "circle-check",
};

/** Displays real issue progress for every milestone, with issue navigation and an edit action. */
export function MilestoneCard({
  milestone,
  projectSlug,
  progress,
  progressError,
  onEdit,
}: {
  milestone: Milestone;
  projectSlug: string;
  progress?: MilestoneProgress;
  progressError: boolean;
  onEdit: () => void;
}) {
  const total = progress?.total ?? 0;
  const finished = (progress?.done ?? 0) + (progress?.canceled ?? 0);
  const percent = total ? Math.round((finished / total) * 100) : 0;
  const segments = progress
    ? [
        { label: "Done", count: progress.done, color: "bg-[#30d895]" },
        { label: "Canceled", count: progress.canceled, color: "bg-[#8a96a8]" },
        { label: "In progress", count: progress.active, color: "bg-[#487bff]" },
        { label: "Remaining", count: progress.remaining, color: "bg-[#3b4657]" },
      ]
    : [];

  return (
    <section className="@container rounded-xl border border-[#292e37] bg-[#121519] px-5 py-4 transition-colors hover:border-[#3b4554] motion-reduce:transition-none max-[700px]:px-4">
      <div className="flex items-center gap-5 @max-[1100px]:flex-wrap @max-[700px]:gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-full border border-[#303640] bg-[#171b21] text-[#c5cfdd] @max-[700px]:hidden">
          <Icon name="target" className="size-5" />
        </span>
        <h2 className="min-w-0 flex-1 text-[16px] font-semibold text-[#edf0f6]">
          <Link
            to="/$projectSlug/issues"
            params={{ projectSlug }}
            search={{ milestone: milestone.id }}
            title={milestone.name}
            className="block truncate rounded-sm hover:text-[#aac0ff] focus-visible:outline-2 focus-visible:outline-[#6486ff]"
          >
            {milestone.name}
          </Link>
        </h2>
        <span
          className={`inline-flex shrink-0 items-center gap-2 rounded-full border px-3 py-[6px] text-[15px] font-medium ${statusStyle[milestone.status]}`}
        >
          <Icon name={statusIcon[milestone.status]} className="size-4" />
          {milestoneStatusLabel[milestone.status]}
        </span>
        {progress ? (
          <>
            <div className="flex min-w-[160px] flex-[0_1_240px] items-center gap-3 @max-[1100px]:order-1 @max-[1100px]:flex-[1_1_180px]">
              <div
                role="progressbar"
                aria-label={`${milestone.name} progress`}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={percent}
                aria-valuetext={`${finished} of ${total} issues finished, ${progress.canceled} canceled`}
                className="flex h-[9px] flex-1 overflow-hidden rounded-full bg-[#29313d]"
              >
                {segments.map((segment) => (
                  <span
                    key={segment.label}
                    className={segment.color}
                    style={{ width: `${total ? (segment.count / total) * 100 : 0}%` }}
                  />
                ))}
              </div>
              <span className="w-11 shrink-0 text-right text-[15px] font-semibold text-[#edf0f6] tabular-nums">
                {percent}%
              </span>
            </div>
            <ul
              aria-label="Issue status breakdown"
              className="m-0 flex max-w-full shrink-0 list-none items-center gap-x-5 gap-y-2 p-0 text-[15px] text-[#a2b0c4] @max-[1100px]:order-1 @max-[1100px]:flex-wrap"
            >
              {segments
                .filter((segment) => segment.label !== "Canceled")
                .map((segment) => (
                  <li key={segment.label} className="flex items-center gap-2 whitespace-nowrap">
                    <span
                      aria-hidden="true"
                      className={`size-[8px] rounded-full ${segment.color}`}
                    />
                    {segment.label} <span className="tabular-nums">{segment.count}</span>
                  </li>
                ))}
            </ul>
          </>
        ) : (
          <p className="text-[13px] text-[#95a3b8] @max-[1100px]:order-1 @max-[1100px]:basis-full">
            {progressError ? "Issue progress unavailable" : "Loading issue progress…"}
          </p>
        )}
        <button
          type="button"
          aria-label={`Edit ${milestone.name}`}
          onClick={onEdit}
          className="grid size-8 shrink-0 place-items-center rounded-lg border-0 bg-transparent text-[#8897ac] transition-colors hover:bg-[#272e38] hover:text-[#e0e7f2] focus-visible:outline-2 focus-visible:outline-[#6486ff] motion-reduce:transition-none"
        >
          <Pencil aria-hidden="true" className="size-[15px]" />
        </button>
      </div>
    </section>
  );
}
