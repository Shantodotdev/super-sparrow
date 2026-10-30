import { useSuspenseQuery } from "@tanstack/react-query";
import { labelsQuery } from "../queries/project";

/** Shows an issue's labels using their persisted colors. */
export function LabelChips({ labelIds }: { labelIds: string[] }) {
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const byId = new Map(labels.map((label) => [label.id, label]));

  return (
    <span className="flex min-w-0 flex-wrap items-center gap-[5px]">
      {labelIds.map((id) => {
        const label = byId.get(id);
        const color = label?.color ?? "#a7c0ff";
        return (
          <span
            key={id}
            className="label-chip inline-block rounded-full border px-[8px] py-[4px] text-[12px]"
            style={{ color, borderColor: `${color}40`, backgroundColor: `${color}18` }}
          >
            {label?.name ?? id}
          </span>
        );
      })}
    </span>
  );
}
