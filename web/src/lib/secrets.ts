import type { SecretConfigId, SecretConfigListing, SecretEntry, ServiceToken } from "./types";

export const secretConfigs: SecretConfigId[] = ["dev", "dev-defaults", "staging", "production"];

export const secretConfigLabel: Record<SecretConfigId, string> = {
  dev: "My dev",
  "dev-defaults": "Dev defaults",
  staging: "Staging",
  production: "Production",
};

const nameRule = /^[A-Z_][A-Z0-9_]{0,127}$/;
const expiryWarningDays = 14;
const dayMilliseconds = 24 * 60 * 60 * 1000;

/** Returns the message that explains why name cannot be a secret key, or "" when it can. */
export function secretNameError(name: string): string {
  if (!name) return "Enter a name.";
  if (name.length > 128) return "Use at most 128 characters.";
  if (!nameRule.test(name))
    return "Use capital letters, digits, and underscores, not starting with a digit.";

  return "";
}

/** Reports whether name is a valid secret key, as the server checks it. */
export function isValidSecretName(name: string): boolean {
  return secretNameError(name) === "";
}

export type DotenvEntry = { name: string; value: string; line: number };

export type DotenvError = { line: number; message: string };

const doubleQuoteEscapes: Record<string, string> = {
  n: "\n",
  r: "\r",
  t: "\t",
  "\\": "\\",
  '"': '"',
};

type ParsedValue = { value: string; lastLine: number } | { error: string };

/**
 * Reads the value that starts at lines[start] after the equals sign: quoted or bare, with a trailing
 * comment dropped when bare. A double-quoted value may continue over following lines, and inside
 * double quotes only \n, \r, \t, \\, and \" are escapes; any other backslash is kept as written.
 */
function parseDotenvValue(lines: string[], start: number, raw: string): ParsedValue {
  const text = raw.trimStart();
  const quote = text[0];

  if (quote !== '"' && quote !== "'") {
    const bare = text.trim();
    const comment = bare.search(/\s#/);

    return { value: (comment === -1 ? bare : bare.slice(0, comment)).trim(), lastLine: start };
  }

  let value = "";
  let lineIndex = start;
  let current = text;
  let index = 1;

  for (;;) {
    if (index >= current.length) {
      if (quote === "'" || lineIndex + 1 >= lines.length)
        return { error: "The quote is not closed." };

      value += "\n";
      lineIndex++;
      current = lines[lineIndex]!;
      index = 0;
      continue;
    }

    const char = current[index]!;
    if (char === quote) {
      const rest = current.slice(index + 1).trim();
      if (rest && !rest.startsWith("#"))
        return { error: "Unexpected text after the closing quote." };

      return { value, lastLine: lineIndex };
    }

    const escaped =
      quote === '"' && char === "\\" ? doubleQuoteEscapes[current[index + 1] ?? ""] : undefined;
    if (escaped !== undefined) {
      value += escaped;
      index += 2;
      continue;
    }

    value += char;
    index++;
  }
}

/**
 * Parses pasted .env text into entries. Blank lines and comments are ignored, an `export ` prefix is
 * allowed, a leading byte order mark and CRLF line endings are accepted, and each line that cannot
 * be used is reported with its 1-based line number. When a name repeats, the last value wins.
 */
export function parseDotenv(text: string): { entries: DotenvEntry[]; errors: DotenvError[] } {
  const entries = new Map<string, DotenvEntry>();
  const errors: DotenvError[] = [];
  const lines = text.replace(/^\uFEFF/, "").split(/\r?\n/);

  for (let index = 0; index < lines.length; index++) {
    const line = index + 1;
    const trimmed = lines[index]!.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;

    const body = lines[index]!.trimStart().replace(/^export\s+/, "");
    const equals = body.indexOf("=");
    if (equals === -1) {
      errors.push({ line, message: "Expected NAME=value." });
      continue;
    }

    const name = body.slice(0, equals).trim();
    const nameError = secretNameError(name);
    if (nameError) {
      errors.push({ line, message: `${name || "Name"}: ${nameError}` });
      continue;
    }

    const parsed = parseDotenvValue(lines, index, body.slice(equals + 1));
    if ("error" in parsed) {
      errors.push({ line, message: `${name}: ${parsed.error}` });
      continue;
    }

    entries.set(name, { name, value: parsed.value, line });
    index = parsed.lastLine;
  }

  return { entries: [...entries.values()], errors };
}

/** Unsaved edits to one config: values to set (adds and changes) and names to delete. */
export type PendingChanges = { set: Record<string, string>; remove: string[] };

export const noPendingChanges: PendingChanges = { set: {}, remove: [] };

/** Records a value for name, cancelling a pending delete of it. */
export function setPending(pending: PendingChanges, name: string, value: string): PendingChanges {
  return {
    set: { ...pending.set, [name]: value },
    remove: pending.remove.filter((item) => item !== name),
  };
}

/** Records a delete of name, or just drops its pending value when the key is not saved yet. */
export function removePending(
  pending: PendingChanges,
  name: string,
  saved: string[],
): PendingChanges {
  const set = { ...pending.set };
  delete set[name];

  return {
    set,
    remove:
      saved.includes(name) && !pending.remove.includes(name)
        ? [...pending.remove, name]
        : pending.remove,
  };
}

/** Forgets any pending change to name. */
export function undoPending(pending: PendingChanges, name: string): PendingChanges {
  const set = { ...pending.set };
  delete set[name];

  return { set, remove: pending.remove.filter((item) => item !== name) };
}

export type ChangeSummary = { added: string[]; changed: string[]; removed: string[] };

/** Sorts pending edits into added, changed, and removed names, given the names already saved. */
export function summarizePending(pending: PendingChanges, saved: string[]): ChangeSummary {
  const names = Object.keys(pending.set).sort();

  return {
    added: names.filter((name) => !saved.includes(name)),
    changed: names.filter((name) => saved.includes(name)),
    removed: pending.remove.filter((name) => saved.includes(name)).sort(),
  };
}

/** Counts the keys a pending change set touches. */
export function pendingCount(pending: PendingChanges): number {
  return Object.keys(pending.set).length + pending.remove.length;
}

/** Builds the PATCH body for a pending change set. */
export function patchBody(pending: PendingChanges): {
  set: Record<string, string>;
  delete: string[];
} {
  return { set: pending.set, delete: pending.remove };
}

/** Reads a list of key names typed one per line or separated by commas, without duplicates. */
export function parseNameList(text: string): string[] {
  return [...new Set(text.split(/[\s,]+/).filter(Boolean))];
}

export type TokenState = "revoked" | "expired" | "expiring" | "active";

/** Says where a service token stands: revoked, expired, expiring within 14 days, or active. */
export function tokenState(
  token: Pick<ServiceToken, "expires_at" | "revoked_at">,
  now: number,
): TokenState {
  if (token.revoked_at) return "revoked";

  const remaining = new Date(token.expires_at).getTime() - now;
  if (remaining <= 0) return "expired";
  if (remaining <= expiryWarningDays * dayMilliseconds) return "expiring";

  return "active";
}

/** The formats a config can be downloaded in. */
export type DownloadFormat = "env" | "shell" | "json";

export const downloadFormats: DownloadFormat[] = ["env", "shell", "json"];

export const downloadFormatLabel: Record<DownloadFormat, string> = {
  env: ".env",
  shell: "Shell script",
  json: "JSON",
};

/** Names the .env file a downloaded config is saved as. */
export function envFileName(projectSlug: string, config: SecretConfigId): string {
  return `${projectSlug}-${config}.env`;
}

/** Names the file a config downloaded in format is saved as. */
export function downloadFileName(
  projectSlug: string,
  config: SecretConfigId,
  format: DownloadFormat,
): string {
  if (format === "env") return envFileName(projectSlug, config);

  return `${projectSlug}-${config}.${format === "shell" ? "sh" : "json"}`;
}

/** Reports whether the caller may change a config's keys, given the overview's can_manage. */
export function canEditConfig(config: SecretConfigId, canManage: boolean): boolean {
  return config === "dev" || canManage;
}

/**
 * Reports whether the caller may read a config's values. Dev defaults are part of everyone's dev,
 * so everyone reads them; staging and production need can_manage.
 */
export function canViewConfig(config: SecretConfigId, canManage: boolean): boolean {
  return config === "dev" || config === "dev-defaults" || canManage;
}

/** Reports whether the caller may download a config: dev always, staging and production with can_manage. */
export function canDownloadConfig(config: SecretConfigId, canManage: boolean): boolean {
  return config === "dev" || (config !== "dev-defaults" && canManage);
}

/**
 * Reports whether the caller may open the history of a key. Staging and production need
 * can_manage, and in dev only the caller's own overrides have a history.
 */
export function canSeeHistory(
  config: SecretConfigId,
  canManage: boolean,
  entry: Pick<SecretEntry, "source">,
): boolean {
  if (config === "staging" || config === "production") return canManage;
  if (config === "dev") return entry.source === "override";

  return true;
}

/** Keys a revealed value by name and version, so a newer version never shows an older value. */
export function revealKey(entry: Pick<SecretEntry, "name" | "version">): string {
  return `${entry.name}@${entry.version}`;
}

export type SecretRow = {
  name: string;
  entry?: SecretEntry;
  state: "saved" | "added" | "changed" | "removed";
};

/** Builds the table rows: saved keys with their pending state, then keys that are only pending, by name. */
export function buildRows(listing: SecretConfigListing, pending: PendingChanges): SecretRow[] {
  const saved = listing.secrets.map((entry) => entry.name);
  const summary = summarizePending(pending, saved);
  const rows: SecretRow[] = listing.secrets.map((entry) => ({
    name: entry.name,
    entry,
    state: summary.removed.includes(entry.name)
      ? "removed"
      : summary.changed.includes(entry.name)
        ? "changed"
        : "saved",
  }));

  return [...rows, ...summary.added.map((name): SecretRow => ({ name, state: "added" }))].sort(
    (a, b) => a.name.localeCompare(b.name),
  );
}

const maxValueBytes = 64 * 1024;

/** Returns the message that explains why value cannot be stored, or "" when it can. */
export function secretValueError(value: string): string {
  return new TextEncoder().encode(value).length > maxValueBytes
    ? "Values can be at most 64 KiB."
    : "";
}
