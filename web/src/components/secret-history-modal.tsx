import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { formatRelative } from "../lib/format";
import { secretConfigLabel } from "../lib/secrets";
import type { SecretConfigId } from "../lib/types";
import { secretHistoryQuery, useRestoreSecret } from "../queries/secrets";
import { Modal } from "./modal";
import { Person } from "./person";
import { Badge, buttonClass, ErrorText, smallButtonClass } from "./secrets-ui";
import { useToast } from "./toast";

type SecretHistoryModalProps = {
  projectKey: string;
  config: SecretConfigId;
  /** The key to show; empty keeps the drawer closed. */
  name: string;
  canRestore: boolean;
  onClose: () => void;
};

/** A drawer with every version of one key, newest first. Restoring makes an older value current again. */
export function SecretHistoryModal({
  projectKey,
  config,
  name,
  canRestore,
  onClose,
}: SecretHistoryModalProps) {
  const { notify } = useToast();
  const history = useQuery({
    ...secretHistoryQuery(projectKey, config, name),
    enabled: name !== "",
  });
  const restore = useRestoreSecret(projectKey, config, name);
  const resetRestore = restore.reset;

  useEffect(() => {
    resetRestore();
  }, [name, resetRestore]);

  return (
    <Modal
      open={name !== ""}
      onClose={onClose}
      variant="drawer"
      label={`History of ${name}`}
      className="h-full w-[min(100%,480px)] overflow-y-auto border-l border-[#303641] bg-[#12151a] p-[24px]"
    >
      <h2 className="font-mono text-[16px]">{name}</h2>
      <p className="muted mb-4 text-[#a2aec2]">
        {secretConfigLabel[config]} history. Values are not shown here.
      </p>
      {history.isPending && name && <p role="status">Loading history…</p>}
      {history.error && <ErrorText>{history.error.message}</ErrorText>}
      {restore.error && <ErrorText>{restore.error.message}</ErrorText>}
      <ul className="grid">
        {history.data?.map((version, index) => (
          <li
            key={version.version}
            className="flex items-center justify-between gap-3 border-b border-[#20252c] py-3"
          >
            <div>
              <div className="flex items-center gap-2">
                <strong>Version {version.version}</strong>
                <Badge tone={version.deleted ? "red" : "green"}>
                  {version.deleted ? "Deleted" : "Set"}
                </Badge>
                {index === 0 && <Badge>Current</Badge>}
              </div>
              <div className="mt-1 flex items-center gap-2 text-[12px] text-[#a2aec2]">
                <Person actorId={version.actor_id} />
                <span>{formatRelative(version.created_at)}</span>
              </div>
            </div>
            {canRestore && index > 0 && !version.deleted && (
              <button
                type="button"
                className={smallButtonClass}
                disabled={restore.isPending}
                onClick={() =>
                  restore.mutate(version.version, {
                    onSuccess: () => notify(`Restored version ${version.version} of ${name}`),
                  })
                }
              >
                Restore
              </button>
            )}
          </li>
        ))}
      </ul>
      <div className="mt-6 flex justify-end">
        <button type="button" className={buttonClass} onClick={onClose}>
          Close
        </button>
      </div>
    </Modal>
  );
}
