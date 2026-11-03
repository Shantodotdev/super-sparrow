import { useState, type FormEvent } from "react";
import { isValidSecretName, parseNameList, secretConfigLabel } from "../lib/secrets";
import type { SecretConfigId } from "../lib/types";
import { useSetRequiredKeys } from "../queries/secrets";
import { Textarea } from "./input";
import { Modal } from "./modal";
import { buttonClass, dialogClass, ErrorText, primaryClass } from "./secrets-ui";

type RequiredKeysDialogProps = {
  open: boolean;
  projectKey: string;
  /** The config whose list is edited. Team dev defaults share the list of dev. */
  config: SecretConfigId;
  names: string[];
  onClose: () => void;
};

/** Edits the list of keys a config must have, one name per line. */
export function RequiredKeysDialog({
  open,
  projectKey,
  config,
  names,
  onClose,
}: RequiredKeysDialogProps) {
  const target = config === "dev-defaults" ? "dev" : config;
  const save = useSetRequiredKeys(projectKey, target);
  const [text, setText] = useState<string | null>(null);

  const draft = text ?? names.join("\n");
  const parsed = parseNameList(draft);
  const invalid = parsed.filter((name) => !isValidSecretName(name));

  const close = () => {
    if (save.isPending) return;
    setText(null);
    save.reset();
    onClose();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (invalid.length) return;

    save.mutate(parsed, { onSuccess: close });
  };

  return (
    <Modal open={open} onClose={close} className={dialogClass} label="Required keys">
      <form onSubmit={submit}>
        <h2>Required keys for {target === "dev" ? "dev" : secretConfigLabel[target]}</h2>
        <p className="muted text-[#a2aec2]">
          One name per line. Sparrow warns when a required key has no value.
        </p>
        <Textarea
          autoFocus
          rows={8}
          aria-label="Required key names"
          className="font-mono text-[13px]"
          value={draft}
          onChange={(event) => setText(event.target.value)}
        />
        {invalid.length > 0 && <ErrorText>Not valid names: {invalid.join(", ")}</ErrorText>}
        {save.error && <ErrorText>{save.error.message}</ErrorText>}
        <div className="mt-[22px] flex justify-end gap-[8px]">
          <button type="button" className={buttonClass} disabled={save.isPending} onClick={close}>
            Cancel
          </button>
          <button className={primaryClass} disabled={save.isPending || invalid.length > 0}>
            Save list
          </button>
        </div>
      </form>
    </Modal>
  );
}
