import { findingStatusLabel } from "../lib/finding-status";
import { slugify } from "../lib/format";
import type { Finding } from "../lib/types";
import { Icon } from "./icon";
import { Person } from "./person";
import { SeverityBadge } from "./severity-badge";

type FindingCardProps = {
  finding: Finding;
  selected: boolean;
  onSelect: () => void;
};

/** A finding in the review round list, with its comment count and status. */
export function FindingCard({ finding, selected, onSelect }: FindingCardProps) {
  return (
    <button
      className={`flex w-full gap-[22px] border border-b border-transparent border-b-[#262c34] bg-transparent px-[16px] py-[18px] text-left [&_p]:mx-0 [&_p]:my-[12px] [&_p]:leading-normal [&_p]:text-[#aab8cc] [&.selected]:rounded-[8px] [&.selected]:border [&.selected]:border-[#567bff] [&.selected]:bg-[#151e30] ${selected ? "selected" : ""}`}
      aria-current={selected ? "true" : undefined}
      onClick={onSelect}
    >
      <span
        className={`grid h-8 w-8 flex-[0_0_28px] place-items-center rounded-full bg-[#e94c61] font-bold text-[#121316] [&.nit]:bg-[#a9b9d0] [&.should-fix]:bg-[#ffc842] ${slugify(finding.severity)}`}
      >
        <Icon name={finding.severity === "nit" ? "info" : "triangle-alert"} />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-[9px] [&_strong]:text-[16px]">
          <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
            {finding.label}
          </span>{" "}
          <strong>{finding.title}</strong> <SeverityBadge value={finding.severity} />
        </div>
        <p>{finding.problem}</p>
        <div className="flex items-center justify-between">
          <Person actorId={finding.found_by_actor_id} />
          <span className="muted text-[#a2aec2]" title={`${finding.comment_count} comments`}>
            <Icon name="message-square" /> &nbsp;{finding.comment_count}　{" "}
            <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
              ● {findingStatusLabel[finding.status]}
            </span>
          </span>
        </div>
      </div>
    </button>
  );
}
