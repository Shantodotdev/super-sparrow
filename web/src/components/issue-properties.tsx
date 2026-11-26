import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useState } from "react";
import { formatDate, webUrl } from "../lib/format";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { Issue, IssueStatus } from "../lib/types";
import { issueQuery, useMoveIssueToProject, usePatchIssue } from "../queries/issues";
import { actorsQuery, milestonesQuery, projectsQuery } from "../queries/project";
import { projectMilestonesQuery } from "../queries/project-resources";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { ResourceSelector } from "./resource-selector";
import { apiWrite } from "../lib/api";
import { useProject } from "../hooks/use-project";
import type { Milestone } from "../lib/types";
import { Select } from "./select";
import { Avatar } from "./avatar";
import { IssueTextField } from "./issue-text-field";
import { LabelSelector } from "./label-selector";
import { Person } from "./person";
import { MoveIssueDialog } from "./move-issue-dialog";

type IssuePropertiesProps = {
  issue: Issue;
};

/** The side panel of an issue page: status, people, branch, and labels. */
export function IssueProperties({ issue }: IssuePropertiesProps) {
  const project = useProject();
  const client = useQueryClient();
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const { data: milestones } = useSuspenseQuery(projectMilestonesQuery(project.key));
  const patch = usePatchIssue(issue.key);
  const { data: projects } = useSuspenseQuery(projectsQuery);
  const moveIssue = useMoveIssueToProject();
  const navigate = useNavigate();
  const { tab } = useSearch({ strict: false }) as { tab?: string };
  const [moveTarget, setMoveTarget] = useState<string>();
  const target = projects.find((item) => item.id === moveTarget);

  const confirmMove = () => {
    if (!target) return;
    moveIssue.mutate(
      { issue, projectKey: target.key },
      {
        onSuccess: async (moved) => {
          // Go to the new address before refetching, so no query asks for the old key.
          await navigate({
            to: "/$projectSlug/issues/$issue",
            params: { projectSlug: target.slug, issue: moved.key },
            search: { tab: tab as never },
            replace: true,
          });
          client.removeQueries({ queryKey: issueQuery(issue.key).queryKey });
          for (const key of ["issues", "milestones", "activity"])
            void client.invalidateQueries({ queryKey: [key] });
          setMoveTarget(undefined);
        },
      },
    );
  };

  const pullRequestUrl = webUrl(issue.pull_request_url);
  const pullNumber = /\/pull\/(\d+)/.exec(pullRequestUrl)?.[1];

  return (
    <aside className="sticky top-0 h-dvh self-start overflow-y-auto border-l border-l-[#242932] p-[24px] text-[13px] max-[700px]:static max-[700px]:h-auto max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:border-t-[#252a32] [&_.icon]:size-[15px] [&_>_.custom-select]:w-full [&_button[aria-expanded]]:min-h-[36px] [&_button[aria-expanded]]:py-0 [&_button[aria-expanded]]:text-[13px] [&_h3]:mx-0 [&_h3]:mt-[16px] [&_h3]:mb-[6px] [&_h3]:text-[12px] [&_h3:first-child]:mt-0 [&_input]:h-[36px] [&_input]:min-h-[36px] [&_input]:py-0 [&_input]:text-[13px] [&_strong]:text-[13px]">
      <h3>Project</h3>
      <Select
        label="Project"
        value={issue.project_id}
        icon="folder"
        onChange={(value) => {
          if (value === issue.project_id) return;
          moveIssue.reset();
          setMoveTarget(value);
        }}
        options={projects.map((item) => ({
          value: item.id,
          label: item.name,
          icon: "folder" as const,
          iconClassName: "text-[#95a6bd]",
        }))}
      />
      <MoveIssueDialog
        issues={[issue]}
        target={target}
        pending={moveIssue.isPending}
        error={moveIssue.error?.message ?? ""}
        onConfirm={confirmMove}
        onClose={() => setMoveTarget(undefined)}
      />
      <h3>Status</h3>
      <div className="[&_.custom-select-value]:flex-1">
        <Select
          label="Status"
          value={issue.status}
          icon={statusIcon[issue.status]}
          onChange={(value) => patch.mutate({ status: value as IssueStatus })}
          options={issueStatuses.map((status) => ({
            value: status,
            label: issueStatusLabel[status],
            icon: statusIcon[status],
            iconClassName: statusIconColor[status],
          }))}
        />
      </div>
      <h3>Assignee</h3>
      <Select
        label="Assignee"
        placeholder="Assign a teammate"
        value={issue.assignee_actor_id}
        icon="circle-user-round"
        onChange={(value) => patch.mutate({ assignee_actor_id: value })}
        options={[
          {
            value: "",
            label: "Unassigned",
            icon: "circle-user-round",
            iconClassName: "text-[#95a6bd]",
          },
          ...actors
            .filter(
              (actor) =>
                actor.kind === "human" &&
                (!actor.disabled_at || actor.id === issue.assignee_actor_id),
            )
            .map((actor) => ({
              value: actor.id,
              label: actor.display_name,
              leading: (
                <span className="flex shrink-0 [&_.avatar]:size-5 [&_.avatar]:basis-5 [&_.avatar]:text-[9px]">
                  <Avatar actor={actor} />
                </span>
              ),
            })),
        ]}
      />
      <h3>Created by</h3>
      <div className="flex min-w-0 items-center justify-between gap-3 text-[#c9d3e1]">
        <Person actorId={issue.created_by_actor_id} />
        <time className="shrink-0 text-[12px] text-[#8fa0b7]" dateTime={issue.created_at}>
          {formatDate(issue.created_at)}
        </time>
      </div>
      <h3>Milestone</h3>
      <ResourceSelector
        label="Milestone"
        options={milestones.filter((item) => item.project_id === issue.project_id)}
        selected={issue.milestone_id ? [issue.milestone_id] : []}
        onChange={(ids) => patch.mutateAsync({ milestone_id: ids[0] ?? "" })}
        onCreate={async (name) => {
          const { milestone } = await apiWrite<{ milestone: Milestone }>(
            `/api/projects/${encodeURIComponent(project.key)}/milestones`,
            "POST",
            { name },
          );
          client.setQueryData(projectMilestonesQuery(project.key).queryKey, (current) => [
            ...(current ?? []),
            milestone,
          ]);
          client.setQueryData(milestonesQuery.queryKey, (current) =>
            current ? [...current, milestone] : current,
          );
          return milestone.id;
        }}
      />
      <div className="mt-5 border-t border-t-[#252b34] pt-[16px]">
        <h3>Labels</h3>
        <div className="mb-2">
          <LabelChips labelIds={issue.label_ids} projectKey={issue.project_key} />
        </div>
        <LabelSelector
          projectId={issue.project_id}
          selected={issue.label_ids}
          onChange={(ids) => patch.mutateAsync({ label_ids: ids })}
        />
      </div>
      <h3>Branch</h3>
      <IssueTextField
        key={`branch-${issue.id}`}
        issue={issue}
        field="branch_name"
        label="Branch name"
      />
      <h3>Pull request</h3>
      <IssueTextField
        key={`pull-${issue.id}`}
        issue={issue}
        field="pull_request_url"
        label="Pull request URL"
      />
      {pullRequestUrl && (
        <a
          className="mt-1.5 inline-flex items-center gap-1.5 text-[12px] text-[#719bff]"
          href={pullRequestUrl}
          target="_blank"
          rel="noreferrer"
        >
          <Icon name="github" /> {pullNumber ? `#${pullNumber}` : "Open pull request"}
        </a>
      )}
    </aside>
  );
}
