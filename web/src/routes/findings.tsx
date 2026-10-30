import { useState } from "react";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { FindingDetail } from "../components/finding-detail";
import { Filter } from "../components/filter";
import { PageHeader } from "../components/page-header";
import { Person } from "../components/person";
import { SeverityBadge } from "../components/severity-badge";
import { findingsQuery } from "../queries/findings";
import { issuesQuery } from "../queries/issues";
import { reviewRoundsQuery } from "../queries/review-rounds";

export const Route = createFileRoute("/findings")({
  staticData: { section: "findings" },
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.query({ ...findingsQuery, staleTime: "static" }),
      context.queryClient.query({ ...issuesQuery, staleTime: "static" }),
      context.queryClient.query({ ...reviewRoundsQuery, staleTime: "static" }),
    ]);
  },
  component: FindingsPage,
});

const severities = ["Blocking", "Should fix", "Nit"];
const statuses = ["Open", "Fix", "Wontfix", "Later"];

/** Findings across every review round, grouped by issue, with the selected one on the right. */
function FindingsPage() {
  const { data: findings } = useSuspenseQuery(findingsQuery);
  const { data: issues } = useSuspenseQuery(issuesQuery);

  const [selected, setSelected] = useState(0);
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState("All");
  const [status, setStatus] = useState("Open");

  const visible = findings.filter(
    (finding) =>
      finding.title.toLowerCase().includes(query.toLowerCase()) &&
      (severity === "All" || finding.severity === severity) &&
      (status === "All" || finding.status === status),
  );
  const active = visible[selected] ?? visible[0];
  const issueTitles = new Map(issues.map((issue) => [issue.key, issue.title]));

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
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setSelected(0);
            }}
          />
          <Filter name="Severity" value={severity} onChange={setSeverity} options={severities} />
          <Filter name="Status" value={status} onChange={setStatus} options={statuses} />
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
            [...new Set(visible.map((finding) => finding.issue))].map((issueKey) => (
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
                  .filter((finding) => finding.issue === issueKey)
                  .map((finding) => (
                    <button
                      key={`${finding.issue}-${finding.round}-${finding.id}`}
                      className={`flex w-full items-center gap-[16px] border border-b border-transparent border-b-[#242932] bg-none p-[13px] text-left [&_.person-inline]:text-[12px] max-[1100px]:[&_.person-inline]:hidden max-[1100px]:[&_>_.muted]:hidden [&_>_span:nth-child(2)]:min-w-0 [&_>_span:nth-child(2)]:flex-1 [&_small]:mt-[5px] [&_small]:block [&_small]:overflow-hidden [&_small]:text-ellipsis [&_small]:whitespace-nowrap [&_small]:text-[#95a6bd] [&.selected]:rounded-[8px] [&.selected]:border [&.selected]:border-[#567bff] [&.selected]:bg-[#151e30] ${finding === active ? "selected" : ""}`}
                      onClick={() => setSelected(visible.indexOf(finding))}
                    >
                      <SeverityBadge value={finding.severity} />
                      <span>
                        <strong>{finding.title}</strong>
                        <small>{finding.problem}</small>
                      </span>
                      <Person name={finding.author} />
                      <span className="muted text-[#a2aec2]">
                        Round {finding.round} · {finding.id}
                      </span>
                    </button>
                  ))}
              </section>
            ))
          )}
        </div>
        {active && (
          <FindingDetail key={`${active.issue}-${active.round}-${active.id}`} finding={active} />
        )}
      </div>
    </>
  );
}
