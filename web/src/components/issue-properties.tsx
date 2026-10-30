import { useProjectSlugs } from "../hooks/use-project";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { slugify, webUrl } from "../lib/format";
import { issueStatusLabel, issueStatuses, statusIcon } from "../lib/issue-status";
import type { Finding, Issue, IssueStatus, ReviewRound } from "../lib/types";
import { useUpdateIssue } from "../queries/issues";
import { labelsQuery, milestonesQuery } from "../queries/project";
import { Icon } from "./icon";
import { LabelChips } from "./label-chips";
import { LabelPicker } from "./label-picker";
import { Person } from "./person";
import { ReviewRoundList } from "./review-round-list";
import { Select } from "./select";

type IssuePropertiesProps = {
  issue: Issue;
  rounds: ReviewRound[];
  findings: Finding[];
};

/** The side panel of an issue page: status, people, branch, review rounds, and labels. */
export function IssueProperties({ issue, rounds, findings }: IssuePropertiesProps) {
  const projectSlug = useProjectSlugs();
  const [editingLabels, setEditingLabels] = useState(false);
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const { data: milestones } = useSuspenseQuery(milestonesQuery);
  const updateIssue = useUpdateIssue();

  const milestone = milestones.find((item) => item.id === issue.milestone_id);
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
          onChange={(value) => updateIssue(issue.id, () => ({ status: value as IssueStatus }))}
          options={issueStatuses.map((status) => ({
            value: status,
            label: issueStatusLabel[status],
          }))}
        />
      </div>
      <h3>Assignee</h3>
      <div className="rounded-[6px] border border-[#252b34] p-[10px] text-[#c7d1df]">
        <Person actorId={issue.assignee_actor_id} />
      </div>
      <h3>Milestone</h3>
      <div className="rounded-[6px] border border-[#252b34] p-[10px] text-[#c7d1df]">
        <Icon name="target" className="issue-icon-goal" /> &nbsp;{milestone?.name ?? "No milestone"}
      </div>
      <h3>Branch</h3>
      <div className="rounded-[6px] border border-[#252b34] p-[10px] text-[#c7d1df]">
        <Icon name="git-branch" className="issue-icon-branch" /> &nbsp;{" "}
        {issue.branch_name || "No branch"}
      </div>
      <h3>Pull request</h3>
      <div className="rounded-[6px] border border-[#252b34] p-[10px] text-[#c7d1df]">
        <Icon name="github" className="issue-icon-pull-request" /> &nbsp;{" "}
        {pullRequestUrl ? (
          <a className="text-[#719bff]" href={pullRequestUrl} target="_blank" rel="noreferrer">
            {pullNumber ? `#${pullNumber}` : pullRequestUrl}
          </a>
        ) : (
          "No pull request"
        )}
      </div>
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
      <div className="mt-8 border-t border-t-[#252b34] pt-[22px] [&_p]:mt-[15px]">
        <div className="mb-[10px] flex justify-between">
          <strong>Labels</strong>
          <button
            className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
            type="button"
            aria-expanded={editingLabels}
            onClick={() => setEditingLabels(!editingLabels)}
          >
            {editingLabels ? "Done" : "Edit"}
          </button>
        </div>
        <LabelChips labelIds={issue.label_ids} />
        {issue.label_ids.length === 0 && (
          <p className="muted mt-[3px] text-[12px] text-[#a2aec2]">No labels</p>
        )}
        {editingLabels && (
          <LabelPicker
            options={labels.filter((label) => label.project_id === issue.project_id)}
            selected={issue.label_ids}
            onChange={(labelIds) => updateIssue(issue.id, () => ({ label_ids: labelIds }))}
          />
        )}
      </div>
    </aside>
  );
}
