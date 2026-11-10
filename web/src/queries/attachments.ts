import { useRef, useState } from "react";
import { queryOptions, useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { apiGet, apiUploadFile, apiWrite } from "../lib/api";
import { attachmentUploadPath, queueFiles } from "../lib/attachments";
import type { Attachment } from "../lib/types";
import { useToast } from "../components/toast";

/** An issue's attachments, oldest first. */
export const issueAttachmentsQuery = (issueKey: string) =>
  queryOptions({
    queryKey: ["attachments", issueKey.toUpperCase()],
    queryFn: async () =>
      (
        await apiGet<{ attachments: Attachment[] }>(
          `/api/issues/${encodeURIComponent(issueKey)}/attachments`,
        )
      ).attachments,
  });

/** Adds a stored attachment to its issue's cached list and refreshes the list. */
function cacheAttachment(client: QueryClient, saved: Attachment) {
  const key = issueAttachmentsQuery(saved.issue_key).queryKey;
  client.setQueryData<Attachment[]>(key, (current) =>
    current && !current.some((item) => item.id === saved.id) ? [...current, saved] : current,
  );
  void client.invalidateQueries({ queryKey: key });
}

/** Deletes an attachment; only people can. */
export function useDeleteAttachment(issueKey: string) {
  const client = useQueryClient();
  const { notify } = useToast();
  return useMutation({
    mutationFn: (attachment: Attachment) =>
      apiWrite<void>(`/api/attachments/${encodeURIComponent(attachment.id)}`, "DELETE"),
    onSuccess: (_, attachment) => {
      client.setQueryData<Attachment[]>(issueAttachmentsQuery(issueKey).queryKey, (current) =>
        current?.filter((item) => item.id !== attachment.id),
      );
      void client.invalidateQueries({ queryKey: issueAttachmentsQuery(issueKey).queryKey });
    },
    onError: (error) => notify(error.message),
  });
}

export type UploadStatus = "queued" | "uploading" | "done" | "failed";

/** One file waiting for, going through, or finished with an upload. */
export type UploadJob = {
  id: number;
  file: File;
  status: UploadStatus;
  /** The fraction of the file sent, from 0 to 1. */
  progress: number;
  error: string;
};

/**
 * Tracks files being attached. Files are queued first and uploaded to an issue when `start` is
 * given its key, so a form can queue files before its issue exists. Uploads run one after another
 * within a `start` call, and a finished upload lands in the issue's attachment list at once.
 */
export function useAttachmentUploads() {
  const client = useQueryClient();
  const [jobs, setJobs] = useState<UploadJob[]>([]);
  const current = useRef<UploadJob[]>([]);
  const controllers = useRef(new Map<number, AbortController>());
  const nextId = useRef(1);

  const update = (next: UploadJob[]) => {
    current.current = next;
    setJobs(next);
  };
  const patch = (id: number, change: Partial<UploadJob>) =>
    update(current.current.map((job) => (job.id === id ? { ...job, ...change } : job)));

  /** Queues the files that pass the checks and returns the new jobs and a message per rejection. */
  const add = (files: File[]): { added: UploadJob[]; rejected: string[] } => {
    const { accepted, rejected } = queueFiles(
      current.current.map((job) => job.file),
      files,
    );
    const added = accepted.map<UploadJob>((file) => ({
      id: nextId.current++,
      file,
      status: "queued",
      progress: 0,
      error: "",
    }));
    update([...current.current, ...added]);
    return { added, rejected };
  };

  const upload = async (issueKey: string, job: UploadJob): Promise<boolean> => {
    const controller = new AbortController();
    controllers.current.set(job.id, controller);
    patch(job.id, { status: "uploading", progress: 0, error: "" });

    try {
      const { attachment } = await apiUploadFile<{ attachment: Attachment }>(
        attachmentUploadPath(issueKey, job.file.name),
        job.file,
        { signal: controller.signal, onProgress: (progress) => patch(job.id, { progress }) },
      );
      cacheAttachment(client, attachment);
      patch(job.id, { status: "done", progress: 1 });
      return true;
    } catch (failure) {
      const canceled = failure instanceof DOMException && failure.name === "AbortError";
      if (!canceled) {
        patch(job.id, {
          status: "failed",
          error: failure instanceof Error ? failure.message : "Could not upload the file",
        });
      }
      return false;
    } finally {
      controllers.current.delete(job.id);
    }
  };

  /** Uploads the given jobs, or every queued and failed job, to an issue; resolves to the failures. */
  const start = async (issueKey: string, only?: UploadJob[]): Promise<UploadJob[]> => {
    const ids = new Set((only ?? current.current).map((job) => job.id));
    const pending = current.current.filter(
      (job) => ids.has(job.id) && (job.status === "queued" || job.status === "failed"),
    );
    const failed: UploadJob[] = [];
    for (const job of pending) {
      // A job removed while an earlier one uploaded is skipped.
      if (!current.current.some((item) => item.id === job.id)) continue;
      if (!(await upload(issueKey, job))) {
        const latest = current.current.find((item) => item.id === job.id);
        if (latest) failed.push(latest);
      }
    }
    return failed;
  };

  /** Stops the job if it is uploading and takes it out of the list. */
  const remove = (id: number) => {
    controllers.current.get(id)?.abort();
    update(current.current.filter((job) => job.id !== id));
  };

  const clear = () => {
    for (const controller of controllers.current.values()) controller.abort();
    update([]);
  };

  return { jobs, add, start, remove, clear };
}
