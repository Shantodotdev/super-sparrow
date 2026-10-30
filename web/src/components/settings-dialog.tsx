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
      className="w-[min(100%,510px)] rounded-[10px] border border-[#303641] bg-[#12151a] p-[24px] shadow-[0_20px_70px_#000a] [&_>_.button]:mr-[8px] [&_h2]:mb-[12px] [&_p]:mb-[16px]"
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
          <div className="mx-0 my-[16px] grid gap-[8px] font-medium text-[#ccd5e1]">
            <span>Agent</span>
            <Select
              label="Agent"
              value={agent}
              onChange={(value) => setAgent(value as AgentName)}
              options={agentNames.map((agentName) => ({ value: agentName, label: agentName }))}
            />
          </div>
        )}
        <div className="mt-[22px] flex justify-end gap-[8px]">
          <button
            className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]"
            type="button"
            onClick={onClose}
          >
            Cancel
          </button>
          <button className="rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]">
            {isMember ? "Add member" : "Create token"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
