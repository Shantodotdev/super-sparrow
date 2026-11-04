import { useState } from "react";
import { parseDotenv, secretValueError } from "../lib/secrets";
import { Textarea } from "./input";
import { Modal } from "./modal";
import { buttonClass, dialogClass, ErrorText, primaryClass } from "./secrets-ui";

type PasteEnvDialogProps = {
  open: boolean;
  onClose: () => void;
  onApply: (entries: { name: string; value: string }[]) => void;
};

/** Adds many keys at once from pasted NAME=value lines, parsed in the browser. */
export function PasteEnvDialog({ open, onClose, onApply }: PasteEnvDialogProps) {
  const [text, setText] = useState("");

  const { entries, errors } = parseDotenv(text);
  const oversized = entries.filter((entry) => secretValueError(entry.value));
  const problems = [
    ...errors,
    ...oversized.map((entry) => ({ line: entry.line, message: `${entry.name}: too large.` })),
  ].sort((a, b) => a.line - b.line);

  const close = () => {
    setText("");
    onClose();
  };

  return (
    <Modal open={open} onClose={close} className={dialogClass} label="Paste .env">
      <h2>Paste .env</h2>
      <p className="muted text-[#a2aec2]">
        Paste NAME=value lines. Comments and blank lines are ignored. The values stay in your
        browser until you save.
      </p>
      <Textarea
        autoFocus
        rows={10}
        aria-label=".env contents"
        className="font-mono text-[13px]"
        placeholder={"API_KEY=abc123\nDATABASE_URL=postgres://…"}
        value={text}
        onChange={(event) => setText(event.target.value)}
      />
      {problems.length > 0 && (
        <ul className="mt-3 grid gap-1" aria-label="Problems">
          {problems.map((problem) => (
            <li key={problem.line}>
              <ErrorText>
                Line {problem.line}: {problem.message}
              </ErrorText>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-[22px] flex items-center justify-end gap-[8px]">
        {entries.length > 0 && problems.length === 0 && (
          <span className="mr-auto text-sm text-[#a2aec2]">
            {entries.length} {entries.length === 1 ? "key" : "keys"} found
          </span>
        )}
        <button type="button" className={buttonClass} onClick={close}>
          Cancel
        </button>
        <button
          type="button"
          className={primaryClass}
          disabled={entries.length === 0 || problems.length > 0}
          onClick={() => {
            onApply(entries);
            close();
          }}
        >
          Add to changes
        </button>
      </div>
    </Modal>
  );
}
