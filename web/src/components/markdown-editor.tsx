import { useEffect } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { TableKit } from "@tiptap/extension-table";
import Image from "@tiptap/extension-image";
import Placeholder from "@tiptap/extension-placeholder";
import "./issue-plan.css";

/** The Markdown editor's extensions, shared by every place that edits an issue's Markdown. */
export function markdownExtensions(placeholder: string) {
  return [
    StarterKit.configure({
      underline: false,
      link: { openOnClick: false, HTMLAttributes: { rel: "noopener noreferrer nofollow" } },
    }),
    Markdown,
    TaskList,
    TaskItem.configure({ nested: true }),
    TableKit,
    Image,
    Placeholder.configure({ placeholder }),
  ];
}

type MarkdownEditorProps = {
  label: string;
  placeholder: string;
  value: string;
  onChange: (markdown: string) => void;
  className?: string;
};

/** A rich Markdown editor whose value is the Markdown text, so it can be controlled like an input. */
export function MarkdownEditor({
  label,
  placeholder,
  value,
  onChange,
  className = "",
}: MarkdownEditorProps) {
  const editor = useEditor({
    extensions: markdownExtensions(placeholder),
    content: value,
    contentType: "markdown",
    editorProps: {
      handlePaste: (_view, event) => {
        const text = event.clipboardData?.getData("text/plain");
        if (!text || event.clipboardData?.getData("text/html")) return false;
        editor?.commands.insertContent(text, { contentType: "markdown" });
        return true;
      },
      attributes: { "aria-label": label, role: "textbox", "aria-multiline": "true" },
    },
    onUpdate: ({ editor: current }) => onChange(current.getMarkdown()),
  });

  // Applies outside changes, such as a reset after creating an issue, without echoing them back.
  useEffect(() => {
    if (editor && editor.getMarkdown() !== value) {
      editor.commands.setContent(value, { contentType: "markdown", emitUpdate: false });
    }
  }, [editor, value]);

  return <EditorContent editor={editor} className={`issue-plan-editor ${className}`} />;
}
