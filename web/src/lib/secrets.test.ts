import { describe, expect, test } from "bun:test";
import {
  buildRows,
  canDownloadConfig,
  canEditConfig,
  canSeeHistory,
  canViewConfig,
  downloadFileName,
  envFileName,
  isValidSecretName,
  pendingCount,
  revealKey,
  secretNameError,
  secretValueError,
  parseDotenv,
  parseNameList,
  patchBody,
  removePending,
  setPending,
  summarizePending,
  tokenState,
  undoPending,
  noPendingChanges,
} from "./secrets";

describe("isValidSecretName", () => {
  test("accepts upper snake case names", () => {
    expect(isValidSecretName("OPENROUTER_API_KEY")).toBe(true);
    expect(isValidSecretName("_PRIVATE1")).toBe(true);
  });

  test("rejects lowercase, leading digits, spaces, and names over 128 characters", () => {
    expect(isValidSecretName("api_key")).toBe(false);
    expect(isValidSecretName("1KEY")).toBe(false);
    expect(isValidSecretName("A KEY")).toBe(false);
    expect(isValidSecretName("")).toBe(false);
    expect(isValidSecretName("A".repeat(128))).toBe(true);
    expect(isValidSecretName("A".repeat(129))).toBe(false);
  });
});

describe("parseDotenv", () => {
  test("reads names and values and ignores comments and blank lines", () => {
    const result = parseDotenv("# header\n\nA=1\nexport B=two words\nC=\n");

    expect(result.errors).toEqual([]);
    expect(result.entries.map(({ name, value }) => [name, value])).toEqual([
      ["A", "1"],
      ["B", "two words"],
      ["C", ""],
    ]);
  });

  test("handles quotes, escapes in double quotes, and trailing comments", () => {
    const result = parseDotenv(`A="x\\ny" # note\nB='a #b'\nC=plain # note\nD=a=b`);

    expect(result.errors).toEqual([]);
    expect(result.entries.map((entry) => entry.value)).toEqual(["x\ny", "a #b", "plain", "a=b"]);
  });

  test("reports bad lines with their line numbers and keeps the good ones", () => {
    const result = parseDotenv('A=1\nnot a pair\nlower=2\nB="open\nC=3');

    expect(result.entries.map((entry) => entry.name)).toEqual(["A", "C"]);
    expect(result.errors.map((error) => error.line)).toEqual([2, 3, 4]);
  });

  test("unescapes only n, r, t, backslash, and quote inside double quotes", () => {
    const result = parseDotenv('A="a\\nb\\tc\\rd\\\\e\\"f"\nB="C:\\path\\x"\nC=\'a\\nb\'');

    expect(result.errors).toEqual([]);
    expect(result.entries.map((entry) => entry.value)).toEqual([
      'a\nb\tc\rd\\e"f',
      "C:\\path\\x",
      "a\\nb",
    ]);
  });

  test("reads multi-line double-quoted values and resumes after them", () => {
    const result = parseDotenv('KEY="-----BEGIN-----\nline two\n-----END-----"\nNEXT=1');

    expect(result.errors).toEqual([]);
    expect(result.entries.map(({ name, value, line }) => [name, value, line])).toEqual([
      ["KEY", "-----BEGIN-----\nline two\n-----END-----", 1],
      ["NEXT", "1", 4],
    ]);
  });

  test("accepts CRLF line endings and a leading byte order mark", () => {
    const result = parseDotenv('\uFEFFA=1\r\nB="x\r\ny"\r\nC=3\r\n');

    expect(result.errors).toEqual([]);
    expect(result.entries.map((entry) => [entry.name, entry.value])).toEqual([
      ["A", "1"],
      ["B", "x\ny"],
      ["C", "3"],
    ]);
  });

  test("reports text after a closing quote and keeps unclosed single quotes single-line", () => {
    const result = parseDotenv('A="x" y\nB=\'open\nC=3');

    expect(result.errors.map((error) => error.line)).toEqual([1, 2]);
    expect(result.entries.map((entry) => entry.name)).toEqual(["C"]);
  });

  test("keeps the last value of a repeated name", () => {
    const result = parseDotenv("A=1\nA=2");

    expect(result.entries).toHaveLength(1);
    expect(result.entries[0]?.value).toBe("2");
  });
});

describe("pending changes", () => {
  const saved = ["A", "B", "C"];

  test("sorts names into added, changed, and removed", () => {
    let pending = setPending(noPendingChanges, "NEW", "1");
    pending = setPending(pending, "B", "2");
    pending = removePending(pending, "C", saved);

    expect(summarizePending(pending, saved)).toEqual({
      added: ["NEW"],
      changed: ["B"],
      removed: ["C"],
    });
  });

  test("removing an unsaved key only drops it, and setting a removed key cancels the delete", () => {
    let pending = setPending(noPendingChanges, "NEW", "1");
    pending = removePending(pending, "NEW", saved);
    expect(pending).toEqual({ set: {}, remove: [] });

    pending = removePending(pending, "A", saved);
    pending = setPending(pending, "A", "x");
    expect(pending).toEqual({ set: { A: "x" }, remove: [] });
  });

  test("undo forgets a pending change", () => {
    const pending = undoPending(removePending(noPendingChanges, "A", saved), "A");

    expect(pending).toEqual(noPendingChanges);
  });

  test("builds the PATCH body", () => {
    const pending = setPending(removePending(noPendingChanges, "A", saved), "B", "1");

    expect(patchBody(pending)).toEqual({ set: { B: "1" }, delete: ["A"] });
  });

  test("counts set and removed names, and ignores a remove of an unsaved key", () => {
    expect(pendingCount(noPendingChanges)).toBe(0);

    const pending = setPending(removePending(noPendingChanges, "A", saved), "NEW", "1");
    expect(pendingCount(pending)).toBe(2);
    expect(pendingCount(removePending(noPendingChanges, "NOPE", saved))).toBe(0);
  });

  test("summarizes edge cases: a pending remove of an unsaved name and an empty set", () => {
    expect(summarizePending(noPendingChanges, saved)).toEqual({
      added: [],
      changed: [],
      removed: [],
    });
    expect(summarizePending({ set: {}, remove: ["GHOST"] }, saved).removed).toEqual([]);
    expect(summarizePending({ set: { Z: "1", A: "1" }, remove: ["C", "B"] }, saved)).toEqual({
      added: ["Z"],
      changed: ["A"],
      removed: ["B", "C"],
    });
  });
});

describe("parseNameList", () => {
  test("splits on lines and commas and drops duplicates", () => {
    expect(parseNameList("A, B\nC\n\nA")).toEqual(["A", "B", "C"]);
  });
});

describe("tokenState", () => {
  const now = Date.parse("2026-11-03T00:00:00Z");
  const day = 24 * 60 * 60 * 1000;

  test("flags revoked, expired, expiring within 14 days, and active tokens", () => {
    const at = (offset: number) => new Date(now + offset * day).toISOString();

    expect(tokenState({ expires_at: at(100), revoked_at: at(-1) }, now)).toBe("revoked");
    expect(tokenState({ expires_at: at(-1), revoked_at: null }, now)).toBe("expired");
    expect(tokenState({ expires_at: at(14), revoked_at: null }, now)).toBe("expiring");
    expect(tokenState({ expires_at: at(15), revoked_at: null }, now)).toBe("active");
  });
});

describe("tokenState boundaries", () => {
  const now = Date.parse("2026-11-03T00:00:00Z");

  test("expiring ends exactly at the expiry instant", () => {
    const at = (offset: number) => new Date(now + offset).toISOString();

    expect(tokenState({ expires_at: at(0), revoked_at: null }, now)).toBe("expired");
    expect(tokenState({ expires_at: at(1), revoked_at: null }, now)).toBe("expiring");
    expect(
      tokenState({ expires_at: at(14 * 24 * 60 * 60 * 1000 + 1), revoked_at: null }, now),
    ).toBe("active");
  });
});

describe("secretNameError", () => {
  test("explains each failure and returns nothing for a valid name", () => {
    expect(secretNameError("")).toBe("Enter a name.");
    expect(secretNameError("A".repeat(129))).toBe("Use at most 128 characters.");
    expect(secretNameError("bad")).toContain("capital letters");
    expect(secretNameError("GOOD_1")).toBe("");
  });
});

describe("secretValueError", () => {
  test("allows up to 64 KiB measured in bytes, not characters", () => {
    expect(secretValueError("")).toBe("");
    expect(secretValueError("a".repeat(64 * 1024))).toBe("");
    expect(secretValueError("a".repeat(64 * 1024 + 1))).not.toBe("");
    expect(secretValueError("é".repeat(32 * 1024))).toBe("");
    expect(secretValueError("é".repeat(32 * 1024 + 1))).not.toBe("");
    expect(secretValueError("😀".repeat(16 * 1024 + 1))).not.toBe("");
  });
});

describe("config permissions", () => {
  test("everyone edits dev, only managers edit the rest", () => {
    expect(canEditConfig("dev", false)).toBe(true);
    expect(canEditConfig("dev-defaults", false)).toBe(false);
    expect(canEditConfig("dev-defaults", true)).toBe(true);
    expect(canEditConfig("production", false)).toBe(false);
  });

  test("everyone views dev and dev defaults, only managers view staging and production", () => {
    expect(canViewConfig("dev", false)).toBe(true);
    expect(canViewConfig("dev-defaults", false)).toBe(true);
    expect(canViewConfig("staging", false)).toBe(false);
    expect(canViewConfig("staging", true)).toBe(true);
    expect(canViewConfig("production", true)).toBe(true);
  });

  test("downloads: dev for everyone, staging and production for managers, never dev defaults", () => {
    expect(canDownloadConfig("dev", false)).toBe(true);
    expect(canDownloadConfig("dev-defaults", true)).toBe(false);
    expect(canDownloadConfig("staging", false)).toBe(false);
    expect(canDownloadConfig("production", true)).toBe(true);
  });

  test("history: dev only for overrides, staging and production for managers", () => {
    expect(canSeeHistory("dev", true, { source: "override" })).toBe(true);
    expect(canSeeHistory("dev", true, { source: "default" })).toBe(false);
    expect(canSeeHistory("dev-defaults", false, {})).toBe(true);
    expect(canSeeHistory("staging", false, {})).toBe(false);
    expect(canSeeHistory("production", true, {})).toBe(true);
  });
});

describe("file names and reveal keys", () => {
  test("names downloads by format", () => {
    expect(envFileName("wp", "dev")).toBe("wp-dev.env");
    expect(downloadFileName("wp", "staging", "env")).toBe("wp-staging.env");
    expect(downloadFileName("wp", "staging", "shell")).toBe("wp-staging.sh");
    expect(downloadFileName("wp", "production", "json")).toBe("wp-production.json");
  });

  test("a new version gets a different reveal key", () => {
    expect(revealKey({ name: "A", version: 1 })).not.toBe(revealKey({ name: "A", version: 2 }));
  });
});

describe("buildRows", () => {
  const entry = (name: string) => ({
    name,
    version: 1,
    updated_at: "2026-11-03T00:00:00Z",
    updated_by: "a",
  });
  const listing = { secrets: [entry("B"), entry("A"), entry("C")], required: [], missing: [] };

  test("marks saved, changed, and removed keys and adds pending-only keys, sorted by name", () => {
    let pending = setPending(noPendingChanges, "B", "x");
    pending = removePending(pending, "C", ["A", "B", "C"]);
    pending = setPending(pending, "AA", "y");

    expect(buildRows(listing, pending).map((row) => [row.name, row.state])).toEqual([
      ["A", "saved"],
      ["AA", "added"],
      ["B", "changed"],
      ["C", "removed"],
    ]);
    expect(buildRows(listing, noPendingChanges).every((row) => row.entry)).toBe(true);
  });
});
