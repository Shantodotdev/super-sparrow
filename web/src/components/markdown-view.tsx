import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { TableKit } from "@tiptap/extension-table";

const viewExtensions = [
  StarterKit.configure({
    underline: false,
    link: {
      openOnClick: true,
      HTMLAttributes: { rel: "noopener noreferrer nofollow", target: "_blank" },
    },
  }),
  Markdown,
  TaskList,
  TaskItem.configure({ nested: true }),
  TableKit,
];

/**
 * Renders Markdown read-only through the editor's schema, so raw HTML in the text is never
 * inserted into the page. Images are left out because the text is untrusted and an image would
 * make the viewer's browser fetch an address the author chose.
 */
export function MarkdownView({
  markdown,
  className = "",
}: {
  markdown: string;
  className?: string;
}) {
  const editor = useEditor(
    {
      extensions: viewExtensions,
      content: markdown,
      contentType: "markdown",
      editable: false,
    },
    [markdown],
  );

  return <EditorContent editor={editor} className={`issue-plan-editor ${className}`} />;
}
