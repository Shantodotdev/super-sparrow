import { Link } from "@tanstack/react-router";
import type { Finding, ReviewRound } from "../lib/types";
import { Icon } from "./icon";

type ReviewRoundListProps = { rounds: ReviewRound[]; findings: Finding[] };

/** Links to each review round of an issue, with the number of findings it raised. */
export function ReviewRoundList({ rounds, findings }: ReviewRoundListProps) {
  if (!rounds.length) return <p className="muted small">No review rounds yet.</p>;

  return (
    <>
      {rounds.map((round) => {
        const count = findings.filter((finding) => finding.round === round.number).length;

        return (
          <Link
            className="round-row"
            key={round.number}
            to="/$issue/reviews/$round"
            params={{ issue: round.issue, round: String(round.number) }}
          >
            <Icon name="file-text" className="issue-icon-review" />
            <span>
              <strong>Round {round.number}</strong>
              <small>
                {count} {count === 1 ? "finding" : "findings"} · {round.date} · {round.author}
              </small>
            </span>
            <Icon name="chevron-right" />
          </Link>
        );
      })}
    </>
  );
}
