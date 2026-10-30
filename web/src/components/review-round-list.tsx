import { useProjectSlugs } from "../hooks/use-project";
import { Link } from "@tanstack/react-router";
import { formatDate } from "../lib/format";
import type { Finding, Issue, ReviewRound } from "../lib/types";
import { useActor } from "../queries/project";
import { Icon } from "./icon";

type ReviewRoundListProps = { issue: Issue; rounds: ReviewRound[]; findings: Finding[] };

/** Links to each review round of an issue, with the number of findings it raised. */
export function ReviewRoundList({ issue, rounds, findings }: ReviewRoundListProps) {
  const projectSlug = useProjectSlugs();
  const actor = useActor();

  if (!rounds.length)
    return <p className="muted mt-[3px] text-[12px] text-[#a2aec2]">No review rounds yet.</p>;

  return (
    <>
      {rounds.map((round) => {
        const count = findings.filter((finding) => finding.round === round.number).length;

        return (
          <Link
            className="mx-0 my-[6px] flex w-full items-center gap-[12px] rounded-[6px] border border-[#272d35] bg-[#111419] p-[10px] text-left [&_>_span:last-child]:ml-auto [&_small]:mt-[4px] [&_small]:block [&_small]:text-[#8fa0b7]"
            key={round.number}
            to="/$projectSlug/issues/$issue/reviews/$round"
            params={{
              projectSlug: projectSlug(issue.project_id),
              issue: issue.key,
              round: String(round.number),
            }}
          >
            <Icon name="file-text" className="issue-icon-review" />
            <span>
              <strong>Round {round.number}</strong>
              <small>
                {count} {count === 1 ? "finding" : "findings"} ·{" "}
                {formatDate(round.finished_at ?? round.created_at)} ·{" "}
                {actor(round.author_actor_id)?.display_name ?? "Unknown"}
              </small>
            </span>
            <Icon name="chevron-right" />
          </Link>
        );
      })}
    </>
  );
}
