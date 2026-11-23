import { createFileRoute, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Icon, type IconName } from "../components/icon";
import { PageHeader } from "../components/page-header";
import { useProject } from "../hooks/use-project";
import { formatRelative, webUrl } from "../lib/format";
import {
  checksLabel,
  pullRequestFilters,
  pullRequestStateLabel,
  reviewLabel,
  validatePullRequestsSearch,
} from "../lib/pull-requests";
import type { Actor, PullRequest } from "../lib/types";
import { pullRequestsQuery } from "../queries/github";
import { actorsQuery } from "../queries/project";

export const Route = createFileRoute("/$projectSlug/pulls")({
  staticData: { section: "pulls" },
  validateSearch: validatePullRequestsSearch,
  component: PullRequestsPage,
});

const stateIcons: Record<string, { icon: IconName; className: string }> = {
  open: { icon: "git-pull-request", className: "text-[#44ce91]" },
  draft: { icon: "git-pull-request-draft", className: "text-[#a9b6c8]" },
  merged: { icon: "git-merge", className: "text-[#a574ff]" },
  closed: { icon: "git-pull-request-closed", className: "text-[#f2a9ae]" },
};

const reviewClasses: Record<string, string> = {
  approved: "border-[#1f5a45] bg-[#10281f] text-[#65dca4]",
  changes_requested: "border-[#713440] bg-[#30191f] text-[#f2a9ae]",
  commented: "border-[#2b313a] bg-[#191c22] text-[#c5cfdd]",
};

const checksClasses: Record<string, string> = {
  pending: "border-[#5e4a1c] bg-[#2a2210] text-[#f2c464]",
  success: "border-[#1f5a45] bg-[#10281f] text-[#65dca4]",
  failure: "border-[#713440] bg-[#30191f] text-[#f2a9ae]",
};

const badgeClass =
  "inline-flex items-center rounded-full border px-2.5 py-[3px] text-[12px] whitespace-nowrap";

/** Lists the project's pull requests from linked GitHub repositories, filtered by state. */
function PullRequestsPage() {
  const project = useProject();
  const { state } = Route.useSearch();
  const navigate = Route.useNavigate();
  const pulls = useQuery(pullRequestsQuery(project.key, state));
  const actors = useQuery(actorsQuery);
  const people = new Map((actors.data ?? []).map((actor) => [actor.id, actor]));

  return (
    <>
      <PageHeader
        crumb={project.name}
        title="Pull requests"
        description="Pull requests from the GitHub repositories linked to this project."
      />
      <div className="px-[32px] py-[25px] max-[700px]:px-[16px]">
        <div
          role="group"
          aria-label="Pull request state"
          className="mb-[20px] inline-flex gap-[4px] rounded-full border border-[#292e38] bg-[#0d0f12] p-[4px]"
        >
          {pullRequestFilters.map((filter) => (
            <button
              key={filter.value}
              type="button"
              aria-pressed={state === filter.value}
              onClick={() => void navigate({ search: { state: filter.value }, replace: true })}
              className="rounded-full border-0 bg-transparent px-4 py-[6px] text-[13px] font-[550] text-[#9eabbd] hover:text-[#e4eaf4] aria-pressed:bg-[#20252d] aria-pressed:text-[#f5f6fa]"
            >
              {filter.label}
            </button>
          ))}
        </div>

        {pulls.isPending && (
          <p role="status" className="muted text-[#a2aec2]">
            Loading pull requests…
          </p>
        )}
        {pulls.error && (
          <p role="alert" className="text-[#f2a9ae]">
            {pulls.error.message}
          </p>
        )}
        {pulls.data && pulls.data.length === 0 && (
          <p role="status" className="muted text-[#a2aec2]">
            {state === "open" || state === "all"
              ? "No pull requests yet. Link a repository to this project in Settings → GitHub, then load its pull requests."
              : `No ${state} pull requests.`}
          </p>
        )}
        {pulls.data && pulls.data.length > 0 && (
          <ul className="m-0 list-none divide-y divide-[#20242b] rounded-[8px] border border-[#222730] bg-[#0d0f12] p-0">
            {pulls.data.map((pull) => (
              <PullRequestRow
                key={pull.id}
                pull={pull}
                author={pull.author_person_id ? people.get(pull.author_person_id) : undefined}
                projectSlug={project.slug}
              />
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

/** One pull request: state, title, author, branches, size, review and check status, and the linked issue. */
function PullRequestRow({
  pull,
  author,
  projectSlug,
}: {
  pull: PullRequest;
  author: Actor | undefined;
  projectSlug: string;
}) {
  const stateKey = pull.state === "open" && pull.draft ? "draft" : pull.state;
  const { icon, className } = stateIcons[stateKey]!;
  const url = webUrl(pull.url);

  return (
    <li className="grid grid-cols-[24px_1fr_auto] items-start gap-4 px-[18px] py-4 max-[700px]:grid-cols-[24px_1fr]">
      <span title={pullRequestStateLabel(pull)} className="pt-[2px]">
        <Icon name={icon} className={`size-[19px] ${className}`} />
        <span className="sr-only">{pullRequestStateLabel(pull)}</span>
      </span>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-[10px] gap-y-[4px]">
          {url ? (
            <a
              href={url}
              target="_blank"
              rel="noreferrer noopener"
              className="border-0 bg-none p-0 font-semibold text-[#e4eaf4] hover:text-[#719bff]"
            >
              {pull.title}
            </a>
          ) : (
            <strong>{pull.title}</strong>
          )}
          {pull.review_state !== "none" && (
            <span className={`${badgeClass} ${reviewClasses[pull.review_state]}`}>
              {reviewLabel[pull.review_state]}
            </span>
          )}
          {pull.checks_state !== "none" && (
            <span className={`${badgeClass} ${checksClasses[pull.checks_state]}`}>
              {checksLabel[pull.checks_state]}
            </span>
          )}
          {pull.issue_key && (
            <Link
              to="/$projectSlug/issues/$issue"
              params={{ projectSlug, issue: pull.issue_key }}
              className="border-0 bg-none p-0 text-[13px] text-[#719bff] hover:text-[#d2e0ff]"
            >
              {pull.issue_key}
            </Link>
          )}
        </div>
        <p className="muted mt-[5px] text-[13px] text-[#95a6bd]">
          {pull.repository}#{pull.number} · {author ? author.display_name : `@${pull.author_login}`}
          {pull.state === "merged" && pull.merged_by_login
            ? ` · merged by @${pull.merged_by_login}`
            : ""}
        </p>
        <p className="muted mt-[5px] flex flex-wrap items-center gap-x-[12px] text-[13px] text-[#95a6bd]">
          <code>
            {pull.head_ref} → {pull.base_ref}
          </code>
          <span>
            <span className="text-[#65dca4]">+{pull.additions}</span>{" "}
            <span className="text-[#f2a9ae]">−{pull.deletions}</span>
          </span>
          {pull.requested_reviewers.length > 0 && (
            <span>
              Reviewers: {pull.requested_reviewers.map((login) => `@${login}`).join(", ")}
            </span>
          )}
        </p>
      </div>
      <time
        dateTime={pull.updated_at}
        className="text-[13px] whitespace-nowrap text-[#95a6bd] max-[700px]:col-start-2"
      >
        {formatRelative(pull.updated_at)}
      </time>
    </li>
  );
}
