import type { TeamMember } from "../lib/types";
import { Avatar } from "./avatar";
import { Icon } from "./icon";

type MembersPanelProps = { members: TeamMember[]; onAdd: () => void };

/** The team member table. */
export function MembersPanel({ members, onAdd }: MembersPanelProps) {
  const agentCount = members.reduce((total, member) => total + member.agents.length, 0);

  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>Team members</h2>
          <p className="muted">
            {members.length} team members · {agentCount} agents
          </p>
        </div>
        <button className="primary" onClick={onAdd}>
          <Icon name="plus" /> &nbsp; Add member
        </button>
      </div>
      <div className="table-wrap">
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
              <tr key={member.email}>
                <td>
                  <div className="person">
                    <Avatar name={member.name} />
                    <div>
                      <strong>{member.name}</strong>
                      <div className="muted small">{member.email}</div>
                    </div>
                  </div>
                </td>
                <td>
                  <div className="agent-list">
                    {member.agents.map((agent) => (
                      <span className="agent-pill" key={agent}>
                        <Avatar name={agent} />
                        {agent}
                        <span className="dot" />
                        Active
                      </span>
                    ))}
                  </div>
                </td>
                <td>
                  <span className="badge">{member.role}</span>
                </td>
                <td className="muted">{member.joined}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
