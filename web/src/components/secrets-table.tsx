import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { formatRelative } from "../lib/format";
import { buildRows, canSeeHistory, revealKey, type PendingChanges } from "../lib/secrets";
import type { SecretConfigId, SecretConfigListing } from "../lib/types";
import { fetchSecretValue, refreshAccessLog } from "../queries/secrets";
import { Person } from "./person";
import { Badge, smallButtonClass, smallDangerClass } from "./secrets-ui";
import { useToast } from "./toast";

type SecretsTableProps = {
  projectKey: string;
  config: SecretConfigId;
  listing: SecretConfigListing;
  /** Whether the caller may read values; without it the table shows key names only. */
  canView: boolean;
  canEdit: boolean;
  /** Whether the caller can manage staging and production, which their history needs too. */
  canManage: boolean;
  pending: PendingChanges;
  onEdit: (name: string) => void;
  onRemove: (name: string) => void;
  onUndo: (name: string) => void;
  onHistory: (name: string) => void;
};

const mask = "••••••••••••";

const stateBadge = { added: "green", changed: "blue", removed: "red" } as const;
const stateLabel = { added: "Added", changed: "Changed", removed: "Removed" } as const;

/**
 * The keys of one config. Values stay masked until Reveal fetches one, which the server logs, and
 * a revealed value is held only in this table, never in the query cache. Revealed values are keyed
 * by name and version, and the page keys the table by project and config so none carries over.
 */
export function SecretsTable({
  projectKey,
  config,
  listing,
  canView,
  canEdit,
  canManage,
  pending,
  onEdit,
  onRemove,
  onUndo,
  onHistory,
}: SecretsTableProps) {
  const { notify } = useToast();
  const client = useQueryClient();
  const [revealed, setRevealed] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set());

  const rows = buildRows(listing, pending);
  const isDev = config === "dev";

  // A save or restore changes versions; drop plaintext that no longer matches a listed version.
  useEffect(() => {
    const current = new Set(listing.secrets.map(revealKey));
    setRevealed((values) => {
      const kept = Object.entries(values).filter(([key]) => current.has(key));

      return kept.length === Object.keys(values).length ? values : Object.fromEntries(kept);
    });
  }, [listing]);

  const markBusy = (name: string, on: boolean) =>
    setBusy((names) => {
      const next = new Set(names);
      if (on) next.add(name);
      else next.delete(name);

      return next;
    });

  const reveal = async (name: string, key: string) => {
    if (key in revealed) {
      setRevealed(({ [key]: _hidden, ...rest }) => rest);
      return;
    }

    markBusy(name, true);
    try {
      const secret = await fetchSecretValue(projectKey, config, name);
      setRevealed((current) => ({ ...current, [revealKey(secret)]: secret.value }));
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not reveal the value");
    } finally {
      markBusy(name, false);
      refreshAccessLog(client, projectKey);
    }
  };

  const copy = async (name: string) => {
    markBusy(name, true);
    try {
      const secret = await fetchSecretValue(projectKey, config, name);
      try {
        await navigator.clipboard.writeText(secret.value);
        notify(`Copied ${name}`);
      } catch {
        notify(`Could not copy ${name}. Allow clipboard access in your browser, or use Reveal.`);
      }
    } catch (error) {
      notify(error instanceof Error ? error.message : `Could not read ${name}`);
    } finally {
      markBusy(name, false);
      refreshAccessLog(client, projectKey);
    }
  };

  if (!rows.length) {
    return <p className="muted text-[#a2aec2]">No keys yet.</p>;
  }

  return (
    <div className="overflow-x-auto" data-page-cache-exclude>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            {canView && <th>Value</th>}
            <th>Updated</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const { entry, name, state } = row;
            const saved = state === "saved" || state === "changed";
            const hasValue = entry !== undefined;
            const key = entry ? revealKey(entry) : "";
            const shown = key in revealed;
            const overridden = isDev && entry?.source === "override";

            return (
              <tr key={name} className={state === "removed" ? "opacity-60" : ""}>
                <td>
                  <div className="flex flex-wrap items-center gap-2">
                    <strong
                      className={`font-mono text-[13px] ${state === "removed" ? "line-through" : ""}`}
                    >
                      {name}
                    </strong>
                    {listing.required.includes(name) && <Badge tone="amber">Required</Badge>}
                    {isDev && entry && (
                      <Badge tone={overridden ? "blue" : "neutral"}>
                        {overridden ? "Your override" : "Team default"}
                      </Badge>
                    )}
                    {state !== "saved" && (
                      <Badge tone={stateBadge[state]}>{stateLabel[state]}</Badge>
                    )}
                  </div>
                </td>
                {canView && (
                  <td>
                    {!hasValue || state === "removed" ? (
                      <span className="text-[#78869a]">
                        {hasValue ? (
                          <>
                            <span aria-hidden="true">{mask}</span>
                            <span className="sr-only">Hidden value</span>
                          </>
                        ) : (
                          "Not saved yet"
                        )}
                      </span>
                    ) : shown ? (
                      <code className="font-mono text-[13px] break-all whitespace-pre-wrap text-[#e1e8f4]">
                        {revealed[key]}
                      </code>
                    ) : (
                      <span className="font-mono text-[#78869a]">
                        <span aria-hidden="true">{mask}</span>
                        <span className="sr-only">Hidden value</span>
                      </span>
                    )}
                  </td>
                )}
                <td className="muted text-[#a2aec2]">
                  {entry ? (
                    <>
                      <div>{formatRelative(entry.updated_at)}</div>
                      <div className="mt-1 text-[12px]">
                        <Person actorId={entry.updated_by} />
                      </div>
                    </>
                  ) : (
                    "—"
                  )}
                </td>
                <td>
                  <div className="flex flex-wrap items-center gap-2">
                    {canView && hasValue && state !== "removed" && (
                      <>
                        <button
                          type="button"
                          className={smallButtonClass}
                          disabled={busy.has(name)}
                          aria-label={`${shown ? "Hide" : "Reveal"} ${name}`}
                          onClick={() => void reveal(name, key)}
                        >
                          {shown ? "Hide" : "Reveal"}
                        </button>
                        <button
                          type="button"
                          className={smallButtonClass}
                          disabled={busy.has(name)}
                          aria-label={`Copy ${name}`}
                          onClick={() => void copy(name)}
                        >
                          Copy
                        </button>
                      </>
                    )}
                    {saved && entry && canSeeHistory(config, canManage, entry) && (
                      <button
                        type="button"
                        className={smallButtonClass}
                        aria-label={`History of ${name}`}
                        onClick={() => onHistory(name)}
                      >
                        History
                      </button>
                    )}
                    {canEdit && state !== "removed" && (
                      <button
                        type="button"
                        className={smallButtonClass}
                        aria-label={`${isDev && !overridden && entry ? "Override" : "Edit"} ${name}`}
                        onClick={() => onEdit(name)}
                      >
                        {isDev && !overridden && entry ? "Override" : "Edit"}
                      </button>
                    )}
                    {canEdit && state !== "removed" && (!isDev || overridden || !entry) && (
                      <button
                        type="button"
                        className={smallDangerClass}
                        aria-label={`${isDev ? "Reset to default" : "Delete"} ${name}`}
                        onClick={() => onRemove(name)}
                      >
                        {isDev ? "Reset to default" : "Delete"}
                      </button>
                    )}
                    {canEdit && state !== "saved" && (
                      <button
                        type="button"
                        className={smallButtonClass}
                        aria-label={`Undo change to ${name}`}
                        onClick={() => onUndo(name)}
                      >
                        Undo
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
