import { useState, type FormEvent } from "react";
import { secretNameError, secretValueError } from "../lib/secrets";
import { Input, Textarea } from "./input";
import { Modal } from "./modal";
import { buttonClass, dialogClass, ErrorText, primaryClass } from "./secrets-ui";

type SecretEditDialogProps = {
  open: boolean;
  /** The key being edited, or empty to add a new one. */
  name: string;
  /** The value already waiting in the pending changes for this key, if any. */
  pendingValue?: string;
  /** In dev, the names of team defaults: setting one creates the caller's own override. */
  defaultNames: string[];
  onClose: () => void;
  onApply: (name: string, value: string) => void;
};

/** Adds a key or sets a new value for one. The value goes into the pending changes, not the server. */
export function SecretEditDialog({
  open,
  name,
  pendingValue,
  defaultNames,
  onClose,
  onApply,
}: SecretEditDialogProps) {
  const editing = name !== "";
  const [typedName, setTypedName] = useState("");
  const [typed, setTyped] = useState<string | null>(null);
  const [allowEmpty, setAllowEmpty] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  const shownName = editing ? name : typedName;
  const value = typed ?? pendingValue ?? "";
  const override = defaultNames.includes(shownName);
  const emptyError =
    value === "" && !allowEmpty ? "Enter a value, or confirm it should be empty." : "";
  const error = secretNameError(shownName) || secretValueError(value) || emptyError;

  const close = () => {
    setTypedName("");
    setTyped(null);
    setAllowEmpty(false);
    setSubmitted(false);
    onClose();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setSubmitted(true);
    if (error) return;

    onApply(shownName, value);
    close();
  };

  return (
    <Modal
      open={open}
      onClose={close}
      className={dialogClass}
      label={editing ? `Edit ${name}` : "Add key"}
    >
      <form onSubmit={submit}>
        <h2>{editing ? (override ? `Override ${name}` : `Edit ${name}`) : "Add key"}</h2>
        {editing && (
          <p className="muted text-[#a2aec2]">
            {pendingValue === undefined
              ? "The current value is not loaded. Enter the new value; it is saved when you review and save your changes."
              : "This is the value waiting to be saved. Change it, and save it when you review your changes."}
          </p>
        )}
        {!editing && override && (
          <p role="status" className="text-sm text-[#f0cf75]">
            {shownName} is already a team default. Adding it sets your own override of it.
          </p>
        )}
        <label>
          Name
          <Input
            autoFocus={!editing}
            readOnly={editing}
            className="font-mono text-[13px]"
            placeholder="API_KEY"
            value={shownName}
            onChange={(event) => setTypedName(event.target.value)}
          />
        </label>
        <label>
          Value
          <Textarea
            autoFocus={editing}
            rows={5}
            className="font-mono text-[13px]"
            value={value}
            onChange={(event) => setTyped(event.target.value)}
          />
        </label>
        {value === "" && (
          <label className="flex items-center gap-2 font-normal text-[#a2aec2]">
            <input
              type="checkbox"
              className="size-4 shrink-0 rounded"
              checked={allowEmpty}
              onChange={(event) => setAllowEmpty(event.target.checked)}
            />
            <span>Set this key to an empty value</span>
          </label>
        )}
        {submitted && error && <ErrorText>{error}</ErrorText>}
        <div className="mt-[22px] flex justify-end gap-[8px]">
          <button type="button" className={buttonClass} onClick={close}>
            Cancel
          </button>
          <button className={primaryClass}>{editing ? "Set value" : "Add key"}</button>
        </div>
      </form>
    </Modal>
  );
}
