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
      <div className="findings-toolbar">
        <PageHeader
          crumb="Projects / Sparrow"
          title="Open findings"
          description="Findings across review rounds that need attention."
        />
        <div className="findings-filters filters">
          <input
            className="filter-search"
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
      <div className="review-split findings-split">
        <div>
          {visible.length === 0 ? (
            <div className="empty-state">
              <h2>No findings found</h2>
              <p>Try changing your filters.</p>
            </div>
          ) : (
            [...new Set(visible.map((finding) => finding.issue))].map((issueKey) => (
              <section key={issueKey}>
                <div className="findings-group">
                  <Link to="/$issue" params={{ issue: issueKey }} className="text-link">
                    {issueKey}
                  </Link>{" "}
                  <strong>{issueTitles.get(issueKey)}</strong>
                </div>
                {visible
                  .filter((finding) => finding.issue === issueKey)
                  .map((finding) => (
                    <button
                      key={`${finding.issue}-${finding.round}-${finding.id}`}
                      className={`finding-list-row ${finding === active ? "selected" : ""}`}
                      onClick={() => setSelected(visible.indexOf(finding))}
                    >
                      <SeverityBadge value={finding.severity} />
                      <span>
                        <strong>{finding.title}</strong>
                        <small>{finding.problem}</small>
                      </span>
                      <Person name={finding.author} />
                      <span className="muted">
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
