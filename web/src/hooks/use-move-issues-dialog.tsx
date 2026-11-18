import { useState, type ReactNode } from "react";
import { MoveIssueDialog } from "../components/move-issue-dialog";
import { useToast } from "../components/toast";
import type { Issue } from "../lib/types";
import { useMoveIssues } from "../queries/issues";
import type { Project } from "../queries/resources";

type MoveRequest = { issues: Issue[]; project: Project };

/**
 * Moves issues to a project after the person confirms. `request` opens the confirmation for the
 * issues that are not in the project yet; `dialog` must be rendered somewhere on the page.
 * `onMoved` runs once every issue has moved.
 */
export function useMoveIssuesDialog(onMoved?: () => void): {
  request: (issues: Issue[], project: Project) => void;
  dialog: ReactNode;
} {
  const { notify } = useToast();
  const move = useMoveIssues();
  const [pending, setPending] = useState<MoveRequest>();

  const request = (issues: Issue[], project: Project) => {
    const movable = issues.filter((issue) => issue.project_id !== project.id);
    if (!movable.length) return;
    move.reset();
    setPending({ issues: movable, project });
  };

  const confirm = () => {
    if (!pending) return;
    move.mutate(
      { issues: pending.issues, projectKey: pending.project.key },
      {
        onSuccess: () => {
          const { issues, project } = pending;
          notify(
            issues.length === 1
              ? `Moved ${issues[0]?.key} to ${project.name}`
              : `Moved ${issues.length} issues to ${project.name}`,
          );
          setPending(undefined);
          onMoved?.();
        },
      },
    );
  };

  const dialog = (
    <MoveIssueDialog
      issues={pending?.issues ?? []}
      target={pending?.project}
      pending={move.isPending}
      error={move.error?.message ?? ""}
      onConfirm={confirm}
      onClose={() => setPending(undefined)}
    />
  );

  return { request, dialog };
}
