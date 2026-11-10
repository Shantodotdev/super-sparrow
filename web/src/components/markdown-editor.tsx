import { useEffect, useRef } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { TableKit } from "@tiptap/extension-table";
import Image from "@tiptap/extension-image";
import Placeholder from "@tiptap/extension-placeholder";
import { transferredFiles } from "../lib/attachments";

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
  /** Receives files pasted into the editor, which are not inserted into the text. */
  onFiles?: (files: File[]) => void;
};

/** A rich Markdown editor whose value is the Markdown text, so it can be controlled like an input. */
export function MarkdownEditor({
  label,
  placeholder,
  value,
  onChange,
  className = "",
  onFiles,
}: MarkdownEditorProps) {
  // The editor keeps its first props; the ref lets its paste handler read the current callback.
  const takeFiles = useRef(onFiles);
  takeFiles.current = onFiles;

  const editor = useEditor({
    extensions: markdownExtensions(placeholder),
    content: value,
    contentType: "markdown",
    editorProps: {
      handlePaste: (_view, event) => {
        const files = transferredFiles(event.clipboardData);
        if (files.length > 0 && takeFiles.current) {
          takeFiles.current(files);
          return true;
        }
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
