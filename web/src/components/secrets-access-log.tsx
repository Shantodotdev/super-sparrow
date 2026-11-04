import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { formatRelative } from "../lib/format";
import { secretConfigLabel } from "../lib/secrets";
import type { SecretConfigId } from "../lib/types";
import { accessLogQuery } from "../queries/secrets";
import { Person } from "./person";
import { Select } from "./select";
import { buttonClass, ErrorText, Panel } from "./secrets-ui";

const shownNames = 4;

function namesText(names: string[]): string {
  if (names.length <= shownNames) return names.join(", ");

  return `${names.slice(0, shownNames).join(", ")} and ${names.length - shownNames} more`;
}

/**
 * Who revealed or downloaded values. Everyone sees the log of their own dev; owners can switch to
 * staging and production.
 */
export function SecretsAccessLog({
  projectKey,
  isOwner,
}: {
  projectKey: string;
  isOwner: boolean;
}) {
  const [config, setConfig] = useState<SecretConfigId>("dev");
  const log = useInfiniteQuery(accessLogQuery(projectKey, config));

  const options: SecretConfigId[] = isOwner ? ["dev", "staging", "production"] : ["dev"];
  const entries = log.data?.pages.flatMap((page) => page.entries) ?? [];

  return (
    <Panel
      title="Access log"
      description={
        isOwner
          ? "Every reveal and download of a value. Dev shows only your own."
          : "Every reveal and download of your own dev values."
      }
      action={
        options.length > 1 ? (
          <Select
            label="Config"
            value={config}
            onChange={(value) => setConfig(value as SecretConfigId)}
            options={options.map((value) => ({ value, label: secretConfigLabel[value] }))}
          />
        ) : undefined
      }
    >
      {log.error && <ErrorText>{log.error.message}</ErrorText>}
      {log.isPending && <p role="status">Loading access log…</p>}
      {log.data && !entries.length && <p className="muted text-[#a2aec2]">Nothing yet.</p>}
      {entries.length > 0 && (
        <div className="overflow-x-auto">
          <table>
            <thead>
              <tr>
                <th>When</th>
                <th>Who</th>
                <th>What</th>
                <th>Keys</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id}>
                  <td className="muted text-[#a2aec2]">{formatRelative(entry.created_at)}</td>
                  <td>{entry.actor_id ? <Person actorId={entry.actor_id} /> : "Service token"}</td>
                  <td>
                    {entry.action === "download" ? "Downloaded" : "Revealed"}{" "}
                    {secretConfigLabel[entry.config]}
                  </td>
                  <td className="font-mono text-[13px]">{namesText(entry.names)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {log.hasNextPage && (
        <div className="mt-4 flex justify-center">
          <button
            type="button"
            className={buttonClass}
            disabled={log.isFetchingNextPage}
            onClick={() => void log.fetchNextPage()}
          >
            {log.isFetchingNextPage ? "Loading…" : "Load more"}
          </button>
        </div>
      )}
    </Panel>
  );
}
