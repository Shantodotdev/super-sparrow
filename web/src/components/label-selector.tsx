import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useProject } from "../hooks/use-project";
import { apiWrite } from "../lib/api";
import type { Label } from "../lib/types";
import { labelsQuery } from "../queries/project";
import { ResourceSelector } from "./resource-selector";

type LabelSelectorProps = {
  projectId: string;
  selected: string[];
  onChange: (ids: string[]) => Promise<unknown>;
};

/** Picks labels of one project, creating a new project label when the search has no match. */
export function LabelSelector({ projectId, selected, onChange }: LabelSelectorProps) {
  const project = useProject();
  const client = useQueryClient();
  const { data: labels } = useSuspenseQuery(labelsQuery);

  return (
    <ResourceSelector
      label="Labels"
      multiple
      colors
      options={labels.filter((label) => label.project_id === projectId)}
      selected={selected}
      onChange={onChange}
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
  );
}
