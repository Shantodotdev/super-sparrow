import { useEffect, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { TableKit } from "@tiptap/extension-table";
import Image from "@tiptap/extension-image";
import Placeholder from "@tiptap/extension-placeholder";
import { useQueryClient } from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import { PlanAutosave, type PlanVersion } from "../lib/plan-autosave";
import type { Issue } from "../lib/types";
import { issueQuery, issuesQuery } from "../queries/issues";
import { useToast } from "./toast";
import "./issue-plan.css";

/** Reads only valid local editor drafts; unavailable browser storage never prevents editing. */
function readDraft(key: string): PlanVersion | undefined {
  try {
    const value = JSON.parse(sessionStorage.getItem(key) ?? "null");
    if (value && typeof value.plan === "string" && Number.isInteger(value.revision)) return value;
  } catch {
    return undefined;
  }
}

/** Keeps unsaved Markdown across reloads without treating browser storage as server state. */
function storeDraft(key: string, draft: PlanVersion | null): void {
  try {
    if (draft) sessionStorage.setItem(key, JSON.stringify(draft));
    else sessionStorage.removeItem(key);
  } catch {
    // Private browsing or a full storage quota must not interrupt editing or saving.
  }
}

/** An inline Markdown editor that saves serially using the server's plan revision. */
export function IssuePlan({ issue }: { issue: Issue }) {
  const client = useQueryClient();
  const { notify } = useToast();
  const [autosave] = useState(() => {
    const draftKey = `sparrow:plan-draft:${issue.id}`;
    return new PlanAutosave({
      initial: { plan: issue.plan ?? "", revision: issue.plan_revision },
      draft: readDraft(draftKey),
      persist: (draft) => storeDraft(draftKey, draft),
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
    extensions: [
      StarterKit.configure({
        underline: false,
        link: { openOnClick: false, HTMLAttributes: { rel: "noopener noreferrer nofollow" } },
      }),
      Markdown,
      TaskList,
      TaskItem.configure({ nested: true }),
      TableKit,
      Image,
      Placeholder.configure({ placeholder: "Add a description…" }),
    ],
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
