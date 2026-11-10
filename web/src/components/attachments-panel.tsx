import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, File as FileIcon, FileText, Trash2 } from "lucide-react";
import { useFileDrop } from "../hooks/use-file-drop";
import { attachmentKind, formatBytes } from "../lib/attachments";
import { formatRelative } from "../lib/format";
import type { Attachment, Issue } from "../lib/types";
import {
  issueAttachmentsQuery,
  useAttachmentUploads,
  useDeleteAttachment,
  type UploadJob,
} from "../queries/attachments";
import { meQuery, useActor } from "../queries/project";
import { AttachmentButtons, UploadList } from "./attachment-queue";
import { Modal } from "./modal";

const iconButton =
  "grid place-items-center rounded-md border-0 bg-transparent p-1.5 text-[#9ba9bc] hover:bg-[#1b2027] hover:text-[#d7dfeb]";

/**
 * An issue's files, listed below its description. Files are added with the button,
 * or by dropping them on the panel, and upload straight away.
 */
export function AttachmentsPanel({ issue }: { issue: Issue }) {
  const attachments = useQuery(issueAttachmentsQuery(issue.key));
  const me = useQuery(meQuery);
  const uploads = useAttachmentUploads();
  const deleteAttachment = useDeleteAttachment(issue.key);
  const actor = useActor();
  const [problems, setProblems] = useState<string[]>([]);
  const [preview, setPreview] = useState<Attachment | null>(null);
  const [deleting, setDeleting] = useState<Attachment | null>(null);
  const keptDeleting = useRef<Attachment | null>(null);
  keptDeleting.current = deleting ?? keptDeleting.current;

  const upload = async (jobs: UploadJob[]) => {
    const failed = await uploads.start(issue.key, jobs);
    for (const job of jobs) {
      if (!failed.some((item) => item.id === job.id)) uploads.remove(job.id);
    }
  };

  const addFiles = (files: File[]) => {
    if (files.length === 0) return;
    const { added, rejected } = uploads.add(files);
    setProblems(rejected);
    void upload(added);
  };

  const { dragging, handlers } = useFileDrop(addFiles);
  const canDelete = me.data?.kind === "human";
  const items = attachments.data ?? [];

  return (
    <section
      aria-label="Attachments"
      tabIndex={0}
      className={`mt-6 max-w-[850px] rounded-xl border bg-[#0d0f12] p-3 outline-none focus-visible:border-[#3a4a78] ${dragging ? "border-[#426fff] bg-[#10141c]" : "border-[#222730]"}`}
      {...handlers}
    >
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="m-0 mr-auto text-[13px] font-semibold text-[#d7dfeb]">
          Attachments
          {items.length > 0 && (
            <span className="ml-2 font-normal text-[#9ba9bc]">{items.length}</span>
          )}
        </h2>
        <AttachmentButtons onFiles={addFiles} onError={(message) => setProblems([message])} />
      </div>

      {problems.length > 0 && (
        <div role="alert" className="mt-2 text-[12px] text-[#f2a9ae]">
          {problems.map((problem) => (
            <p key={problem} className="m-0">
              {problem}
            </p>
          ))}
        </div>
      )}
      {attachments.error && (
        <p role="alert" className="m-0 mt-2 text-[12px] text-[#f2a9ae]">
          {attachments.error.message}
        </p>
      )}

      <div className={`flex flex-col gap-2 ${items.length || uploads.jobs.length ? "mt-3" : ""}`}>
        <UploadList
          jobs={uploads.jobs}
          onRemove={(job) => uploads.remove(job.id)}
          onRetry={(job) => void upload([job])}
        />
        {items.map((attachment) => (
          <AttachmentRow
            key={attachment.id}
            attachment={attachment}
            uploader={actor(attachment.created_by_actor_id)?.display_name ?? "Unknown"}
            canDelete={canDelete}
            onPreview={() => setPreview(attachment)}
            onDelete={() => setDeleting(attachment)}
          />
        ))}
      </div>

      <Modal
        open={preview !== null}
        onClose={() => setPreview(null)}
        className="max-w-[min(94vw,1200px)] rounded-xl border border-[#303641] bg-[#0d0f12] p-2 shadow-[0_20px_70px_#000a]"
        overlayClassName="items-center pt-0"
        label={preview ? `Preview of ${preview.filename}` : "Preview"}
      >
        {preview && (
          <img
            src={preview.url}
            alt={preview.filename}
            className="block max-h-[85dvh] max-w-full rounded-lg object-contain"
          />
        )}
      </Modal>

      <Modal
        open={deleting !== null}
        onClose={() => setDeleting(null)}
        className="w-[min(100%,460px)] rounded-[10px] border border-[#303641] bg-[#12151a] p-[24px] shadow-[0_20px_70px_#000a] [&_h2]:mb-[12px] [&_p]:mb-[16px]"
        label="Delete attachment"
      >
        <h2>Delete “{keptDeleting.current?.filename}”?</h2>
        <p className="muted text-[#a2aec2]">The file is removed from the issue for everyone.</p>
        <div className="mt-[22px] flex justify-end gap-[8px]">
          <button
            autoFocus
            type="button"
            className="rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]"
            onClick={() => setDeleting(null)}
          >
            Cancel
          </button>
          <button
            type="button"
            className="rounded-full border border-[#a33a48] bg-[#7d1f2c] px-[15px] py-[10px] font-[550] text-white hover:bg-[#962636]"
            onClick={() => {
              if (deleting) deleteAttachment.mutate(deleting);
              setDeleting(null);
            }}
          >
            Delete
          </button>
        </div>
      </Modal>
    </section>
  );
}

type AttachmentRowProps = {
  attachment: Attachment;
  uploader: string;
  canDelete: boolean;
  onPreview: () => void;
  onDelete: () => void;
};

function AttachmentRow({
  attachment,
  uploader,
  canDelete,
  onPreview,
  onDelete,
}: AttachmentRowProps) {
  const kind = attachmentKind(attachment.content_type);

  return (
    <article className="rounded-lg border border-[#222730] bg-[#12151a] p-2">
      <div className="flex items-center gap-3">
        {kind === "image" ? (
          <button
            type="button"
            aria-label={`Preview ${attachment.filename}`}
            className="size-12 shrink-0 overflow-hidden rounded-md border border-[#222730] bg-[#0d0f12] p-0"
            onClick={onPreview}
          >
            <img src={attachment.url} alt="" loading="lazy" className="size-full object-cover" />
          </button>
        ) : (
          <a
            href={attachment.url}
            download={attachment.filename}
            aria-label={`Download ${attachment.filename}`}
            className="grid size-12 shrink-0 place-items-center rounded-md border border-[#222730] bg-[#0d0f12] text-[#9ba9bc] hover:text-[#d7dfeb]"
          >
            {attachment.content_type === "application/pdf" ? (
              <FileText aria-hidden="true" className="size-5" />
            ) : (
              <FileIcon aria-hidden="true" className="size-5" />
            )}
          </a>
        )}
        <div className="min-w-0 flex-1">
          <p className="m-0 truncate text-[13px] text-[#d7dfeb]" title={attachment.filename}>
            {attachment.filename}
          </p>
          <p className="m-0 mt-0.5 truncate text-[12px] text-[#9ba9bc]">
            {formatBytes(attachment.byte_size)} · {uploader} ·{" "}
            {formatRelative(attachment.created_at)}
          </p>
        </div>
        <a
          href={attachment.url}
          download={attachment.filename}
          aria-label={`Download ${attachment.filename}`}
          title="Download"
          className={iconButton}
        >
          <Download aria-hidden="true" className="size-4" />
        </a>
        {canDelete && (
          <button
            type="button"
            aria-label={`Delete ${attachment.filename}`}
            title="Delete"
            className={`${iconButton} hover:text-[#f2a9ae]!`}
            onClick={onDelete}
          >
            <Trash2 aria-hidden="true" className="size-4" />
          </button>
        )}
      </div>
      {kind === "video" && (
        <video
          controls
          preload="metadata"
          src={attachment.url}
          className="mt-2 max-h-[340px] w-full max-w-160 rounded-md bg-black"
        />
      )}
    </article>
  );
}
