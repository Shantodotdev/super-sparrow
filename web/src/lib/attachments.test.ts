import { describe, expect, test } from "bun:test";
import {
  attachmentKind,
  attachmentUploadPath,
  formatBytes,
  maxFileBytes,
  maxVideoBytes,
  queueFiles,
  screenshotFilename,
  uploadProblem,
} from "./attachments";

const file = (name: string, size: number, type = "", lastModified = 1) => ({
  name,
  size,
  type,
  lastModified,
});

describe("formatBytes", () => {
  test("uses the largest fitting unit", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(1_000_000)).toBe("1 MB");
    expect(formatBytes(150_000_000)).toBe("150 MB");
    expect(formatBytes(5_000_000_000)).toBe("5 GB");
  });

  test("treats invalid sizes as empty", () => {
    expect(formatBytes(-1)).toBe("0 B");
    expect(formatBytes(Number.NaN)).toBe("0 B");
  });
});

describe("attachmentKind", () => {
  test("sorts content types", () => {
    expect(attachmentKind("image/png")).toBe("image");
    expect(attachmentKind("Video/MP4")).toBe("video");
    expect(attachmentKind("application/pdf")).toBe("file");
    expect(attachmentKind("")).toBe("file");
  });
});

describe("uploadProblem", () => {
  test("accepts files within their limit", () => {
    expect(uploadProblem(file("a.pdf", maxFileBytes, "application/pdf"))).toBeNull();
    expect(uploadProblem(file("a.mp4", maxVideoBytes, "video/mp4"))).toBeNull();
  });

  test("rejects a large file that is not a video", () => {
    expect(uploadProblem(file("a.zip", maxFileBytes + 1, "application/zip"))).toContain("100 MB");
    expect(uploadProblem(file("a", maxFileBytes + 1))).not.toBeNull();
  });

  test("accepts a large video up to the limit and rejects beyond it", () => {
    expect(uploadProblem(file("a.mov", 2_000_000_000, "video/quicktime"))).toBeNull();
    expect(uploadProblem(file("a.mov", maxVideoBytes + 1, "video/quicktime"))).toContain("5 GB");
  });

  test("rejects an empty file", () => {
    expect(uploadProblem(file("a.txt", 0, "text/plain"))).toContain("empty");
  });
});

describe("queueFiles", () => {
  test("accepts valid files and reports invalid ones", () => {
    const result = queueFiles([], [file("a.txt", 5), file("big.zip", maxFileBytes + 1)]);
    expect(result.accepted.map((item) => item.name)).toEqual(["a.txt"]);
    expect(result.rejected).toHaveLength(1);
  });

  test("turns away files that are already queued or repeated in the batch", () => {
    const queued = [file("a.txt", 5)];
    const result = queueFiles(queued, [file("a.txt", 5), file("b.txt", 5), file("b.txt", 5)]);
    expect(result.accepted.map((item) => item.name)).toEqual(["b.txt"]);
    expect(result.rejected).toEqual(["a.txt is already added.", "b.txt is already added."]);
  });

  test("keeps files with the same name but another size or modification time", () => {
    const result = queueFiles(
      [file("a.txt", 5, "", 1)],
      [file("a.txt", 6, "", 1), file("a.txt", 5, "", 2)],
    );
    expect(result.accepted).toHaveLength(2);
  });
});

describe("attachmentUploadPath", () => {
  test("encodes the issue key and filename", () => {
    expect(attachmentUploadPath("WP-1", "my file & more.png")).toBe(
      "/api/issues/WP-1/attachments?filename=my%20file%20%26%20more.png",
    );
  });
});

describe("screenshotFilename", () => {
  test("names the file by local time", () => {
    expect(screenshotFilename(new Date(2026, 10, 9, 4, 5, 6))).toBe(
      "screenshot-20261109-040506.png",
    );
  });
});
