import { Input } from "./input";
import { useRef, useState, type FormEvent } from "react";
import { agentTools } from "../data/team";
import type { NewAgentToken } from "../lib/types";
import { useAddMember, useCreateToken } from "../queries/team";
import { Modal } from "./modal";
import { Select } from "./select";

export type SettingsDialogKind = "member" | "token";

type SettingsDialogProps = { kind: SettingsDialogKind | null; onClose: () => void };

const buttonClass =
  "button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]";
const primaryClass =
  "rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]";

/**
 * A form for adding a team member or creating an agent token; `kind` null keeps it closed. After a
 * token is created, the dialog shows its secret once, with how to give it to the agent, because
 * the server never sends the secret again.
 */
export function SettingsDialog({ kind, onClose }: SettingsDialogProps) {
  const addMember = useAddMember();
  const createToken = useCreateToken();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [agentName, setAgentName] = useState(agentTools[0]!.agentName);
  const [created, setCreated] = useState<NewAgentToken | null>(null);
  const [copied, setCopied] = useState(false);

  // Remember the last kind so the title and fields do not change while the dialog animates out.
  const lastKind = useRef<SettingsDialogKind>("member");
  if (kind) lastKind.current = kind;
  const shownKind = kind ?? lastKind.current;
  const isMember = shownKind === "member";
  const toolName = agentTools.find((tool) => tool.agentName === agentName)!.label;

  const close = () => {
    setName("");
    setEmail("");
    setCreated(null);
    setCopied(false);
    onClose();
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();

    if (pending) return;
    setPending(true);
    setError("");
    try {
      if (isMember) {
        const result = await addMember({ name, email });
        if (!result.invitation_sent) {
          setError(
            "Member added, but invitation email could not be sent. They can sign in with their email.",
          );
          return;
        }
        close();
        return;
      }
      setCreated(await createToken({ label: name, agentName, toolName }));
    } catch (error) {
      setError(error instanceof Error ? error.message : "Request failed");
    } finally {
      setPending(false);
    }
  };

  const copy = async (secret: string) => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <Modal
      open={kind !== null}
      onClose={close}
      className="w-[min(100%,510px)] rounded-[10px] border border-[#303641] bg-[#12151a] p-[24px] shadow-[0_20px_70px_#000a] [&_>_.button]:mr-[8px] [&_h2]:mb-[12px] [&_p]:mb-[16px]"
      label={isMember ? "Add member" : "Create agent token"}
    >
      {created ? (
        <div>
          <h2>Copy your token now</h2>
          <p className="muted text-[#a2aec2]">
            This is the only time the token is shown. Set it as <code>SPARROW_TOKEN</code> where{" "}
            {toolName} runs; the repository's MCP configuration reads it from there. See
            &ldquo;Connecting agents&rdquo; in docs/DEPLOYMENT.md.
          </p>
          <Input
            readOnly
            aria-label="New agent token"
            value={created.secret}
            onFocus={(event) => event.target.select()}
          />
          <div className="mt-[22px] flex justify-end gap-[8px]">
            <button className={buttonClass} type="button" onClick={() => void copy(created.secret)}>
              {copied ? "Copied" : "Copy token"}
            </button>
            <button className={primaryClass} type="button" onClick={close}>
              Done
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={submit}>
          {error && <p role="alert">{error}</p>}
          <h2>{isMember ? "Add member" : "Create agent token"}</h2>
          <label>
            {isMember ? "Name" : "Token label, such as the machine"}
            <Input
              autoFocus
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          {isMember ? (
            <label>
              Email
              <Input
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
                value={agentName}
                onChange={setAgentName}
                options={agentTools.map((tool) => ({ value: tool.agentName, label: tool.label }))}
              />
            </div>
          )}
          <div className="mt-[22px] flex justify-end gap-[8px]">
            <button className={buttonClass} type="button" onClick={close}>
              Cancel
            </button>
            <button disabled={pending} className={primaryClass}>
              {isMember ? "Add member" : "Create token"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
