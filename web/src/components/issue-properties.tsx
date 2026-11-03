import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { webUrl } from "../lib/format";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { Issue, IssueStatus } from "../lib/types";
import { usePatchIssue } from "../queries/issues";
import { actorsQuery, milestonesQuery } from "../queries/project";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { ResourceSelector } from "./resource-selector";
import { apiWrite } from "../lib/api";
import { useProject } from "../hooks/use-project";
import type { Milestone } from "../lib/types";
import { SubissueList } from "./subissue-list";
import { Select } from "./select";
import { Avatar } from "./avatar";
import { IssueTextField } from "./issue-text-field";
import { LabelSelector } from "./label-selector";

type IssuePropertiesProps = {
  issue: Issue;
};

/** The side panel of an issue page: status, people, branch, subissues, and labels. */
export function IssueProperties({ issue }: IssuePropertiesProps) {
  const project = useProject();
  const client = useQueryClient();
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const { data: milestones } = useSuspenseQuery(milestonesQuery);
  const patch = usePatchIssue(issue.key);

  const pullRequestUrl = webUrl(issue.pull_request_url);
  const pullNumber = /\/pull\/(\d+)/.exec(pullRequestUrl)?.[1];

  return (
    <aside className="min-h-0 overflow-y-auto border-l border-l-[#242932] p-[24px] text-[13px] max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:border-t-[#252a32] [&_.icon]:size-[15px] [&_>_.custom-select]:w-full [&_button[aria-expanded]]:min-h-[36px] [&_button[aria-expanded]]:py-0 [&_button[aria-expanded]]:text-[13px] [&_h3]:mx-0 [&_h3]:mt-[16px] [&_h3]:mb-[6px] [&_h3]:text-[12px] [&_h3:first-child]:mt-0 [&_input]:h-[36px] [&_input]:min-h-[36px] [&_input]:py-0 [&_input]:text-[13px] [&_strong]:text-[13px]">
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
          client.setQueryData(milestonesQuery.queryKey, (current) => [
            ...(current ?? []),
            milestone,
          ]);
          return milestone.id;
        }}
      />
      <div className="mt-5 border-t border-t-[#252b34] pt-[16px]">
        <h3>Labels</h3>
        <div className="mb-2">
          <LabelChips labelIds={issue.label_ids} />
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
      <div className="mt-5 border-t border-t-[#252b34] pt-[16px] [&_p]:mt-[10px]">
        <SubissueList issue={issue} compact />
      </div>
    </aside>
  );
}
