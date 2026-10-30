import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { IssueList } from "../components/issue-list";
import { PageHeader } from "../components/page-header";
import { issuesQuery } from "../queries/issues";
import { actorsQuery, meQuery } from "../queries/project";
import { loadQuery } from "../queries/options";
import type { IssueStatus } from "../lib/types";

export const Route = createFileRoute("/issues")({
  staticData: { section: "issues" },
  loader: async ({ context }) => {
    await Promise.all([
      loadQuery(context.queryClient, issuesQuery),
      loadQuery(context.queryClient, actorsQuery),
      loadQuery(context.queryClient, meQuery),
    ]);
  },
  component: MyIssuesPage,
});

/** Lists the signed-in person's assignments across all projects. */
function MyIssuesPage() {
  const { data: issues } = useSuspenseQuery(issuesQuery);
  const { data: me } = useSuspenseQuery(meQuery);
  const [collapsed, setCollapsed] = useState<IssueStatus[]>([]);
  const assigned = issues.filter((issue) => issue.assignee_actor_id === me.id);

  return (
    <>
      <PageHeader crumb="My issues" title="My issues" description="Assigned to you" />
      {!assigned.length && (
        <p className="px-[26px] py-8 text-[#a2aec2]">No issues assigned to you.</p>
      )}
      <IssueList
        issues={assigned}
        toolbarHeight={0}
        collapsed={collapsed}
        onToggleGroup={(group) =>
          setCollapsed((items) =>
            items.includes(group) ? items.filter((item) => item !== group) : [...items, group],
          )
        }
      />
    </>
  );
}
