import { useEffect, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import { useQueryClient } from "@tanstack/react-query";
import { PlanAutosave } from "../lib/plan-autosave";
import { readPlanDraft, storePlanDraft } from "../lib/plan-draft";
import type { Issue, Subissue } from "../lib/types";
import { subissuesQuery, useSaveSubissue } from "../queries/subissues";
import { markdownExtensions } from "./markdown-editor";
import { useToast } from "./toast";

/** Autosaves Markdown descriptions with revision checks and recoverable drafts, like the main issue plan. */
export function SubissueDescription({ issue, subissue }: { issue: Issue; subissue: Subissue }) {
  const client = useQueryClient();
  const { notify } = useToast();
  const save = useSaveSubissue(issue.key);
  const [autosave] = useState(() => {
    const draftKey = `sparrow:subissue-description-draft:${subissue.id}`;
    return new PlanAutosave({
      initial: { plan: subissue.description, revision: subissue.description_revision },
      draft: readPlanDraft(draftKey),
      persist: (draft) => storePlanDraft(draftKey, draft),
      save: async ({ plan, revision }) => {
        const { subissue: saved } = await save.mutateAsync({
          id: subissue.id,
          description: plan,
          description_revision: revision,
        });
        return { plan: saved.description, revision: saved.description_revision };
      },
    });
  });
  const [state, setState] = useState(autosave.state);
  const editor = useEditor({
    extensions: markdownExtensions("Add a description…"),
    content: autosave.state.plan,
    contentType: "markdown",
    editorProps: {
      handlePaste: (_view, event) => {
        const text = event.clipboardData?.getData("text/plain");
        if (!text || event.clipboardData?.getData("text/html")) return false;
        editor?.commands.insertContent(text, { contentType: "markdown" });
        return true;
      },
      attributes: {
        "aria-label": "Subissue description",
        role: "textbox",
        "aria-multiline": "true",
      },
    },
    onUpdate: ({ editor: current }) => autosave.edit(current.getMarkdown()),
    onBlur: () => void autosave.flush(),
  });

  useEffect(() => autosave.subscribe(setState), [autosave]);

  useEffect(() => {
    if (autosave.receive({ plan: subissue.description, revision: subissue.description_revision })) {
      editor?.commands.setContent(autosave.state.plan, {
        contentType: "markdown",
        emitUpdate: false,
      });
    }
  }, [
    subissue.description,
    subissue.description_revision,
    autosave,
    editor,
    state.dirty,
    state.revision,
  ]);

  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (!autosave.state.dirty && !autosave.state.saving) return;
      void autosave.flush();
      event.preventDefault();
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => {
      window.removeEventListener("beforeunload", beforeUnload);
      void autosave.flush();
    };
  }, [autosave]);

  return (
    <div
      className="mt-6 flex min-h-[360px] flex-1 cursor-text flex-col border-t border-[#252a32] pt-6"
      onClick={(event) => {
        if (event.target === event.currentTarget) editor?.commands.focus("end");
      }}
    >
      <EditorContent
        editor={editor}
        className="issue-plan-editor flex flex-1 flex-col [&_.tiptap]:min-h-[360px]! [&_.tiptap]:flex-1 [&_.tiptap]:cursor-text"
        onClick={(event) => {
          if (event.target === event.currentTarget) editor?.commands.focus("end");
        }}
      />
      {state.error && (
        <div role="alert" className="mt-3 text-sm text-[#f2a9ae]">
          <p>{state.error.message} Your edits are kept as a draft.</p>
          <div className="mt-2 flex gap-3">
            <button
              type="button"
              className="text-[#a7c0ff]"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(autosave.state.plan);
                } catch {
                  notify("Could not copy. Select and copy the description directly.");
                }
              }}
            >
              Copy my Markdown
            </button>
            <button
              type="button"
              disabled={state.saving}
              className="text-[#a7c0ff] disabled:opacity-50"
              onClick={async () => {
                try {
                  const latest = (
                    await client.fetchQuery({ ...subissuesQuery(issue.key), staleTime: 0 })
                  ).find((item) => item.id === subissue.id);
                  if (!latest) throw new Error("Subissue no longer exists");
                  autosave.reset({
                    plan: latest.description,
                    revision: latest.description_revision,
                  });
                  editor?.commands.setContent(latest.description, {
                    contentType: "markdown",
                    emitUpdate: false,
                  });
                } catch (error) {
                  notify(
                    error instanceof Error
                      ? error.message
                      : "Could not load the latest description",
                  );
                }
              }}
            >
              Use latest description
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
