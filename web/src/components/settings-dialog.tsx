import { useRef, useState, type FormEvent } from "react";
import { agentNames } from "../data/team";
import type { AgentName } from "../lib/types";
import { useAddMember, useAddToken } from "../queries/team";
import { Modal } from "./modal";
import { Select } from "./select";

export type SettingsDialogKind = "member" | "token";

type SettingsDialogProps = { kind: SettingsDialogKind | null; onClose: () => void };

/** A form for adding a team member or creating an agent token; `kind` null keeps it closed. */
export function SettingsDialog({ kind, onClose }: SettingsDialogProps) {
  const addMember = useAddMember();
  const addToken = useAddToken();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [agent, setAgent] = useState<AgentName>("Claude Code");

  // Remember the last kind so the title and fields do not change while the dialog animates out.
  const lastKind = useRef<SettingsDialogKind>("member");
  if (kind) lastKind.current = kind;
  const shownKind = kind ?? lastKind.current;
  const isMember = shownKind === "member";

  const submit = (event: FormEvent) => {
    event.preventDefault();

    if (isMember) addMember({ name, email });
    else addToken({ label: name, agent });

    setName("");
    setEmail("");
    onClose();
  };

  return (
    <Modal
      open={kind !== null}
      onClose={onClose}
      className="modal"
      label={isMember ? "Add member" : "Create agent token"}
    >
      <form onSubmit={submit}>
        <h2>{isMember ? "Add member" : "Create agent token"}</h2>
        <label>
          {isMember ? "Name" : "Token label"}
          <input
            autoFocus
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        {isMember ? (
          <label>
            Email
            <input
              type="email"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </label>
        ) : (
          <div className="select-field">
            <span>Agent</span>
            <Select
              label="Agent"
              value={agent}
              onChange={(value) => setAgent(value as AgentName)}
              options={agentNames.map((agentName) => ({ value: agentName, label: agentName }))}
            />
          </div>
        )}
        <div className="modal-actions">
          <button className="button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary">{isMember ? "Add member" : "Create token"}</button>
        </div>
      </form>
    </Modal>
  );
}
