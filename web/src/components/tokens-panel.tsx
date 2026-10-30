import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiWrite } from "../lib/api";
import { useToast } from "./toast";
import { formatDate } from "../lib/format";
import type { AgentToken } from "../lib/types";
import { useActor } from "../queries/project";
import { Icon } from "./icon";
import { Person } from "./person";

type TokensPanelProps = {
  tokens: AgentToken[];
  onCreate: () => void;
  /** Asks to revoke a token; the page confirms before anything is revoked. */
  onRevoke: (token: AgentToken) => void;
};

/** The agent token table. Tokens show only their hint; the secret is shown once, when created. */
export function TokensPanel({ tokens, onCreate, onRevoke }: TokensPanelProps) {
  const actor = useActor();
  const client = useQueryClient();
  const { notify } = useToast();
  const renew = useMutation({
    mutationFn: (id: string) => apiWrite(`/api/tokens/${id}/renew`, "POST"),
    onSuccess: () => client.invalidateQueries(),
    onError: (error) => notify(error.message),
  });
  const now = Date.now();

  return (
    <section className="rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]">
      <div className="mb-[20px] flex items-start justify-between gap-[20px] [&_p]:mt-[3px]">
        <div>
          <h2>Agent tokens</h2>
          <p className="muted text-[#a2aec2]">Manage the tokens your agents use to connect.</p>
        </div>
        <button
          className="rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]"
          onClick={onCreate}
        >
          <Icon name="plus" /> &nbsp; Create token
        </button>
      </div>
      <div className="overflow-x-auto">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Owned by</th>
              <th>Agent</th>
              <th>Expires</th>
              <th>Last used</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {tokens.map((token) => (
              <tr key={token.id}>
                <td>
                  <strong>{token.label}</strong>
                  <div className="muted mt-[3px] text-[12px] text-[#a2aec2]">
                    {token.display_hint}
                  </div>
                </td>
                <td>{actor(token.owner_actor_id)?.display_name ?? "Unknown"}</td>
                <td>
                  <Person actorId={token.agent_actor_id} />
                </td>
                <td>
                  {new Date(token.expires_at).getTime() <= now ? "Expired " : ""}
                  {formatDate(token.expires_at)}
                </td>
                <td>{token.last_used_at ? formatDate(token.last_used_at) : "Never"}</td>
                <td>
                  <button
                    className="mr-3"
                    disabled={renew.isPending}
                    onClick={() => renew.mutate(token.id)}
                  >
                    Renew
                  </button>
                  <button
                    className="text-button border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
                    onClick={() => onRevoke(token)}
                  >
                    Revoke
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
