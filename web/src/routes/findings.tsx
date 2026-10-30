import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { FindingDetail } from "../components/finding-detail";
import { Filter, all } from "../components/filter";
import { PageHeader } from "../components/page-header";
import { Person } from "../components/person";
import { SeverityBadge } from "../components/severity-badge";
import {
  findingSeverities,
  findingStatusLabel,
  findingStatuses,
  isUnresolved,
  severityLabel,
} from "../lib/finding-status";
import { validateFindingsSearch, type FindingsSearch } from "../lib/search";
import type { Finding } from "../lib/types";
import { findingsQuery } from "../queries/findings";
import { issuesQuery } from "../queries/issues";
import { actorsQuery, commentsQuery, reviewRoundsQuery } from "../queries/project";
import { loadQuery } from "../queries/options";

export const Route = createFileRoute("/findings")({
  staticData: { section: "findings" },
  validateSearch: validateFindingsSearch,
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, findingsQuery),
      loadQuery(context.queryClient, issuesQuery),
      loadQuery(context.queryClient, reviewRoundsQuery),
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, commentsQuery),
    ]);
  },
  component: FindingsPage,
});

/** The status filter's choice for the server's unresolved findings, which the page shows first. */
const unresolved = "unresolved";

const severityOptions = findingSeverities.map((value) => ({ value, label: severityLabel[value] }));
const statusOptions = [
  { value: unresolved, label: "Unresolved" },
  ...findingStatuses.map((value) => ({ value, label: findingStatusLabel[value] })),
];

const severityOrder = Object.fromEntries(findingSeverities.map((value, index) => [value, index]));

/** Reports whether a finding passes the page's filters. Duplicates count as resolved. */
function matches(finding: Finding, search: FindingsSearch): boolean {
  const query = (search.q ?? "").trim().toLowerCase();
  const status =
    search.status === undefined
      ? isUnresolved(finding)
      : search.status === "all" || finding.status === search.status;

  return (
    status &&
    `${finding.title} ${finding.problem}`.toLowerCase().includes(query) &&
    (!search.severity || finding.severity === search.severity) &&
    (!search.issue || finding.issue_key === search.issue)
  );
}

/**
 * Findings across every review round, grouped by issue, most severe first, with the selected one
 * on the right. Filters and the selected finding live in the URL. The selected finding stays
 * selected after a decision moves it out of the filter, so a second key press never decides a
 * finding nobody looked at.
 */
function FindingsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data: findings } = useSuspenseQuery(findingsQuery);
  const { data: issues } = useSuspenseQuery(issuesQuery);

  const visible = findings
    .filter((finding) => matches(finding, search))
    .sort((a, b) => severityOrder[a.severity]! - severityOrder[b.severity]! || b.round - a.round);
  const active = findings.find((finding) => finding.id === search.finding) ?? visible[0];
  const issueTitles = new Map(issues.map((issue) => [issue.key, issue.title]));
  const issueOptions = [...new Set(findings.map((finding) => finding.issue_key))]
    .sort()
    .map((key) => ({ value: key, label: key }));

  /**
   * Sets one filter in the URL, replacing the history entry. "All" removes a filter, except the
   * status filter, whose absence means unresolved; it keeps "all" instead.
   */
  const setFilter = (name: Exclude<keyof FindingsSearch, "finding">, value: string) => {
    const empty = name === "status" ? value === unresolved : value === all || value === "";
    void navigate({
      search: (previous) => ({ ...previous, [name]: empty ? undefined : value }),
      replace: true,
    });
  };

  return (
    <>
      <div className="sticky top-0 z-2 bg-[#08090b]">
        <PageHeader
          crumb="Projects / Sparrow"
          title="Open findings"
          description="Findings across review rounds that need attention."
        />
        <div className="m-0 flex flex-wrap items-center gap-[12px] px-8 py-[20px]">
          <input
            className="filter-search w-[295px] rounded-full"
            placeholder="Search findings..."
            aria-label="Search findings"
            value={search.q ?? ""}
            onChange={(event) => setFilter("q", event.target.value)}
          />
          <Filter
            name="Severity"
            value={search.severity ?? all}
            onChange={(value) => setFilter("severity", value)}
            options={severityOptions}
          />
          <Filter
            name="Status"
            value={search.status ?? unresolved}
            onChange={(value) => setFilter("status", value)}
            options={statusOptions}
          />
          <Filter
            name="Issue"
            value={search.issue ?? all}
            onChange={(value) => setFilter("issue", value)}
            options={issueOptions}
          />
        </div>
      </div>
      <div className="findings-split grid grid-cols-[minmax(0,1.1fr)_minmax(380px,1fr)] border-t border-t-[#252a32] max-[700px]:block [&_>_div:first-child]:min-w-0">
        <div>
          {visible.length === 0 ? (
            <div className="mx-auto my-40 max-w-120 p-[24px] text-center text-[#93a0b2] [&_h2]:mx-0 [&_h2]:mt-[18px] [&_h2]:mb-2 [&_h2]:text-[#e9edf5]">
              <h2>No findings found</h2>
              <p>Try changing your filters.</p>
            </div>
          ) : (
            [...new Set(visible.map((finding) => finding.issue_key))].map((issueKey) => (
              <section key={issueKey}>
                <div className="flex gap-4 border-b border-b-[#242932] px-[20px] py-[16px]">
                  <Link
                    to="/$issue"
                    params={{ issue: issueKey }}
                    className="border-0 bg-none p-0 text-[#719bff] hover:text-[#d2e0ff]"
                  >
                    {issueKey}
                  </Link>{" "}
                  <strong>{issueTitles.get(issueKey)}</strong>
                </div>
                {visible
                  .filter((finding) => finding.issue_key === issueKey)
                  .map((finding) => (
                    <button
                      key={finding.id}
                      aria-current={finding === active ? "true" : undefined}
                      className={`flex w-full items-center gap-[16px] border border-b border-transparent border-b-[#242932] bg-none p-[13px] text-left [&_.person-inline]:text-[12px] max-[1100px]:[&_.person-inline]:hidden max-[1100px]:[&_>_.muted]:hidden [&_>_span:nth-child(2)]:min-w-0 [&_>_span:nth-child(2)]:flex-1 [&_small]:mt-[5px] [&_small]:block [&_small]:overflow-hidden [&_small]:text-ellipsis [&_small]:whitespace-nowrap [&_small]:text-[#95a6bd] [&.selected]:rounded-[8px] [&.selected]:border [&.selected]:border-[#567bff] [&.selected]:bg-[#151e30] ${finding === active ? "selected" : ""}`}
                      onClick={() =>
                        void navigate({
                          search: (previous) => ({ ...previous, finding: finding.id }),
                          replace: true,
                        })
                      }
                    >
                      <SeverityBadge value={finding.severity} />
                      <span>
                        <strong>{finding.title}</strong>
                        <small>{finding.problem}</small>
                      </span>
                      <Person actorId={finding.found_by_actor_id} />
                      <span className="muted text-[#a2aec2]">
                        Round {finding.round} · {finding.label} ·{" "}
                        {findingStatusLabel[finding.status]}
                      </span>
                    </button>
                  ))}
              </section>
            ))
          )}
        </div>
        {active && <FindingDetail key={active.id} finding={active} />}
      </div>
    </>
  );
}
