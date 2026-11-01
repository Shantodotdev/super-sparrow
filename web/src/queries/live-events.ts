import type { QueryClient } from "@tanstack/react-query";
import type { ActivityEvent, Issue } from "../lib/types";
import type { Project } from "./resources";

/** Refreshes affected cached resources once per event batch, retaining unrelated project data. */
export function refreshLiveEvents(client: QueryClient, events: ActivityEvent[]): void {
  const projects = client.getQueryData<Project[]>(["projects"]) ?? [];
  const affected = new Set<string>(["activity"]);
  const projectKeys = new Set<string>();
  const issueIDs = new Set<string>();
  for (const event of events) {
    const project = projects.find((p) => p.id === event.project_id);
    if (project) projectKeys.add(project.key);
    switch (event.target_type) {
      case "issue":
        if (event.kind === "issue.created") {
          const issue =
            typeof event.payload.key === "string"
              ? client.getQueryData<Issue>(["issue", event.payload.key])
              : undefined;
          if (issue?.id === event.target_id) break;
        }
        affected.add("issues");
        issueIDs.add(event.target_id);
        break;
      case "finding":
      case "review_round":
      case "comment":
        affected.add("issues");
        affected.add("findings");
        affected.add("review-rounds");
        affected.add("comments");
        if (typeof event.payload.issue_id === "string") issueIDs.add(event.payload.issue_id);
        break;
      case "project":
        affected.add("projects");
        affected.add("issues");
        affected.add("labels");
        affected.add("milestones");
        break;
      case "label":
      case "milestone":
        affected.add(event.target_type === "label" ? "labels" : "milestones");
        affected.add("issues");
        break;
      case "actor":
        affected.add("actors");
        affected.add("me");
        affected.add("tokens");
        affected.add("sessions");
        break;
    }
  }
  void client.invalidateQueries({
    predicate: (query) => {
      const [resource, key] = query.queryKey;
      if (resource === "subissues") {
        return events.some(
          (event) =>
            event.kind.startsWith("issue.subissue_") &&
            (client.getQueryData<Issue>(["issue", key])?.id === event.target_id ||
              (query.state.data as { issue_id: string }[] | undefined)?.some(
                (item) => item.issue_id === event.target_id,
              )),
        );
      }
      if (resource === "issue") {
        const issue = query.state.data as Issue | undefined;
        return (
          !!issue &&
          (issueIDs.has(issue.id) ||
            events.some(
              (event) =>
                event.project_id === issue.project_id &&
                ["finding", "review_round", "comment"].includes(event.target_type),
            ))
        );
      }
      if (typeof resource !== "string" || !affected.has(resource)) return false;
      return key === undefined || typeof key !== "string" || projectKeys.has(key);
    },
  });
}
