import { slugify } from "../lib/format";
import type { Finding } from "../lib/types";
import { Icon } from "./icon";
import { Person } from "./person";
import { SeverityBadge } from "./severity-badge";

type FindingCardProps = {
  finding: Finding;
  position: number;
  selected: boolean;
  onSelect: () => void;
};

/** A finding in the review round list; `position` is its 1-based place in the round. */
export function FindingCard({ finding, position, selected, onSelect }: FindingCardProps) {
  return (
    <button className={`finding-card ${selected ? "selected" : ""}`} onClick={onSelect}>
      <span className={`severity-symbol ${slugify(finding.severity)}`}>
        <Icon name={finding.severity === "Nit" ? "info" : "triangle-alert"} />
      </span>
      <div className="finding-card-main">
        <div className="finding-title">
          <span className="badge">{finding.id}</span> <strong>{finding.title}</strong>{" "}
          <SeverityBadge value={finding.severity} />
        </div>
        <p>{finding.problem}</p>
        <div className="finding-footer">
          <Person name={finding.author} />
          <span className="muted">
            <Icon name="message-square" /> &nbsp;{position}　{" "}
            <span className="badge">● {finding.status}</span>
          </span>
        </div>
      </div>
    </button>
  );
}
