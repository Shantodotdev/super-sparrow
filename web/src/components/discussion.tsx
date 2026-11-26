import { useKeyboardShortcuts } from "./keyboard-shortcuts-provider";
import { matchesChord } from "../lib/shortcuts";
import { Textarea } from "./input";
import { useState, type FormEvent, type KeyboardEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { formatRelative } from "../lib/format";
import type { Comment } from "../lib/types";
import { useAddComment } from "../queries/findings";
import { findingCommentsQuery, issueCommentsQuery } from "../queries/issue-resources";
import { useActor } from "../queries/project";
import { Avatar } from "./avatar";
import { reviewActorName } from "../lib/review";
import { MarkdownView } from "./markdown-view";

type DiscussionProps = {
  issueKey: string;
  /** The finding whose discussion to show, or omitted for the issue's own comments. */
  finding?: { round: number; number: number };
  variant?: "default" | "review";
};

/** Puts each reply right after the comment it answers, keeping the order comments were made in. */
function threaded(comments: Comment[]): { comment: Comment; reply: boolean }[] {
  const replies = (parentId: string) => comments.filter((c) => c.parent_comment_id === parentId);
  const ids = new Set(comments.map((comment) => comment.id));
  const roots = comments.filter((c) => !c.parent_comment_id || !ids.has(c.parent_comment_id));

  return roots.flatMap((root) => [
    { comment: root, reply: false },
    ...replies(root.id).map((comment) => ({ comment, reply: true })),
  ]);
}

/** The comments on an issue or a finding, oldest first, with a box for adding one. */
export function Discussion({ issueKey, finding, variant = "default" }: DiscussionProps) {
  const { bindings } = useKeyboardShortcuts();
  const commentsQueryResult = useQuery(
    finding
      ? findingCommentsQuery(issueKey, finding.round, finding.number)
      : issueCommentsQuery(issueKey),
  );
  const actor = useActor();
  const addComment = useAddComment();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [body, setBody] = useState("");

  const comments = commentsQueryResult.data ?? [];

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!body.trim() || pending) return;

    setPending(true);
    try {
      await addComment(issueKey, body.trim(), finding);
      setBody("");
      setError("");
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not add comment");
    } finally {
      setPending(false);
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!matchesChord(event, bindings.submit)) return;
    event.preventDefault();
    event.currentTarget.form?.requestSubmit();
  };

  return (
    <div className={`flex min-h-0 flex-1 flex-col ${variant === "review" ? "text-[14.5px]" : ""}`}>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {commentsQueryResult.isPending && (
          <p role="status" className="text-[13px] text-[#8290a4]">
            Loading comments…
          </p>
        )}
        {variant === "review" && !commentsQueryResult.isPending && comments.length === 0 && (
          <p className="text-[13px] text-[#8290a4]">
            No comments yet. Add context or ask a question.
          </p>
        )}
        {threaded(comments).map(({ comment, reply }) => {
          const author = actor(comment.author_actor_id);
          return (
            <div
              key={comment.id}
              className={`flex gap-[10px] py-[6px] ${reply ? "pl-[44px]" : ""}`}
            >
              <Avatar actor={author} />
              <div className="min-w-0">
                <div className="flex flex-wrap items-baseline gap-[8px]">
                  <strong className="text-[#eef0f5]">
                    {variant === "review"
                      ? reviewActorName(author)
                      : (author?.display_name ?? "Unknown")}
                  </strong>
                  <small
                    className={`${variant === "review" ? "text-[12.5px]" : "text-[12px]"} text-[#8b9ab0]`}
                  >
                    {formatRelative(comment.created_at)}
                  </small>
                </div>
                <div
                  className={`mt-[4px] max-w-160 leading-normal text-[#d5dce8] ${variant === "review" ? "pb-3" : "inline-block rounded-2xl rounded-tl-[4px] bg-[#1c2029] px-4 py-[9px]"}`}
                >
                  <MarkdownView
                    markdown={comment.body}
                    className="[&_.tiptap]:min-h-0 [&_.tiptap]:leading-normal [&_.tiptap]:text-inherit [&_.tiptap>:last-child]:mb-0"
                  />
                </div>
              </div>
            </div>
          );
        })}
      </div>
      {error && <p role="alert">{error}</p>}
      <form className="mt-[16px] flex shrink-0 items-end gap-[8px]" onSubmit={submit}>
        <Textarea
          aria-label="Add a comment"
          rows={3}
          maxLength={20000}
          value={body}
          onChange={(event) => setBody(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Add a comment..."
          className="min-h-[88px] flex-1 resize-y"
        />
        <button
          className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b] disabled:cursor-default"
          disabled={pending || !body.trim()}
        >
          {pending ? "Sending…" : variant === "review" ? "Comment" : "Send"}
        </button>
      </form>
    </div>
  );
}
