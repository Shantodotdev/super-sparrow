import type { FindingSeverity } from "../lib/types";

const severityStyles: Record<FindingSeverity, string> = {
  Blocking: "border-[#79212e] bg-[#320e17] text-[#ff7f88]",
  "Should fix": "border-[#70500e] bg-[#2b210c] text-[#ffd26f]",
  Nit: "border-[#503d82] bg-[#201a36] text-[#bfaaff]",
};

export function SeverityBadge({ value }: { value: FindingSeverity }) {
  return (
    <span
      className={`inline-block rounded-full border px-2 py-[4px] text-[12px] whitespace-nowrap ${severityStyles[value]}`}
    >
      {value}
    </span>
  );
}
