import { useRef, useState } from "react";
import { queryOptions, useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { apiGet, apiPutFile, apiWrite, type UploadTarget } from "../lib/api";
import { attachmentUploadRequest, queueFiles } from "../lib/attachments";
import type { Attachment } from "../lib/types";
import { useToast } from "../components/toast";

/**
 * What a set of files belongs to: the issue itself, one of its subissues, or one of its findings.
 * Setting both `subissueId` and `finding` is not meaningful; `subissueId` wins.
 */
export type AttachmentScope = {
  issueKey: string;
  subissueId?: string;
  finding?: { id: string; round: number; number: number };
};

/** The API path of a scope's attachments. */
function scopePath({ issueKey, subissueId, finding }: AttachmentScope): string {
  const issue = `/api/issues/${encodeURIComponent(issueKey)}`;
  if (subissueId) return `${issue}/subissues/${encodeURIComponent(subissueId)}/attachments`;
  if (finding) return `${issue}/reviews/${finding.round}/findings/${finding.number}/attachments`;
  return `${issue}/attachments`;
}

/** A scope's attachments, oldest first. The issue key leads the query key so live events can refresh every scope of an issue. */
export const attachmentsQuery = (scope: AttachmentScope) =>
  queryOptions({
    queryKey: [
      "attachments",
      scope.issueKey.toUpperCase(),
      scope.subissueId ?? scope.finding?.id ?? "",
    ],
    queryFn: async () =>
      (await apiGet<{ attachments: Attachment[] }>(scopePath(scope))).attachments,
  });

/** An issue's own attachments, oldest first. */
export const issueAttachmentsQuery = (issueKey: string) => attachmentsQuery({ issueKey });

/** Adds a stored attachment to its scope's cached list and refreshes the list. */
function cacheAttachment(client: QueryClient, scope: AttachmentScope, saved: Attachment) {
  const key = attachmentsQuery(scope).queryKey;
  client.setQueryData<Attachment[]>(key, (current) =>
    current && !current.some((item) => item.id === saved.id) ? [...current, saved] : current,
  );
  void client.invalidateQueries({ queryKey: key });
}

/** Deletes an attachment; only people can. */
export function useDeleteAttachment(scope: AttachmentScope) {
  const client = useQueryClient();
  const { notify } = useToast();
  return useMutation({
    mutationFn: (attachment: Attachment) =>
      apiWrite<void>(`/api/attachments/${encodeURIComponent(attachment.id)}`, "DELETE"),
    onSuccess: (_, attachment) => {
      const key = attachmentsQuery(scope).queryKey;
      client.setQueryData<Attachment[]>(key, (current) =>
        current?.filter((item) => item.id !== attachment.id),
      );
      void client.invalidateQueries({ queryKey: key });
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
 * Tracks files being attached. Files are queued first and uploaded when `start` is given an issue
 * key or a scope, so a form can queue files before its issue exists. Uploads run one after another
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

  const upload = async (scope: AttachmentScope, job: UploadJob): Promise<boolean> => {
    const controller = new AbortController();
    controllers.current.set(job.id, controller);
    patch(job.id, { status: "uploading", progress: 0, error: "" });
    let pendingId = "";

    try {
      const started = await apiWrite<{ attachment: Attachment; upload: UploadTarget }>(
        scopePath(scope),
        "POST",
        attachmentUploadRequest(job.file),
      );
      pendingId = started.attachment.id;
      await apiPutFile(started.upload, job.file, {
        signal: controller.signal,
        onProgress: (progress) => patch(job.id, { progress }),
      });
      const { attachment } = await apiWrite<{ attachment: Attachment }>(
        `/api/attachments/${encodeURIComponent(pendingId)}/complete`,
        "POST",
      );
      cacheAttachment(client, scope, attachment);
      patch(job.id, { status: "done", progress: 1 });
      return true;
    } catch (failure) {
      // Best effort: drop the pending row so it does not linger.
      if (pendingId)
        void apiWrite<void>(`/api/attachments/${encodeURIComponent(pendingId)}`, "DELETE").catch(
          () => {},
        );
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

  /** Uploads the given jobs, or every queued and failed job, to an issue or a scope of it; resolves to the failures. */
  const start = async (
    target: string | AttachmentScope,
    only?: UploadJob[],
  ): Promise<UploadJob[]> => {
    const scope = typeof target === "string" ? { issueKey: target } : target;
    const ids = new Set((only ?? current.current).map((job) => job.id));
    const pending = current.current.filter(
      (job) => ids.has(job.id) && (job.status === "queued" || job.status === "failed"),
    );
    const failed: UploadJob[] = [];
    for (const job of pending) {
      // A job removed while an earlier one uploaded is skipped.
      if (!current.current.some((item) => item.id === job.id)) continue;
      if (!(await upload(scope, job))) {
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
