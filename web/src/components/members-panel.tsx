import { formatDate } from "../lib/format";
import type { Actor } from "../lib/types";
import { Avatar } from "./avatar";
import { Icon } from "./icon";

type MembersPanelProps = { actors: Actor[]; onAdd: () => void };

/** The team member table, each person with the agents they own. */
export function MembersPanel({ actors, onAdd }: MembersPanelProps) {
  const members = actors.filter((actor) => actor.kind === "human");
  const agents = actors.filter((actor) => actor.kind === "agent");

  return (
    <section className="rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]">
      <div className="mb-[20px] flex items-start justify-between gap-[20px] [&_p]:mt-[3px]">
        <div>
          <h2>Team members</h2>
          <p className="muted text-[#a2aec2]">
            {members.length} team members · {agents.length} agents
          </p>
        </div>
        <button
          className="rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]"
          onClick={onAdd}
        >
          <Icon name="plus" /> &nbsp; Add member
        </button>
      </div>
      <div className="overflow-x-auto">
        <table>
          <thead>
            <tr>
              <th>Member</th>
              <th>Agents</th>
              <th>Role</th>
              <th>Joined</th>
            </tr>
          </thead>
          <tbody>
            {members.map((member) => (
              <tr key={member.id}>
                <td>
                  <div className="flex items-center gap-[10px]">
                    <Avatar actor={member} />
                    <div>
                      <strong>{member.display_name}</strong>
                      <div className="muted mt-[3px] text-[12px] text-[#a2aec2]">
                        {member.email}
                      </div>
                    </div>
                  </div>
                </td>
                <td>
                  <div className="flex flex-wrap items-center gap-[10px]">
                    {agents
                      .filter((agent) => agent.owner_actor_id === member.id)
                      .map((agent) => (
                        <span
                          className="inline-flex items-center gap-2 rounded-[8px] border border-[#222731] bg-[#171a1f] px-[8px] py-[5px] text-[12px] whitespace-nowrap [&_.agent-icon]:h-[24px] [&_.agent-icon]:w-[24px]"
                          key={agent.id}
                        >
                          <Avatar actor={agent} />
                          {agent.display_name}
                          <span
                            className={`ml-[3px] h-2 w-2 rounded-full ${agent.disabled_at ? "bg-[#77889f]" : "bg-[#16d99b]"}`}
                          />
                          {agent.disabled_at ? "Disabled" : "Active"}
                        </span>
                      ))}
                  </div>
                </td>
                <td>
                  <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
                    {member.disabled_at ? "disabled" : member.role}
                  </span>
                </td>
                <td className="muted text-[#a2aec2]">{formatDate(member.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
