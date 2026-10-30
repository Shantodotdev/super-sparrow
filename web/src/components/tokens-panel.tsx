import type { AgentToken } from "../lib/types";
import { Icon } from "./icon";
import { Person } from "./person";

type TokensPanelProps = {
  tokens: AgentToken[];
  onCreate: () => void;
  onRevoke: (id: string) => void;
};

/** The agent token table. */
export function TokensPanel({ tokens, onCreate, onRevoke }: TokensPanelProps) {
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
                </td>
                <td>{token.owner}</td>
                <td>
                  <Person name={token.agent} />
                </td>
                <td>{token.expires}</td>
                <td>{token.lastUsed}</td>
                <td>
                  <button
                    className="text-button border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
                    onClick={() => onRevoke(token.id)}
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
