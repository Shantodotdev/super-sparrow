import type { Issue, IssuePriority } from "../lib/types";
import { usePatchIssue } from "../queries/issues";
import { Select } from "./select";

/** Shows the persisted priority below an issue's title and saves changes immediately. */
export function IssuePrioritySelect({ issue }: { issue: Issue }) {
  const patch = usePatchIssue(issue.key);
  return (
    <Select
      label="Priority"
      variant="chip"
      value={issue.priority ?? ""}
      onChange={(value) => patch.mutate({ priority: value as IssuePriority })}
      options={[
        { value: "", label: "No priority", icon: "circle-dashed", iconClassName: "text-[#a9b6c8]" },
        {
          value: "urgent",
          label: "Urgent",
          icon: "triangle-alert",
          iconClassName: "text-[#f2a9ae]",
        },
        { value: "high", label: "High", icon: "triangle-alert", iconClassName: "text-[#f2ad67]" },
        { value: "medium", label: "Medium", icon: "circle-dot", iconClassName: "text-[#f2cf87]" },
        { value: "low", label: "Low", icon: "circle", iconClassName: "text-[#a7c0ff]" },
      ]}
    />
  );
}
