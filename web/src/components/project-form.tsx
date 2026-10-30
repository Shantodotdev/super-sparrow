import { Input } from "./input";
import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { apiWrite } from "../lib/api";
import type { Project } from "../queries/resources";
import { projectsQuery } from "../queries/project";

/** Suggests an API-compatible key from the name's initials or its first word. */
function projectKey(name: string): string {
  const words = name.toUpperCase().match(/[A-Z][A-Z0-9]*/g) ?? [];
  const initials = words.map((word) => word[0]).join("");
  return (initials.length > 1 ? initials : (words[0] ?? "")).slice(0, initials.length > 1 ? 10 : 3);
}

/** Creates a project and opens its issue list only after the server has saved it. */
export function ProjectForm({ onCreated }: { onCreated?: () => void }) {
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [keyEdited, setKeyEdited] = useState(false);
  const client = useQueryClient();
  const navigate = useNavigate();
  const create = useMutation({
    mutationFn: () =>
      apiWrite<{ project: Project }>("/api/projects", "POST", {
        key: key.trim(),
        name: name.trim(),
      }),
    onSuccess: async ({ project }) => {
      client.setQueryData(projectsQuery.queryKey, (projects) => [...(projects ?? []), project]);
      await client.invalidateQueries({ queryKey: projectsQuery.queryKey });
      await navigate({ to: "/$projectSlug/issues", params: { projectSlug: project.slug } });
      onCreated?.();
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!create.isPending && name.trim() && key.trim()) create.mutate();
  };

  return (
    <form className="grid gap-4" onSubmit={submit}>
      <label className="m-0 grid gap-3 text-base text-[#cbd4e4]">
        Project name
        <Input
          className="h-14 w-full px-4 text-lg"
          name="projectName"
          autoFocus
          required
          maxLength={200}
          placeholder="e.g. Website redesign"
          value={name}
          onChange={(event) => {
            setName(event.target.value);
            if (!keyEdited) setKey(projectKey(event.target.value));
          }}
          disabled={create.isPending}
        />
      </label>
      <label className="m-0 grid gap-3 text-base text-[#cbd4e4]">
        Project key
        <Input
          className="h-14 w-full px-4 text-lg"
          name="projectKey"
          required
          minLength={2}
          maxLength={10}
          pattern="[A-Z][A-Z0-9]{1,9}"
          placeholder="e.g. WEB"
          value={key}
          onChange={(event) => {
            const value = event.target.value.toUpperCase();
            setKeyEdited(value !== "");
            setKey(value);
          }}
          disabled={create.isPending}
        />
      </label>
      {create.error && (
        <p role="alert" className="text-sm text-[#ff9999]">
          {create.error.message}
        </p>
      )}
      <button
        disabled={create.isPending || !name.trim() || !key.trim()}
        className="rounded-xl border border-[#5f7fff] bg-[#2455eb] px-5 py-3 font-medium text-white shadow-[0_4px_24px_#2455eb35] hover:bg-[#3566f6] disabled:opacity-50"
      >
        {create.isPending ? "Creating project…" : "Create project"}
      </button>
    </form>
  );
}
