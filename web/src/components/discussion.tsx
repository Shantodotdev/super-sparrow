import { Textarea } from "./input";
import { useState, type FormEvent, type KeyboardEvent } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { formatRelative } from "../lib/format";
import type { Comment } from "../lib/types";
import { useAddComment } from "../queries/findings";
import { commentsQuery, useActor } from "../queries/project";
import { Avatar } from "./avatar";

type DiscussionProps = {
  issueId: string;
  /** The finding whose discussion to show, or "" for the issue's own comments. */
  findingId: string;
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
export function Discussion({ issueId, findingId }: DiscussionProps) {
  const { data: allComments } = useSuspenseQuery(commentsQuery);
  const actor = useActor();
  const addComment = useAddComment();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [body, setBody] = useState("");

  const comments = allComments.filter((c) => c.issue_id === issueId && c.finding_id === findingId);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!body.trim() || pending) return;

    setPending(true);
    try {
      await addComment(issueId, findingId, body.trim());
      setBody("");
      setError("");
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not add comment");
    } finally {
      setPending(false);
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "Enter" || !(event.ctrlKey || event.metaKey)) return;
    event.preventDefault();
    event.currentTarget.form?.requestSubmit();
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        {threaded(comments).map(({ comment, reply }) => {
          const author = actor(comment.author_actor_id);
          return (
            <div
              key={comment.id}
              className={`flex gap-[10px] py-[6px] ${reply ? "pl-[44px]" : ""}`}
            >
              <Avatar actor={author} />
              <div className="min-w-0">
                <div className="flex items-baseline gap-[8px]">
                  <strong className="text-[#eef0f5]">{author?.display_name ?? "Unknown"}</strong>
                  <small className="text-[12px] text-[#8b9ab0]">
                    {formatRelative(comment.created_at)}
                  </small>
                </div>
                <p className="mt-[4px] inline-block max-w-160 rounded-2xl rounded-tl-[4px] bg-[#1c2029] px-4 py-[9px] leading-normal wrap-break-word whitespace-pre-wrap text-[#d5dce8]">
                  {comment.body}
                </p>
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
          Send
        </button>
      </form>
    </div>
  );
}
