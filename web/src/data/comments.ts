import type { Actor, Comment } from "../lib/types";
import { at } from "./project";
import { agents, people } from "./team";

function comment(
  id: string,
  issueNumber: number,
  findingId: string,
  author: Actor,
  body: string,
  time: string,
  parentId = "",
): Comment {
  const created = at("2024-11-06", time);
  return {
    id: `comment-${id}`,
    issue_id: `issue-${issueNumber}`,
    finding_id: findingId,
    parent_comment_id: parentId,
    body,
    author_actor_id: author.id,
    edited_at: null,
    created_at: created,
    updated_at: created,
  };
}

export const seedComments: Comment[] = [
  comment("1", 142, "finding-142-2-1", agents.shantoCodex, "The session TTL is shorter than the code lifetime, so a slow sign-in fails.", "11:35"),
  comment("2", 142, "finding-142-2-1", people.shanto, "Let's fix this and add tests.", "11:37", "comment-1"),
  comment("3", 142, "", people.priya, "The code screen looks good on mobile too.", "12:10"),
  comment("4", 142, "", agents.shantoClaude, "Ticked the backend and screens items in the plan.", "12:40"),
  comment("5", 154, "finding-154-2-1", people.shanto, "Refuse the second decision with a conflict rather than merging them.", "14:02"),
];
