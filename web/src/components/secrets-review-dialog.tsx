import { secretConfigLabel, type ChangeSummary } from "../lib/secrets";
import type { SecretConfigId } from "../lib/types";
import { Modal } from "./modal";
import { buttonClass, dialogClass, ErrorText, primaryClass } from "./secrets-ui";

type SecretsReviewDialogProps = {
  open: boolean;
  config: SecretConfigId;
  summary: ChangeSummary;
  /** In dev, the changed names that are team defaults, which become the caller's own overrides. */
  overrides: string[];
  confirmed: boolean;
  saving: boolean;
  error: string;
  onConfirmedChange: (confirmed: boolean) => void;
  onClose: () => void;
  onConfirm: () => void;
};

function NameGroup({ title, names }: { title: string; names: string[] }) {
  if (!names.length) return null;

  return (
    <section>
      <h3 className="mb-1 text-sm font-semibold text-[#ccd5e1]">
        {title} ({names.length})
      </h3>
      <ul className="grid gap-1 font-mono text-[13px] text-[#c5cfdd]">
        {names.map((name) => (
          <li key={name}>{name}</li>
        ))}
      </ul>
    </section>
  );
}

/** Lists the key names a save will add, change, and remove, never their values. Production needs a confirmation. */
export function SecretsReviewDialog({
  open,
  config,
  summary,
  overrides,
  confirmed,
  saving,
  error,
  onConfirmedChange,
  onClose,
  onConfirm,
}: SecretsReviewDialogProps) {
  const needsConfirm = config === "production";

  const close = () => {
    if (!saving) onClose();
  };

  return (
    <Modal
      open={open}
      onClose={close}
      className={dialogClass}
      label={`Review ${secretConfigLabel[config]} changes`}
    >
      <h2>Review changes to {secretConfigLabel[config]}</h2>
      <p className="muted text-[#a2aec2]">Only names are shown. Values are never listed here.</p>
      <div className="grid gap-4">
        <NameGroup title="Added" names={summary.added} />
        <NameGroup title="Override" names={overrides} />
        <NameGroup
          title="Changed"
          names={summary.changed.filter((name) => !overrides.includes(name))}
        />
        <NameGroup
          title={config === "dev" ? "Reset to default" : "Delete"}
          names={summary.removed}
        />
      </div>
      {needsConfirm && (
        <label className="flex items-start gap-3 rounded-lg border border-[#6b5420] bg-[#2a2210] p-3 font-normal text-[#f0cf75]">
          <input
            type="checkbox"
            className="mt-1 size-4 shrink-0 rounded"
            checked={confirmed}
            onChange={(event) => onConfirmedChange(event.target.checked)}
          />
          <span>I understand these changes apply to production.</span>
        </label>
      )}
      {error && <ErrorText>{error}</ErrorText>}
      <div className="mt-[22px] flex justify-end gap-[8px]">
        <button type="button" className={buttonClass} disabled={saving} onClick={close}>
          Cancel
        </button>
        <button
          type="button"
          className={primaryClass}
          disabled={saving || (needsConfirm && !confirmed)}
          onClick={onConfirm}
        >
          {saving ? "Saving…" : "Save changes"}
        </button>
      </div>
    </Modal>
  );
}
