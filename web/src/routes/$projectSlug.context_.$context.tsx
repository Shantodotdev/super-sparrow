import { useKeyboardShortcuts } from "../components/keyboard-shortcuts-provider";
import { matchesChord } from "../lib/shortcuts";
import { useEffect, useState } from "react";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../lib/api";
import type { ContextNote } from "../lib/types";
import { Icon } from "../components/icon";
import { MarkdownEditor, appendImage } from "../components/markdown-editor";
import { NotFoundState } from "../components/not-found-state";
import { ScreenshotButton } from "../components/screenshot-button";
import { Select } from "../components/select";
import { useToast } from "../components/toast";
import { useProject } from "../hooks/use-project";
import { contextQuery, useDeleteContext, useUpdateContext } from "../queries/contexts";
import { projectIssuesQuery } from "../queries/project-resources";

export const Route = createFileRoute("/$projectSlug/context_/$context")({
  staticData: { section: "context" },
  loader: ({ context, params }) => {
    void context.queryClient.prefetchQuery(contextQuery(params.context));
  },
  component: ContextPage,
});

function ContextPage() {
  const { context: key } = Route.useParams();
  const found = useQuery(contextQuery(key));

  if (found.error instanceof ApiError && [400, 404].includes(found.error.status)) {
    return <NotFoundState title="Context not found" description={`There is no context ${key}.`} />;
  }
  if (found.error) {
    return (
      <p role="alert" className="p-8 text-[#f38b8b]">
        {found.error.message}
      </p>
    );
  }
  if (!found.data) return null;
  return <ContextEditor key={found.data.id} saved={found.data} />;
}

/**
 * Edits one context. The body is saved on request, with the personal save shortcut or the Save button, and carries
 * the revision it started from, so an edit made elsewhere in the meantime is never overwritten.
 */
function ContextEditor({ saved }: { saved: ContextNote }) {
  const { bindings } = useKeyboardShortcuts();
  const project = useProject();
  const navigate = useNavigate();
  const { notify } = useToast();
  const update = useUpdateContext(saved.key);
  const remove = useDeleteContext(saved);
  const issues = useQuery(projectIssuesQuery(project.key));
  const [title, setTitle] = useState(saved.title);
  const [body, setBody] = useState(saved.body);
  // The saved body the draft started from; an edit is saved against its revision.
  const [base, setBase] = useState({ body: saved.body, revision: saved.body_revision });
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  // The server trims what it saves, so surrounding whitespace is not an unsaved change.
  const dirty = body.trim() !== base.body.trim();

  // Takes in edits made elsewhere while there are no unsaved edits here.
  useEffect(() => {
    if (saved.body_revision <= base.revision || dirty) return;
    setBody(saved.body);
    setBase({ body: saved.body, revision: saved.body_revision });
  }, [saved.body, saved.body_revision, base.revision, dirty]);

  useEffect(() => setTitle(saved.title), [saved.title]);

  const save = async () => {
    if (!dirty || update.isPending) return;
    setError("");
    try {
      const { context } = await update.mutateAsync({ body, body_revision: base.revision });
      setBase({ body: context.body, revision: context.body_revision });
      setConflict(false);
      notify("Saved");
    } catch (failure) {
      if (failure instanceof ApiError && failure.status === 409) setConflict(true);
      else setError(failure instanceof Error ? failure.message : "Could not save");
    }
  };

  const saveTitle = () => {
    if (title.trim() && title.trim() !== saved.title) update.mutate({ title: title.trim() });
    else setTitle(saved.title);
  };

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!event.defaultPrevented && matchesChord(event, bindings.saveContext)) {
        event.preventDefault();
        void save();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const projectIssues = (issues.data ?? []).filter((issue) => issue.project_id === project.id);

  return (
    <div className="mx-auto max-w-[920px] px-[26px] pt-[24px] pb-16 max-[700px]:px-4">
      <div className="mb-5 flex items-center gap-2 text-[13px] text-[#a9b8cb]">
        <Link
          to="/$projectSlug/context"
          params={{ projectSlug: project.slug }}
          className="text-inherit no-underline hover:text-[#d2e0ff]"
        >
          Context
        </Link>
        <Icon name="chevron-right" />
        <button
          type="button"
          className="rounded border-0 bg-transparent px-1 font-mono text-inherit hover:bg-[#1d2129] hover:text-[#d2e0ff]"
          title="Copy the key to give a coding agent"
          onClick={() => {
            void navigator.clipboard.writeText(saved.key).then(
              () => notify(`Copied ${saved.key}`),
              () => notify("Could not copy"),
            );
          }}
        >
          {saved.key}
        </button>
        <span className="ml-auto flex items-center gap-2">
          <Select
            variant="chip"
            icon="list-todo"
            label="Linked issue"
            value={saved.issue_id}
            onChange={(issueId) => update.mutate({ issue_id: issueId })}
            options={[
              { value: "", label: "No linked issue" },
              ...projectIssues.map((issue) => ({
                value: issue.id,
                label: `${issue.key} ${issue.title}`,
              })),
            ]}
          />
          <button
            type="button"
            className="rounded-full border border-[#4a2b30] bg-transparent px-3 py-1.5 text-[12px] text-[#f2a9ae] hover:bg-[#2a1719]"
            onClick={() => {
              if (!window.confirm(`Delete ${saved.key}? Its notes cannot be recovered.`)) return;
              remove.mutate(undefined, {
                onSuccess: () =>
                  void navigate({
                    to: "/$projectSlug/context",
                    params: { projectSlug: project.slug },
                  }),
                onError: (failure) => notify(failure.message),
              });
            }}
          >
            Delete
          </button>
        </span>
      </div>

      <input
        aria-label="Context title"
        className="w-full border-0 bg-transparent p-0 text-[26px] font-semibold text-[#f1f2f4] outline-none"
        value={title}
        onChange={(event) => setTitle(event.target.value)}
        onBlur={saveTitle}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
        }}
      />

      {conflict && (
        <div
          role="alert"
          className="mt-4 rounded-lg border border-[#5a4426] bg-[#211a10] p-3 text-[13px] text-[#f2cf87]"
        >
          Someone changed this context since you started editing. Your edits are still here.
          <button
            type="button"
            className="ml-3 border-0 bg-transparent p-0 text-[#a7c0ff] underline"
            onClick={() => {
              setBody(saved.body);
              setBase({ body: saved.body, revision: saved.body_revision });
              setConflict(false);
            }}
          >
            Discard mine and load theirs
          </button>
          <button
            type="button"
            className="ml-3 border-0 bg-transparent p-0 text-[#a7c0ff] underline"
            onClick={() => {
              setBase({ body: saved.body, revision: saved.body_revision });
              setConflict(false);
            }}
          >
            Keep mine
          </button>
        </div>
      )}
      {error && (
        <p role="alert" className="mt-4 text-[13px] text-[#f2a9ae]">
          {error}
        </p>
      )}

      <div className="sticky top-0 z-2 mt-5 flex items-center gap-2 border-b border-[#20242b] bg-[#08090b] py-2">
        <ScreenshotButton
          projectKey={project.key}
          onCaptured={(attachment) => setBody((current) => appendImage(current, attachment.url))}
          onError={setError}
        />
        <span className="text-[12px] text-[#7d8899]">Paste or drop images into the notes.</span>
        <span className="ml-auto text-[12px] text-[#7d8899]">
          {update.isPending ? "Saving…" : dirty ? "Unsaved changes" : "Saved"}
        </span>
        <button
          type="button"
          className="rounded-full border border-[#3264ff] bg-[#2455eb] px-4 py-1.5 text-[13px] font-[550] text-white hover:bg-[#3566f6] disabled:opacity-50"
          disabled={!dirty || update.isPending}
          onClick={() => void save()}
          title={bindings.saveContext}
        >
          Save
        </button>
      </div>

      <MarkdownEditor
        label="Context notes"
        placeholder="Write notes, paste links, paste or drop screenshots…"
        value={body}
        onChange={setBody}
        projectKey={project.key}
        onImageError={setError}
        className="mt-4 [&_.tiptap]:min-h-[50vh]!"
      />
    </div>
  );
}
