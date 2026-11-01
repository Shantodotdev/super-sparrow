import { useEffect, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import { useQueryClient } from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import { PlanAutosave } from "../lib/plan-autosave";
import { readPlanDraft, storePlanDraft } from "../lib/plan-draft";
import type { Issue } from "../lib/types";
import { issueQuery, issuesQuery } from "../queries/issues";
import { markdownExtensions } from "./markdown-editor";
import { useToast } from "./toast";

/** An inline Markdown editor that saves serially using the server's plan revision. */
export function IssuePlan({ issue }: { issue: Issue }) {
  const client = useQueryClient();
  const { notify } = useToast();
  const [autosave] = useState(() => {
    const draftKey = `sparrow:plan-draft:${issue.id}`;
    return new PlanAutosave({
      initial: { plan: issue.plan ?? "", revision: issue.plan_revision },
      draft: readPlanDraft(draftKey),
      persist: (draft) => storePlanDraft(draftKey, draft),
      save: async ({ plan, revision }) => {
        const { issue: saved } = await apiWrite<{ issue: Issue }>(
          `/api/issues/${encodeURIComponent(issue.key)}`,
          "PATCH",
          { plan, plan_revision: revision },
        );
        client.setQueryData(issueQuery(issue.key).queryKey, (current) =>
          current && current.plan_revision > saved.plan_revision ? current : saved,
        );
        void client.invalidateQueries({ queryKey: issuesQuery.queryKey });
        void client.invalidateQueries({ queryKey: ["events"] });
        return { plan: saved.plan ?? "", revision: saved.plan_revision };
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
      attributes: { "aria-label": "Issue plan", role: "textbox", "aria-multiline": "true" },
    },
    onUpdate: ({ editor: current }) => autosave.edit(current.getMarkdown()),
    onBlur: () => void autosave.flush(),
  });

  useEffect(() => autosave.subscribe(setState), [autosave]);

  useEffect(() => {
    if (autosave.receive({ plan: issue.plan ?? "", revision: issue.plan_revision })) {
      editor?.commands.setContent(autosave.state.plan, {
        contentType: "markdown",
        emitUpdate: false,
      });
    }
  }, [issue.plan, issue.plan_revision, autosave, editor, state.dirty, state.revision]);

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
    <div className="mt-6 max-w-[850px]">
      <EditorContent editor={editor} className="issue-plan-editor" />
      {state.error && (
        <div role="alert" className="mt-3 text-sm text-[#f2a9ae]">
          <p>{state.error.message} Your edits remain in the editor.</p>
          <div className="mt-2 flex gap-3">
            <button
              type="button"
              className="text-[#a7c0ff]"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(autosave.state.plan);
                } catch {
                  notify("Could not copy. Select and copy the plan directly.");
                }
              }}
            >
              Copy my Markdown
            </button>
            <button
              type="button"
              className="text-[#a7c0ff] disabled:opacity-50"
              disabled={state.saving}
              onClick={async () => {
                try {
                  const { issue: latest } = await apiGet<{ issue: Issue }>(
                    `/api/issues/${encodeURIComponent(issue.key)}`,
                  );
                  autosave.reset({ plan: latest.plan ?? "", revision: latest.plan_revision });
                  editor?.commands.setContent(latest.plan ?? "", {
                    contentType: "markdown",
                    emitUpdate: false,
                  });
                  client.setQueryData(issueQuery(issue.key).queryKey, latest);
                } catch (error) {
                  notify(error instanceof Error ? error.message : "Could not load the latest plan");
                }
              }}
            >
              Use latest plan
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
