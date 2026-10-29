import { useEffect, useState } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import type { Finding } from "../lib/types";
import { useDecideFinding } from "../queries/findings";
import { reviewRoundsQuery } from "../queries/review-rounds";
import { Avatar } from "./avatar";
import { Icon } from "./icon";
import { Person } from "./person";
import { SeverityBadge } from "./severity-badge";

type MessageProps = { author: string; date?: string; children: React.ReactNode };

function Message({ author, date, children }: MessageProps) {
  return (
    <div className="discussion">
      <Avatar name={author} />
      <p>
        <strong>{author}</strong> {date && <small>{date}</small>}
        <br />
        {children}
      </p>
    </div>
  );
}

/**
 * Shows one finding with its evidence, discussion, and decision controls.
 * Pressing F, W, or L outside a text field chooses Fix, Wontfix, or Later.
 */
export function FindingDetail({ finding }: { finding: Finding }) {
  const { data: rounds } = useSuspenseQuery(reviewRoundsQuery);
  const decide = useDecideFinding();
  const [reason, setReason] = useState("");
  const [comment, setComment] = useState("");
  const [comments, setComments] = useState<string[]>([]);

  const date = rounds.find(
    (round) => round.issue === finding.issue && round.number === finding.round,
  )?.date;

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (target.matches("input, textarea, [role=combobox]") || event.metaKey || event.ctrlKey) {
        return;
      }

      const key = event.key.toLowerCase();
      if (key === "f") decide(finding, "Fix");
      if (key === "l") decide(finding, "Later");
      if (key === "w") setReason(" ");
    };

    window.addEventListener("keydown", handleShortcut);

    return () => window.removeEventListener("keydown", handleShortcut);
  }, [finding, decide]);

  const addComment = (event: React.FormEvent) => {
    event.preventDefault();
    if (!comment.trim()) return;

    setComments((items) => [...items, comment.trim()]);
    setComment("");
  };

  const confirmWontfix = (event: React.FormEvent) => {
    event.preventDefault();
    if (!reason.trim()) return;

    decide(finding, "Wontfix");
    setReason("");
  };

  return (
    <aside className="finding-detail">
      <div className="detail-heading">
        <span className="badge">{finding.id}</span>
        <strong>{finding.title}</strong>
        <SeverityBadge value={finding.severity} />
        <span className="badge detail-status">● {finding.status}</span>
      </div>
      <p className="detail-author">
        Found by <Person name={finding.author} />{" "}
        {date && <span className="muted"> · &nbsp; {date}</span>}
      </p>
      <h3>Problem</h3>
      <p>{finding.problem}</p>
      <h3>Evidence</h3>
      <div className="evidence">
        <Icon name="file-text" /> &nbsp; {finding.file}
      </div>
      <h3>Discussion</h3>
      <Message author={finding.author} date={date}>
        Please review the behavior described above.
      </Message>
      <Message author="Shanto" date={date}>
        Thanks, I’ll take a look.
      </Message>
      {comments.map((text, index) => (
        <Message author="Shanto" key={`${text}-${index}`}>
          {text}
        </Message>
      ))}
      <form className="comment-form" onSubmit={addComment}>
        <input
          aria-label="Add a comment"
          value={comment}
          onChange={(event) => setComment(event.target.value)}
          placeholder="Add a comment..."
        />
        <button className="button" disabled={!comment.trim()}>
          Send
        </button>
      </form>
      <h3>Decision</h3>
      <div className="decisions">
        <button onClick={() => decide(finding, "Fix")}>
          <Icon name="check" /> &nbsp; Fix <kbd>F</kbd>
        </button>
        <button onClick={() => setReason(" ")}>
          <Icon name="circle-slash" /> &nbsp; Wontfix <kbd>W</kbd>
        </button>
        <button onClick={() => decide(finding, "Later")}>
          <Icon name="clock" /> &nbsp; Later <kbd>L</kbd>
        </button>
      </div>
      {reason !== "" && (
        <form onSubmit={confirmWontfix}>
          <label>
            Reason
            <input required value={reason} onChange={(event) => setReason(event.target.value)} />
          </label>
          <button className="button">Confirm wontfix</button>{" "}
          <button type="button" className="text-button" onClick={() => setReason("")}>
            Cancel
          </button>
        </form>
      )}
    </aside>
  );
}
