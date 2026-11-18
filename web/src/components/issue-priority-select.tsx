import {
  issuePriorities,
  issuePriorityColor,
  issuePriorityIcon,
  issuePriorityLabel,
} from "../lib/issue-status";
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
      options={(["", ...issuePriorities] as IssuePriority[]).map((value) => ({
        value,
        label: issuePriorityLabel[value],
        icon: issuePriorityIcon[value],
        iconClassName: issuePriorityColor[value],
      }))}
    />
  );
}
