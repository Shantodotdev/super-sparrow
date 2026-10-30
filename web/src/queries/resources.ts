import { apiGet } from "../lib/api";
import type { ActivityEvent, Finding, Issue } from "../lib/types";

export type Project = { id: string; key: string; name: string };

export async function getProjects(): Promise<Project[]> {
  return (await apiGet<{ projects: Project[] }>("/api/projects")).projects;
}

/** Reads every issue page without losing records beyond the API page size. */
export async function getIssues(): Promise<Issue[]> {
  const projects = await getProjects();
  return (
    await Promise.all(
      projects.map(async (project) => {
        const issues: Issue[] = [];
        let before: number | null = null;
        do {
          const page: { issues: Issue[]; next_before: number | null } = await apiGet(
            `/api/projects/${encodeURIComponent(project.key)}/issues?limit=200${before === null ? "" : `&before=${before}`}`,
          );
          issues.push(...page.issues);
          before = page.next_before;
        } while (before !== null);
        return issues;
      }),
    )
  ).flat();
}

export async function getFindings(): Promise<Finding[]> {
  const projects = await getProjects();
  return (
    await Promise.all(
      projects.map(
        async (project) =>
          (
            await apiGet<{ findings: Finding[] }>(
              `/api/projects/${encodeURIComponent(project.key)}/findings?include_duplicates=true`,
            )
          ).findings,
      ),
    )
  ).flat();
}

/** Reads the complete activity history using sequence cursors. */
export async function getEvents(): Promise<ActivityEvent[]> {
  const events: ActivityEvent[] = [];
  let after = 0;
  for (;;) {
    const page = await apiGet<{ events: ActivityEvent[]; next_after: number | null }>(
      `/api/events?limit=200&after=${after}`,
    );
    events.push(...page.events);
    if (page.next_after === null || page.next_after <= after) return events;
    after = page.next_after;
  }
}

export function findingPath(finding: Finding): string {
  return `/api/issues/${encodeURIComponent(finding.issue_key)}/reviews/${finding.round}/findings/${finding.number}`;
}
