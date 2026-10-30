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
    <div className="mx-0 my-[12px] flex gap-[10px] text-[#b2bfd0] [&_p]:leading-normal [&_small]:ml-[6px] [&_small]:text-[#8b9ab0] [&_strong]:text-[#eef0f5]">
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
    <aside className="min-w-0 border-l border-l-[#252a32] px-8 py-[22px] max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:border-t-[#252a32] max-[700px]:px-[16px] max-[700px]:py-[20px] [&_>_p:not(.detail-author)]:leading-normal [&_>_p:not(.detail-author)]:text-[#b2bfd0] [&_h3]:mx-0 [&_h3]:mt-[24px] [&_h3]:mb-[9px] [&_h3]:text-[14px] [&_h3]:font-semibold">
      <div className="flex flex-wrap items-center gap-[10px] [&_strong]:text-[16px]">
        <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
          {finding.id}
        </span>
        <strong>{finding.title}</strong>
        <SeverityBadge value={finding.severity} />
        <span className="ml-auto inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
          ● {finding.status}
        </span>
      </div>
      <p className="detail-author mt-[18px] flex flex-wrap items-center gap-[6px] text-[#aebbd0]">
        Found by <Person name={finding.author} />{" "}
        {date && <span className="muted text-[#a2aec2]"> · &nbsp; {date}</span>}
      </p>
      <h3>Problem</h3>
      <p>{finding.problem}</p>
      <h3>Evidence</h3>
      <div className="rounded-[6px] border border-[#303640] p-[10px] text-[13px] text-[#b0c8f5]">
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
      <form className="mt-[16px] flex gap-[8px]" onSubmit={addComment}>
        <input
          aria-label="Add a comment"
          value={comment}
          onChange={(event) => setComment(event.target.value)}
          placeholder="Add a comment..."
        />
        <button
          className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]"
          disabled={!comment.trim()}
        >
          Send
        </button>
      </form>
      <h3>Decision</h3>
      <div className="flex gap-[10px] [&_button]:flex-1 [&_button]:rounded-full [&_button]:border [&_button]:border-[#343b49] [&_button]:bg-[#141820] [&_button]:p-[10px] [&_button:first-child]:border-[#5278ff] [&_button:first-child]:bg-[#19284e] [&_kbd]:float-right">
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
          <button className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]">
            Confirm wontfix
          </button>{" "}
          <button
            type="button"
            className="text-button border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
            onClick={() => setReason("")}
          >
            Cancel
          </button>
        </form>
      )}
    </aside>
  );
}
