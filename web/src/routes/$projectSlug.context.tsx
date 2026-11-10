import { useState } from "react";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { formatDistanceToNow } from "date-fns";
import { Icon } from "../components/icon";
import { Input } from "../components/input";
import { Modal } from "../components/modal";
import { useProject } from "../hooks/use-project";
import { projectContextsQuery, useCreateContext } from "../queries/contexts";
import { loadQuery } from "../queries/options";
import { projectsQuery } from "../queries/project";

export const Route = createFileRoute("/$projectSlug/context")({
  staticData: { section: "context" },
  loader: async ({ context, params }) => {
    const projects = await loadQuery(context.queryClient, projectsQuery);
    const project = projects.find((item) => item.slug === params.projectSlug);
    if (project) void context.queryClient.prefetchQuery(projectContextsQuery(project.key));
  },
  component: ContextsPage,
});

/**
 * A project's contexts: notes, inspiration, and screenshots kept outside the issue tracker. Each
 * has a key, such as WP-C3, that people hand to coding agents.
 */
function ContextsPage() {
  const project = useProject();
  const navigate = useNavigate();
  const contexts = useQuery(projectContextsQuery(project.key));
  const createContext = useCreateContext();
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");

  const query = search.trim().toLowerCase();
  const shown = (contexts.data ?? []).filter(
    (item) => item.title.toLowerCase().includes(query) || item.key.toLowerCase() === query,
  );

  const create = async () => {
    if (!title.trim()) return;
    try {
      const { context } = await createContext({
        projectKey: project.key,
        title: title.trim(),
        body: "",
      });
      setCreating(false);
      setTitle("");
      void navigate({
        to: "/$projectSlug/context/$context",
        params: { projectSlug: project.slug, context: context.key },
      });
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Could not create the context");
    }
  };

  return (
    <div className="px-[26px] pt-[24px] pb-10 max-[700px]:px-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative max-w-[360px] min-w-[180px] flex-[1_1_220px]">
          <Icon
            name="search"
            className="pointer-events-none absolute top-1/2 left-4 -translate-y-1/2 text-[#8999ad]"
          />
          <Input
            className="w-full rounded-full pl-12"
            placeholder="Search context..."
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            aria-label="Search context"
          />
        </div>
        <button
          type="button"
          className="ml-auto inline-flex items-center gap-2 rounded-full border border-[#3264ff] bg-[#2455eb] px-4 py-2 font-[550] text-white hover:bg-[#3566f6]"
          onClick={() => {
            setError("");
            setCreating(true);
          }}
        >
          <Icon name="plus" /> New context
        </button>
      </div>
      <p className="mt-4 mb-6 max-w-[720px] text-[14px] text-[#93a0b2]">
        Keep inspiration, research, and screenshots here. Give a coding agent a context key, such as{" "}
        <code>{project.key}-C1</code>, and it can read the notes and see the images with{" "}
        <code>get_context</code>. Press <kbd>Ctrl</kbd> <kbd>Q</kbd> anywhere to capture one
        quickly.
      </p>

      {contexts.error && (
        <p role="alert" className="text-[#f38b8b]">
          {contexts.error.message}
        </p>
      )}
      {contexts.data && shown.length === 0 && (
        <p className="py-16 text-center text-[#8e9bac]">
          {query ? "No context matches your search." : "No context yet."}
        </p>
      )}
      <ul className="m-0 grid list-none gap-2 p-0">
        {shown.map((item) => (
          <li key={item.id}>
            <Link
              to="/$projectSlug/context/$context"
              params={{ projectSlug: project.slug, context: item.key }}
              className="flex items-center gap-4 rounded-[10px] border border-[#252a33] bg-[#101216] px-4 py-3 text-inherit no-underline hover:border-[#3a4252] hover:bg-[#151920]"
            >
              <Icon name="file-text" className="text-[#7890ff]" />
              <span className="w-[72px] shrink-0 font-mono text-[13px] text-[#8e9bac]">
                {item.key}
              </span>
              <span className="min-w-0 flex-1 truncate text-[#e9edf5]">{item.title}</span>
              {item.issue_key && (
                <span className="shrink-0 rounded-full border border-[#303744] px-2 py-0.5 text-[12px] text-[#a9b8cb]">
                  {item.issue_key}
                </span>
              )}
              <span className="shrink-0 text-[12px] text-[#7d8899]">
                {formatDistanceToNow(new Date(item.updated_at), { addSuffix: true })}
              </span>
            </Link>
          </li>
        ))}
      </ul>

      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        label="New context"
        className="w-[min(100%,480px)] rounded-2xl border border-[#343740] bg-[#191a1e] p-5 shadow-[0_24px_80px_#000b]"
      >
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void create();
          }}
        >
          <h2 className="mt-0 mb-4 text-[15px]">New context in {project.name}</h2>
          {error && (
            <p role="alert" className="text-[13px] text-[#f2a9ae]">
              {error}
            </p>
          )}
          <Input
            autoFocus
            aria-label="Context title"
            className="w-full"
            placeholder="Title"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
          <div className="mt-4 flex justify-end">
            <button
              className="rounded-full border border-[#3264ff] bg-[#2455eb] px-4 py-2 font-[550] text-white hover:bg-[#3566f6] disabled:opacity-50"
              disabled={!title.trim()}
            >
              Create
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
