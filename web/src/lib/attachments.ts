/** The largest file the server takes. */
export const maxVideoBytes = 5_000_000_000;

/** The largest file that is not a video. */
export const maxFileBytes = 100_000_000;

/** How the panel shows an attachment: a thumbnail, a player, or a file icon. */
export type AttachmentKind = "image" | "video" | "file";

/** What the upload checks need to know about a file. */
export type FileInfo = { name: string; size: number; type: string };

/** Formats a byte count for people, such as "1.4 MB". */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 B";
  if (bytes < 1000) return `${Math.round(bytes)} B`;

  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1000;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }

  const digits = value >= 100 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(digits)} ${units[unit]}`;
}

/** Tells how an attachment of this content type is shown. */
export function attachmentKind(contentType: string): AttachmentKind {
  const type = contentType.toLowerCase();
  if (type.startsWith("image/")) return "image";
  if (type.startsWith("video/")) return "video";
  return "file";
}

/** Returns why a file cannot be uploaded, or null when it can. */
export function uploadProblem(file: FileInfo): string | null {
  if (file.size === 0) return `${file.name} is empty.`;
  if (file.type.startsWith("video/")) {
    return file.size > maxVideoBytes
      ? `${file.name} is ${formatBytes(file.size)}; videos can be up to ${formatBytes(maxVideoBytes)}.`
      : null;
  }
  return file.size > maxFileBytes
    ? `${file.name} is ${formatBytes(file.size)}; files other than videos can be up to ${formatBytes(maxFileBytes)}.`
    : null;
}

/** The files taken into a queue and a message for each file turned away. */
export type QueueResult<T extends FileInfo & { lastModified?: number }> = {
  accepted: T[];
  rejected: string[];
};

/**
 * Splits incoming files into those to queue and those to turn away. A file that is invalid, or
 * is already queued or repeated in the same batch (same name, size, and modification time), is
 * turned away.
 */
export function queueFiles<T extends FileInfo & { lastModified?: number }>(
  queued: readonly T[],
  incoming: readonly T[],
): QueueResult<T> {
  const identity = (file: T) => `${file.name}\u0000${file.size}\u0000${file.lastModified ?? 0}`;
  const seen = new Set(queued.map(identity));
  const accepted: T[] = [];
  const rejected: string[] = [];

  for (const file of incoming) {
    const problem = uploadProblem(file);
    if (problem) {
      rejected.push(problem);
    } else if (seen.has(identity(file))) {
      rejected.push(`${file.name} is already added.`);
    } else {
      seen.add(identity(file));
      accepted.push(file);
    }
  }

  return { accepted, rejected };
}

/** The JSON body that starts an upload: what the server needs to sign a direct upload. */
export function attachmentUploadRequest(file: FileInfo): {
  filename: string;
  content_type: string;
  byte_size: number;
} {
  return {
    filename: file.name,
    content_type: file.type || "application/octet-stream",
    byte_size: file.size,
  };
}

/** Names a screenshot by when it was taken, such as "screenshot-20261110-142530.png". */
export function screenshotFilename(taken: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  const day = `${taken.getFullYear()}${pad(taken.getMonth() + 1)}${pad(taken.getDate())}`;
  const time = `${pad(taken.getHours())}${pad(taken.getMinutes())}${pad(taken.getSeconds())}`;
  return `screenshot-${day}-${time}.png`;
}

/** The files in a drop, if there are any. */
export function transferredFiles(data: DataTransfer | null | undefined): File[] {
  return Array.from(data?.files ?? []);
}

/** Whether a drag carries files, as opposed to text or a link. */
export function dragHasFiles(data: DataTransfer | null | undefined): boolean {
  return Array.from(data?.types ?? []).includes("Files");
}
