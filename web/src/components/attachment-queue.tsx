import { useRef } from "react";
import { Check, File as FileIcon, Paperclip, RotateCw, X } from "lucide-react";
import { formatBytes } from "../lib/attachments";
import type { UploadJob, useAttachmentUploads } from "../queries/attachments";
import { ScreenshotButton } from "./screenshot-button";

type Uploads = ReturnType<typeof useAttachmentUploads>;

const chipButton =
  "inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] hover:border-[#484b54] hover:bg-[#383b42] disabled:opacity-60";

type AttachmentButtonsProps = {
  onFiles: (files: File[]) => void;
  onError: (message: string) => void;
  disabled?: boolean;
};

/** The "Add files" and "Screenshot" buttons that hand files to `onFiles`. */
export function AttachmentButtons({ onFiles, onError, disabled = false }: AttachmentButtonsProps) {
  const input = useRef<HTMLInputElement>(null);

  return (
    <>
      <input
        ref={input}
        type="file"
        multiple
        hidden
        aria-label="Choose files to attach"
        onChange={(event) => {
          onFiles(Array.from(event.target.files ?? []));
          event.target.value = "";
        }}
      />
      <button
        type="button"
        className={chipButton}
        disabled={disabled}
        onClick={() => input.current?.click()}
      >
        <Paperclip aria-hidden="true" className="size-4" />
        Add files
      </button>
      <ScreenshotButton onCaptured={(file) => onFiles([file])} onError={onError} />
    </>
  );
}

type UploadListProps = {
  jobs: UploadJob[];
  onRemove: (job: UploadJob) => void;
  onRetry?: (job: UploadJob) => void;
  /** Whether finished uploads stay in the list with a check mark. */
  showDone?: boolean;
};

/** Lists files being attached with their progress, and a cancel, remove, or retry control each. */
export function UploadList({ jobs, onRemove, onRetry, showDone = false }: UploadListProps) {
  const shown = showDone ? jobs : jobs.filter((job) => job.status !== "done");
  if (shown.length === 0) return null;

  return (
    <ul className="m-0 flex list-none flex-col gap-1.5 p-0">
      {shown.map((job) => (
        <UploadRow key={job.id} job={job} onRemove={onRemove} onRetry={onRetry} />
      ))}
    </ul>
  );
}

function UploadRow({
  job,
  onRemove,
  onRetry,
}: {
  job: UploadJob;
  onRemove: (job: UploadJob) => void;
  onRetry?: (job: UploadJob) => void;
}) {
  const percent = Math.round(job.progress * 100);
  const uploading = job.status === "uploading";

  return (
    <li className="rounded-lg border border-[#222730] bg-[#0d0f12] px-2.5 py-1.5 text-[12px] text-[#aebed4]">
      <div className="flex items-center gap-2">
        {job.status === "done" ? (
          <Check aria-hidden="true" className="size-4 shrink-0 text-[#7fcd9c]" />
        ) : (
          <FileIcon aria-hidden="true" className="size-4 shrink-0 text-[#9ba9bc]" />
        )}
        <span className="min-w-0 flex-1 truncate text-[#d7dfeb]" title={job.file.name}>
          {job.file.name}
        </span>
        <span className="shrink-0 text-[#9ba9bc]">
          {uploading ? `${percent}%` : formatBytes(job.file.size)}
        </span>
        {job.status === "failed" && onRetry && (
          <button
            type="button"
            aria-label={`Retry ${job.file.name}`}
            title="Retry"
            className="grid place-items-center rounded border-0 bg-transparent p-1 text-[#a7c0ff] hover:bg-[#222730]"
            onClick={() => onRetry(job)}
          >
            <RotateCw aria-hidden="true" className="size-3.5" />
          </button>
        )}
        {job.status !== "done" && (
          <button
            type="button"
            aria-label={`${uploading ? "Cancel upload of" : "Remove"} ${job.file.name}`}
            title={uploading ? "Cancel" : "Remove"}
            className="grid place-items-center rounded border-0 bg-transparent p-1 text-[#9ba9bc] hover:bg-[#222730] hover:text-[#d7dfeb]"
            onClick={() => onRemove(job)}
          >
            <X aria-hidden="true" className="size-3.5" />
          </button>
        )}
      </div>
      {uploading && (
        <div
          role="progressbar"
          aria-label={`Uploading ${job.file.name}`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent}
          className="mt-1.5 h-1 overflow-hidden rounded-full bg-[#222730]"
        >
          <div className="h-full bg-[#426fff]" style={{ width: `${percent}%` }} />
        </div>
      )}
      {job.status === "failed" && <p className="m-0 mt-1 text-[#f2a9ae]">{job.error}</p>}
    </li>
  );
}

type UploadFailuresProps = {
  issueKey: string;
  uploads: Uploads;
  onDone: () => void;
};

/**
 * Shown after an issue was created but some of its files did not upload: names the issue, lists
 * the files with a retry each, and lets the person retry them all or finish.
 */
export function UploadFailures({ issueKey, uploads, onDone }: UploadFailuresProps) {
  const failed = uploads.jobs.filter((job) => job.status === "failed");
  const uploading = uploads.jobs.some((job) => job.status === "uploading");

  const retry = async (jobs: UploadJob[]) => {
    const remaining = await uploads.start(issueKey, jobs);
    for (const job of jobs) {
      if (!remaining.some((item) => item.id === job.id)) uploads.remove(job.id);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      {failed.length > 0 ? (
        <p role="alert" className="m-0 text-[13px] text-[#f2a9ae]">
          {issueKey} was created, but {failed.length === 1 ? "1 file" : `${failed.length} files`}{" "}
          did not upload.
        </p>
      ) : (
        <p className="m-0 text-[13px] text-[#aebed4]">{issueKey} was created with its files.</p>
      )}
      <UploadList
        jobs={uploads.jobs}
        onRemove={(job) => uploads.remove(job.id)}
        onRetry={(job) => void retry([job])}
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          className={chipButton}
          disabled={uploading || failed.length === 0}
          onClick={() => void retry(failed)}
        >
          Retry all
        </button>
        <button type="button" className={chipButton} onClick={onDone}>
          Done
        </button>
      </div>
    </div>
  );
}
