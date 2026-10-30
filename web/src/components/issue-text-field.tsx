import { useState } from "react";
import type { Issue } from "../lib/types";
import { usePatchIssue } from "../queries/issues";
import { Input } from "./input";

type IssueTextFieldProps = {
  issue: Issue;
  field: "title" | "branch_name" | "pull_request_url";
  label: string;
  className?: string;
};

/** Saves a single issue text field on blur without replacing the draft while it is being edited. */
export function IssueTextField({ issue, field, label, className }: IssueTextFieldProps) {
  const [draft, setDraft] = useState<string | null>(null);
  const patch = usePatchIssue(issue.key);
  return (
    <Input
      aria-label={label}
      placeholder={label}
      className={className ?? "min-h-12 w-full rounded-[10px] px-3"}
      value={draft ?? issue[field]}
      disabled={patch.isPending}
      onChange={(event) => setDraft(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter") event.currentTarget.blur();
      }}
      onBlur={() => {
        if (draft === null || draft === issue[field]) {
          setDraft(null);
          return;
        }
        patch.mutate({ [field]: draft }, { onSuccess: () => setDraft(null) });
      }}
    />
  );
}
