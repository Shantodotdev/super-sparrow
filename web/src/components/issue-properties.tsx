import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { sampleIssueDetails } from "../data/issue-details";
import { slugify } from "../lib/format";
import { issueStatuses, statusIcon } from "../lib/issue-status";
import type { Finding, Issue, IssueStatus, ReviewRound } from "../lib/types";
import { issueLabelOptions, issuesQuery, useUpdateIssue } from "../queries/issues";
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
  const [editingLabels, setEditingLabels] = useState(false);
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const updateIssue = useUpdateIssue();
  const { branch, pullRequest } = sampleIssueDetails;

  return (
    <aside className="properties">
      <h3>Status</h3>
      <div className={`issue-status status-${slugify(issue.status)}`}>
        <Select
          label="Status"
          value={issue.status}
          icon={statusIcon[issue.status]}
          onChange={(value) => updateIssue(issue.key, { status: value as IssueStatus })}
          options={issueStatuses.map((status) => ({ value: status, label: status }))}
        />
      </div>
      <h3>Assignee</h3>
      <div className="property">
        <Person name={issue.assignee} />
      </div>
      <h3>Milestone</h3>
      <div className="property">
        <Icon name="target" className="issue-icon-goal" /> &nbsp;{issue.milestone}
      </div>
      <h3>Branch</h3>
      <div className="property">
        <Icon name="git-branch" className="issue-icon-branch" /> &nbsp; {branch}
      </div>
      <h3>Pull request</h3>
      <div className="property">
        <Icon name="github" className="issue-icon-pull-request" /> &nbsp;{" "}
        <span className="link-blue">#{pullRequest.number}</span> &nbsp; {pullRequest.title}
      </div>
      <div className="property-section">
        <div className="section-heading">
          <strong>Review rounds</strong>
          {rounds.length > 0 && (
            <Link
              className="text-link"
              to="/$issue"
              params={{ issue: issue.key }}
              search={{ tab: "rounds" }}
            >
              View all →
            </Link>
          )}
        </div>
        <ReviewRoundList rounds={rounds} findings={findings} />
      </div>
      <div className="property-section">
        <div className="section-heading">
          <strong>Labels</strong>
          <button
            className="text-link"
            type="button"
            aria-expanded={editingLabels}
            onClick={() => setEditingLabels(!editingLabels)}
          >
            {editingLabels ? "Done" : "Edit"}
          </button>
        </div>
        <LabelChips labels={issue.labels} />
        {issue.labels.length === 0 && <p className="muted small">No labels</p>}
        {editingLabels && (
          <LabelPicker
            options={issueLabelOptions(issues)}
            selected={issue.labels}
            onChange={(labels) => updateIssue(issue.key, { labels })}
          />
        )}
      </div>
    </aside>
  );
}
