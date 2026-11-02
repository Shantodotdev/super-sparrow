import { findingStatusLabel, severityLabel } from "../lib/finding-status";
import type { Finding, FindingSeverity as Severity, FindingStatus as Status } from "../lib/types";
import { Icon, type IconName } from "./icon";
import { ReviewPerson } from "./review-person";

const severityColor: Record<Severity, string> = {
  blocking: "text-[#f27c87]",
  should_fix: "text-[#e8b35e]",
  nit: "text-[#aaa5dd]",
};

const statusIcon: Record<Status, IconName> = {
  open: "circle",
  fix: "circle-dot",
  fixed: "circle-check",
  verified: "circle-check",
  wontfix: "circle-slash",
  later: "clock",
};

/** Identifies severity with both an icon and text. */
export function FindingSeverity({ severity }: { severity: Severity }) {
  return (
    <span className={`inline-flex items-center gap-2 ${severityColor[severity]}`}>
      <Icon name={severity === "nit" ? "info" : "triangle-alert"} />
      {severityLabel[severity]}
    </span>
  );
}

/** Keeps a finding's progress visually distinct from its severity. */
export function FindingStatus({ status }: { status: Status }) {
  return (
    <span
      className={`inline-flex items-center gap-2 text-[13px] whitespace-nowrap ${status === "verified" ? "text-[#61c99d]" : status === "open" || status === "fix" || status === "fixed" ? "text-[#89a5ff]" : "text-[#a2aec2]"}`}
    >
      <Icon name={statusIcon[status]} />
      {findingStatusLabel[status]}
    </span>
  );
}

/** Opens a finding's drawer without exposing decisions in the list itself. */
export function FindingRow({
  finding,
  compact = false,
  onOpen,
}: {
  finding: Finding;
  compact?: boolean;
  onOpen: () => void;
}) {
  return (
    <div>
      <button
        type="button"
        onClick={onOpen}
        aria-label={`Open round ${finding.round} finding ${finding.label}: ${finding.title}`}
        className={`group grid w-full grid-cols-[16px_minmax(0,1fr)_80px_16px] items-center gap-3 rounded-[10px] px-3 py-4 text-left transition-colors hover:bg-[#18191a] focus-visible:outline-2 focus-visible:outline-[#6c82ff] ${compact ? "sm:grid-cols-[16px_minmax(0,1fr)_70px_80px_16px]" : "sm:grid-cols-[16px_minmax(0,1fr)_70px_80px_16px] md:grid-cols-[16px_minmax(0,1fr)_70px_80px_80px_16px]"}`}
      >
        <Icon
          name={finding.severity === "nit" ? "info" : "triangle-alert"}
          className={`shrink-0 ${severityColor[finding.severity]}`}
        />
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[15px] font-semibold text-[#eef0f5]" title={finding.title}>
            {finding.title}
          </span>
          {finding.duplicate_of_finding_id && (
            <span className="shrink-0 text-[12px] text-[#97a4b7]">Duplicate</span>
          )}
          {!compact && finding.file_path && (
            <span
              className="hidden max-w-32 shrink-0 truncate font-mono text-[12px] text-[#8290a4] lg:block"
              title={`${finding.file_path}${finding.line ? `:${finding.line}` : ""}`}
            >
              {finding.file_path}
              {finding.line ? `:${finding.line}` : ""}
            </span>
          )}
        </span>
        <span
          className={`hidden text-[12px] whitespace-nowrap sm:block ${severityColor[finding.severity]}`}
        >
          {severityLabel[finding.severity]}
        </span>
        {!compact && (
          <span className="hidden min-w-0 truncate text-[12px] text-[#97a4b7] md:block">
            <ReviewPerson actorId={finding.found_by_actor_id} />
          </span>
        )}
        <FindingStatus status={finding.status} />
        <Icon name="chevron-right" className="text-[#8290a4] group-hover:text-[#c5d1e3]" />
      </button>
    </div>
  );
}
