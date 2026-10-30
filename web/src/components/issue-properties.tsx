import { useProjectSlugs } from "../hooks/use-project";
import { Link } from "@tanstack/react-router";
import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { slugify, webUrl } from "../lib/format";
import { issueStatusLabel, issueStatuses, statusIcon } from "../lib/issue-status";
import type { Finding, Issue, IssueStatus, ReviewRound } from "../lib/types";
import { usePatchIssue } from "../queries/issues";
import { actorsQuery, labelsQuery, milestonesQuery } from "../queries/project";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { ResourceSelector } from "./resource-selector";
import { apiWrite } from "../lib/api";
import { useProject } from "../hooks/use-project";
import type { Label, Milestone } from "../lib/types";
import { ReviewRoundList } from "./review-round-list";
import { Select } from "./select";
import { IssueTextField } from "./issue-text-field";

type IssuePropertiesProps = {
  issue: Issue;
  rounds: ReviewRound[];
  findings: Finding[];
};

/** The side panel of an issue page: status, people, branch, review rounds, and labels. */
export function IssueProperties({ issue, rounds, findings }: IssuePropertiesProps) {
  const projectSlug = useProjectSlugs();
  const project = useProject();
  const client = useQueryClient();
  const { data: actors } = useSuspenseQuery(actorsQuery);
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const { data: milestones } = useSuspenseQuery(milestonesQuery);
  const patch = usePatchIssue(issue.key);

  const pullRequestUrl = webUrl(issue.pull_request_url);
  const pullNumber = /\/pull\/(\d+)/.exec(pullRequestUrl)?.[1];

  return (
    <aside className="border-l border-l-[#242932] p-[30px] max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:border-t-[#252a32] [&_>_.custom-select]:w-full [&_h3]:mx-0 [&_h3]:mt-[22px] [&_h3]:mb-[9px] [&_h3]:text-[14px] [&_h3:first-child]:mt-0">
      <h3>Status</h3>
      <div
        className={`[--issue-status-color:#98a9c3] [&_.custom-select-value]:flex-1 [&_[role=combobox]>svg:first-child]:text-(--issue-status-color) [&.status-backlog]:text-[#a9b6c8] [&.status-backlog]:[--issue-status-color:#a9b6c8] [&.status-done]:text-[#44ce91] [&.status-done]:[--issue-status-color:#44ce91] [&.status-in-progress]:text-[#4c7aff] [&.status-in-progress]:[--issue-status-color:#4c7aff] [&.status-in-review]:text-[#a574ff] [&.status-in-review]:[--issue-status-color:#a574ff] status-${slugify(issue.status)}`}
      >
        <Select
          label="Status"
          value={issue.status}
          icon={statusIcon[issue.status]}
          onChange={(value) => patch.mutate({ status: value as IssueStatus })}
          options={issueStatuses.map((status) => ({
            value: status,
            label: issueStatusLabel[status],
          }))}
        />
      </div>
      <h3>Assignee</h3>
      <Select
        label="Assignee"
        value={issue.assignee_actor_id}
        icon="circle-user-round"
        onChange={(value) => patch.mutate({ assignee_actor_id: value })}
        options={[
          { value: "", label: "Unassigned" },
          ...actors
            .filter(
              (actor) =>
                actor.kind !== "system" &&
                (!actor.disabled_at || actor.id === issue.assignee_actor_id),
            )
            .map((actor) => ({
              value: actor.id,
              label: actor.display_name + (actor.kind === "agent" ? " (agent)" : ""),
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
            { name, status: "planned" },
          );
          client.setQueryData(milestonesQuery.queryKey, (current) => [
            ...(current ?? []),
            milestone,
          ]);
          return milestone.id;
        }}
      />
      <div className="mt-8 border-t border-t-[#252b34] pt-[22px]">
        <h3>Labels</h3>
        <div className="mb-3">
          <LabelChips labelIds={issue.label_ids} />
        </div>
        <ResourceSelector
          label="Labels"
          multiple
          colors
          options={labels.filter((label) => label.project_id === issue.project_id)}
          selected={issue.label_ids}
          onChange={(ids) => patch.mutateAsync({ label_ids: ids })}
          onCreate={async (name, color) => {
            const { label } = await apiWrite<{ label: Label }>(
              `/api/projects/${encodeURIComponent(project.key)}/labels`,
              "POST",
              { name, color },
            );
            client.setQueryData(labelsQuery.queryKey, (current) => [...(current ?? []), label]);
            return label.id;
          }}
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
          className="mt-2 inline-flex items-center gap-2 text-sm text-[#719bff]"
          href={pullRequestUrl}
          target="_blank"
          rel="noreferrer"
        >
          <Icon name="github" /> {pullNumber ? `#${pullNumber}` : "Open pull request"}
        </a>
      )}
      <div className="mt-8 border-t border-t-[#252b34] pt-[22px] [&_p]:mt-[15px]">
        <div className="mb-[10px] flex justify-between">
          <strong>Review rounds</strong>
          {rounds.length > 0 && (
            <Link
              className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
              to="/$projectSlug/issues/$issue"
              params={{ projectSlug: projectSlug(issue.project_id), issue: issue.key }}
              search={{ tab: "rounds" }}
            >
              View all →
            </Link>
          )}
        </div>
        <ReviewRoundList issue={issue} rounds={rounds} findings={findings} />
      </div>
    </aside>
  );
}
