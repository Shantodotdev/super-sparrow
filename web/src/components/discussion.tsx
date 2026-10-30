import { useState, type FormEvent } from "react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { formatDate } from "../lib/format";
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

  return (
    <>
      {comments.length === 0 && <p className="muted text-[#a2aec2]">No comments yet.</p>}
      {threaded(comments).map(({ comment, reply }) => {
        const author = actor(comment.author_actor_id);
        return (
          <div
            key={comment.id}
            className={`mx-0 my-[12px] flex gap-[10px] text-[#b2bfd0] [&_p]:leading-normal [&_small]:ml-[6px] [&_small]:text-[#8b9ab0] [&_strong]:text-[#eef0f5] ${reply ? "ml-[46px]" : ""}`}
          >
            <Avatar actor={author} />
            <p>
              <strong>{author?.display_name ?? "Unknown"}</strong>{" "}
              <small>{formatDate(comment.created_at)}</small>
              <br />
              <span className="whitespace-pre-wrap">{comment.body}</span>
            </p>
          </div>
        );
      })}
      {error && <p role="alert">{error}</p>}
      <form className="mt-[16px] flex gap-[8px]" onSubmit={submit}>
        <input
          aria-label="Add a comment"
          value={body}
          onChange={(event) => setBody(event.target.value)}
          placeholder="Add a comment..."
        />
        <button
          className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]"
          disabled={pending || !body.trim()}
        >
          Send
        </button>
      </form>
    </>
  );
}
