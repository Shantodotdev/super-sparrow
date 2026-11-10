import { useEffect, useRef } from "react";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { TableKit } from "@tiptap/extension-table";
import Image from "@tiptap/extension-image";
import Placeholder from "@tiptap/extension-placeholder";
import { apiUploadImage } from "../lib/api";

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

/** The image files in a paste or a drop, if there are any. */
export function imageFiles(data: DataTransfer | null | undefined): File[] {
  return Array.from(data?.files ?? []).filter((file) => file.type.startsWith("image/"));
}

/**
 * Uploads images to a project and shows each in the editor where it was pasted or dropped.
 * Failures are reported through onError, and the rest of the images are still added.
 */
export async function insertUploadedImages(
  editor: Editor,
  projectKey: string,
  files: File[],
  onError: (message: string) => void,
): Promise<void> {
  for (const file of files) {
    try {
      const attachment = await apiUploadImage(projectKey, file);
      editor.chain().focus().setImage({ src: attachment.url, alt: "Screenshot" }).run();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Could not upload the image");
    }
  }
}

/** Appends an image to Markdown on its own paragraph. */
export function appendImage(markdown: string, url: string, alt = "Screenshot"): string {
  const image = `![${alt}](${url})`;
  return markdown.trim()
    ? `${markdown.trimEnd()}

${image}`
    : image;
}

type MarkdownEditorProps = {
  label: string;
  placeholder: string;
  value: string;
  onChange: (markdown: string) => void;
  className?: string;
  /** The project pasted and dropped images are uploaded to; without it, images are not taken. */
  projectKey?: string;
  onImageError?: (message: string) => void;
};

/** A rich Markdown editor whose value is the Markdown text, so it can be controlled like an input. */
export function MarkdownEditor({
  label,
  placeholder,
  value,
  onChange,
  className = "",
  projectKey,
  onImageError,
}: MarkdownEditorProps) {
  // The editor keeps its first props; the refs let its handlers read the current ones.
  const upload = useRef({ projectKey, onImageError });
  upload.current = { projectKey, onImageError };
  const takeImages = (files: File[]) => {
    const { projectKey: key, onImageError: report } = upload.current;
    if (!key || !editor || files.length === 0) return false;
    void insertUploadedImages(editor, key, files, report ?? (() => undefined));
    return true;
  };

  const editor = useEditor({
    extensions: markdownExtensions(placeholder),
    content: value,
    contentType: "markdown",
    editorProps: {
      handlePaste: (_view, event) => {
        if (takeImages(imageFiles(event.clipboardData))) return true;
        const text = event.clipboardData?.getData("text/plain");
        if (!text || event.clipboardData?.getData("text/html")) return false;
        editor?.commands.insertContent(text, { contentType: "markdown" });
        return true;
      },
      handleDrop: (_view, event) => takeImages(imageFiles(event.dataTransfer)),
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
