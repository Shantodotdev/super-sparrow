import { useLabelName } from "../queries/project";

const labelStyles: Record<string, string> = {
  ux: "border-[#3b255b] bg-[#211634] text-[#c5a8ff]",
  ui: "border-[#3b255b] bg-[#211634] text-[#c5a8ff]",
  frontend: "border-[#3b255b] bg-[#211634] text-[#c5a8ff]",
  feature: "border-[#1b493b] bg-[#102820] text-[#a1e4cd]",
  backend: "border-[#1b493b] bg-[#102820] text-[#a1e4cd]",
  bug: "border-[#62303a] bg-[#32171e] text-[#f2a9ae]",
  security: "border-[#62303a] bg-[#32171e] text-[#f2a9ae]",
  documentation: "border-[#594626] bg-[#302513] text-[#f2cf87]",
  research: "border-[#594626] bg-[#302513] text-[#f2cf87]",
  realtime: "border-[#235057] bg-[#10292d] text-[#8fd5da]",
  accessibility: "border-[#235057] bg-[#10292d] text-[#8fd5da]",
  database: "border-[#235057] bg-[#10292d] text-[#8fd5da]",
};

/** Shows an issue's labels by ID, each in its color. */
export function LabelChips({ labelIds }: { labelIds: string[] }) {
  const labelName = useLabelName();
  const labels = labelIds.map(labelName);

  return (
    <span className="flex min-w-0 flex-wrap items-center gap-[5px]">
      {labels.map((label) => (
        <span
          className={`label-chip inline-block rounded-full border px-[8px] py-[4px] text-[12px] ${labelStyles[label.toLowerCase()] ?? "border-[#1d356d] bg-[#101d3c] text-[#a7c0ff]"}`}
          key={label}
        >
          {label}
        </span>
      ))}
    </span>
  );
}
