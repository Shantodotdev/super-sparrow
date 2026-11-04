import { useState, type FormEvent } from "react";
import { Check, Copy } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { formatDate, formatRelative } from "../lib/format";
import { secretConfigLabel, tokenState } from "../lib/secrets";
import type { NewServiceToken, ServiceToken } from "../lib/types";
import {
  serviceTokensQuery,
  useCreateServiceToken,
  useRenewServiceToken,
  useRevokeServiceToken,
} from "../queries/secrets";
import { Icon } from "./icon";
import { Input } from "./input";
import { Modal } from "./modal";
import { Person } from "./person";
import { Select } from "./select";
import {
  Badge,
  buttonClass,
  dangerClass,
  dialogClass,
  ErrorText,
  Panel,
  primaryClass,
  smallButtonClass,
  smallDangerClass,
} from "./secrets-ui";
import { useToast } from "./toast";

const tokenConfigs = ["staging", "production"] as const;

function TokenStatus({ token, now }: { token: ServiceToken; now: number }) {
  const state = tokenState(token, now);
  if (state === "revoked") return <Badge tone="red">Revoked</Badge>;
  if (state === "expired") return <Badge tone="red">Expired {formatDate(token.expires_at)}</Badge>;
  if (state === "expiring")
    return <Badge tone="amber">Expires {formatDate(token.expires_at)}</Badge>;

  return <span>{formatDate(token.expires_at)}</span>;
}

/** Creates a service token, then shows its secret once with a copy button. */
function CreateTokenDialog({
  open,
  projectKey,
  onClose,
}: {
  open: boolean;
  projectKey: string;
  onClose: () => void;
}) {
  const create = useCreateServiceToken(projectKey);
  const [config, setConfig] = useState<(typeof tokenConfigs)[number]>("staging");
  const [label, setLabel] = useState("");
  const [created, setCreated] = useState<NewServiceToken | null>(null);
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const [warnUncopied, setWarnUncopied] = useState(false);

  const close = () => {
    if (create.isPending) return;
    setLabel("");
    setCreated(null);
    setCopied(false);
    setCopyFailed(false);
    setWarnUncopied(false);
    create.reset();
    onClose();
  };

  // Escape and clicks outside must not discard a secret that was never copied and cannot be shown again.
  const dismiss = () => {
    if (created && !copied) {
      setWarnUncopied(true);
      return;
    }

    close();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    create.mutate({ config, label }, { onSuccess: setCreated });
  };

  const copy = async (secret: string) => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  };

  return (
    <Modal open={open} onClose={dismiss} className={dialogClass} label="Create service token">
      {created ? (
        <div>
          <h2>Copy your token now</h2>
          <p className="muted text-[#a2aec2]">
            This token reads the whole {secretConfigLabel[created.token.config]} config. You won’t
            see it again, so copy it before closing this window.
          </p>
          <div className="relative">
            <Input
              readOnly
              aria-label="New service token"
              className="w-full pr-12 font-mono text-[13px]"
              value={created.secret}
              onFocus={(event) => event.target.select()}
            />
            <button
              type="button"
              aria-label="Copy token"
              title={copied ? "Copied" : "Copy token"}
              onClick={() => void copy(created.secret)}
              className="absolute top-1/2 right-2 grid size-8 -translate-y-1/2 place-items-center rounded-md border-0 bg-[#1a202a] text-[#a9bad2] hover:bg-[#283344] hover:text-white focus-visible:outline-2 focus-visible:outline-[#6486ff]"
            >
              {copied ? (
                <Check aria-hidden="true" className="size-4 text-[#65dca4]" />
              ) : (
                <Copy aria-hidden="true" className="size-4" />
              )}
            </button>
            <span role="status" className="sr-only">
              {copied ? "Token copied" : ""}
            </span>
          </div>
          {copyFailed && (
            <ErrorText>
              Could not copy the token. Select it above and copy it by hand before closing.
            </ErrorText>
          )}
          {warnUncopied && !copied && (
            <ErrorText>
              You have not copied this token, and it cannot be shown again. Copy it, or press Done
              to close anyway.
            </ErrorText>
          )}
          <div className="mt-[22px] flex justify-end">
            <button type="button" className={primaryClass} onClick={close}>
              Done
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={submit}>
          <h2>Create service token</h2>
          <p className="muted text-[#a2aec2]">
            A service token lets CI read one whole config. It lasts one year and cannot write.
          </p>
          <div className="mx-0 my-[16px] grid gap-[8px] font-medium text-[#ccd5e1]">
            <span aria-hidden="true">Config</span>
            <Select
              label="Config"
              value={config}
              onChange={(value) => setConfig(value as (typeof tokenConfigs)[number])}
              options={tokenConfigs.map((value) => ({
                value,
                label: secretConfigLabel[value],
              }))}
            />
          </div>
          <label>
            Label, such as the CI job
            <Input
              autoFocus
              required
              maxLength={100}
              disabled={create.isPending}
              value={label}
              onChange={(event) => setLabel(event.target.value)}
            />
          </label>
          {create.error && <ErrorText>{create.error.message}</ErrorText>}
          <div className="mt-[22px] flex justify-end gap-[8px]">
            <button
              type="button"
              className={buttonClass}
              disabled={create.isPending}
              onClick={close}
            >
              Cancel
            </button>
            <button className={primaryClass} disabled={create.isPending || !label.trim()}>
              Create token
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

/** The service tokens CI uses to download staging or production. Shown only to people who can manage. */
export function ServiceTokensPanel({ projectKey }: { projectKey: string }) {
  const { notify } = useToast();
  const tokens = useQuery(serviceTokensQuery(projectKey));
  const renew = useRenewServiceToken(projectKey);
  const revoke = useRevokeServiceToken(projectKey);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<ServiceToken | null>(null);
  const [revokeOpen, setRevokeOpen] = useState(false);

  const now = Date.now();

  return (
    <>
      <Panel
        title="Service tokens"
        description="Tokens that let CI download staging or production. Each one reads a single config."
        action={
          <button
            type="button"
            className={`${primaryClass} inline-flex items-center gap-2`}
            onClick={() => setCreating(true)}
          >
            <Icon name="plus" /> Create token
          </button>
        }
      >
        {tokens.error && <ErrorText>{tokens.error.message}</ErrorText>}
        {tokens.isPending && <p role="status">Loading tokens…</p>}
        {tokens.data && !tokens.data.length && (
          <p className="muted text-[#a2aec2]">No service tokens yet.</p>
        )}
        {tokens.data && tokens.data.length > 0 && (
          <div className="overflow-x-auto">
            <table>
              <thead>
                <tr>
                  <th>Label</th>
                  <th>Config</th>
                  <th>Created by</th>
                  <th>Expires</th>
                  <th>Last used</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {tokens.data.map((token) => (
                  <tr key={token.id} className={token.revoked_at ? "opacity-60" : ""}>
                    <td>
                      <strong>{token.label}</strong>
                      <div className="muted mt-[3px] text-[12px] text-[#a2aec2]">
                        {token.display_hint}
                      </div>
                    </td>
                    <td>{secretConfigLabel[token.config]}</td>
                    <td>
                      <Person actorId={token.created_by} />
                    </td>
                    <td>
                      <TokenStatus token={token} now={now} />
                    </td>
                    <td>{token.last_used_at ? formatRelative(token.last_used_at) : "Never"}</td>
                    <td>
                      {!token.revoked_at && (
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            className={smallButtonClass}
                            disabled={renew.isPending}
                            onClick={() =>
                              renew.mutate(token.id, {
                                onSuccess: () => notify(`Renewed ${token.label}`),
                                onError: (error) => notify(error.message),
                              })
                            }
                          >
                            Renew
                          </button>
                          <button
                            type="button"
                            className={smallDangerClass}
                            onClick={() => {
                              setRevoking(token);
                              setRevokeOpen(true);
                            }}
                          >
                            Revoke
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
      <CreateTokenDialog
        open={creating}
        projectKey={projectKey}
        onClose={() => setCreating(false)}
      />
      <Modal
        open={revokeOpen}
        onClose={() => {
          if (!revoke.isPending) setRevokeOpen(false);
        }}
        className={dialogClass}
        label="Revoke service token"
      >
        <h2>Revoke “{revoking?.label}”?</h2>
        <p className="muted text-[#a2aec2]">
          Anything using this token stops being able to download{" "}
          {revoking ? secretConfigLabel[revoking.config] : "the config"} right away.
        </p>
        <div className="mt-[22px] flex justify-end gap-[8px]">
          <button
            autoFocus
            type="button"
            className={buttonClass}
            disabled={revoke.isPending}
            onClick={() => setRevokeOpen(false)}
          >
            Cancel
          </button>
          <button
            type="button"
            className={dangerClass}
            disabled={revoke.isPending}
            onClick={() => {
              if (!revoking) return;
              revoke.mutate(revoking.id, {
                onSuccess: () => setRevokeOpen(false),
                onError: (error) => notify(error.message),
              });
            }}
          >
            Revoke token
          </button>
        </div>
      </Modal>
    </>
  );
}
