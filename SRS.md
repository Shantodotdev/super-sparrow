# Software requirements specification

This document defines the requirements and architecture of Tracker (working name), the first version. [PRODUCT.md](PRODUCT.md) explains what the product is and why it exists.

## 1. Scope

Version 1 covers projects, milestones, issues with plans, review rounds, findings, comments, actor identity for humans and agents, an activity history, the MCP server, the CLI, the web and desktop interfaces, a Linear importer, deployment, and backups.

Out of scope for version 1: documents, secrets, CI/CD, cycles, estimates, roadmaps, custom fields, email notifications, fine-grained permissions, and GitHub sync beyond closing an issue when its pull request merges.

## 2. Architecture

| Part       | Choice                                         | Why                                                              |
| ---------- | ---------------------------------------------- | ---------------------------------------------------------------- |
| Server     | Go, one binary                                 | Low memory, fast builds, simple deployment, agents write it well |
| Database   | SQLite in WAL mode                             | No separate database process; enough for a small team            |
| Backups    | Litestream to Cloudflare R2, encrypted         | Continuous backups with point-in-time restore                    |
| Agent API  | MCP server (streamable HTTP) inside the binary | Claude Code and Codex connect with no glue code                  |
| CLI        | Go, same repository, calls the HTTP API        | For scripts, hooks, and agents without MCP                       |
| Frontend   | SolidJS or Svelte, built with Vite             | Small bundle, fast rendering                                     |
| Live views | Server-Sent Events                             | One-way updates are all the interface needs                      |
| Desktop    | Tauri v2 wrapping the same frontend build      | Native app with low resource use                                 |

The Go server serves the HTTP API, the MCP endpoint, the event stream, and the built frontend. The desktop app loads the same frontend and talks to the same server.

```
Browser / Tauri app ──HTTP + SSE──┐
Claude Code / Codex ──MCP─────────┼──> Go server ──> SQLite ──Litestream──> R2
CLI / scripts ───────HTTP─────────┘
```

## 3. Data model

All tables have `id`, `created_at`, and `updated_at`. Records that belong to a workspace carry `workspace_id`, and every query on them filters by it.

- **Workspace:** name, slug. Version 1 runs one workspace, but the column exists so more can be added later.
- **Actor:** kind (`human` or `agent`), display name, and for agents: agent name (`claude-code`, `codex`), model, and the human it works for. Every write records its actor.
- **Token:** actor, hashed secret, label, created by, last used, revoked at. Only the hash is stored.
- **Project:** key (`WP`), name, next issue number.
- **Milestone:** project, name, order, status.
- **Issue:** project, number (shown as `WP-142`), title, plan (markdown), status, label set, parent issue, milestone, assignee, branch name, pull request URL.
- **Review round:** issue, round number, reviewed range (from commit, to commit), whether uncommitted changes were included, plan coverage note, summary, author actor.
- **Finding:** round, number within the round (R1, R2), severity, title, problem, fix, evidence, file path, line, commit, status, decision reason, found by actor, fixed by actor, fixed in commit, verified by actor, duplicate-of finding, follow-up issue.
- **Comment:** target (issue or finding), body (markdown), author actor, parent comment.
- **Event:** target, kind, actor, payload (JSON), sequence number. Append-only.

### Statuses

- **Issue:** Backlog, Todo, In Progress, In Review, Done, Canceled.
- **Finding severity:** Blocking, Should fix, Nit.
- **Finding status:** Open → Fix, Wontfix, or Later → Fixed → Verified. Later creates a follow-up issue linked to the finding. Wontfix requires a reason.

## 4. Functional requirements

### Issues and plans

- FR-1: Create, edit, and move issues between statuses. Issue numbers are unique and sequential within a project.
- FR-2: Store the plan as markdown in the issue description, with checklists that can be ticked.
- FR-3: Link an issue to a branch name and a pull request URL.
- FR-4: Find an issue from a branch name that contains its key, such as `feat/wp-142-meta-comments`.

### Reviews and findings

- FR-5: Each review round has its own URL, such as `/WP-142/reviews/2`.
- FR-6: Round numbers increase per issue. Finding numbers restart at R1 in each round.
- FR-7: Show findings as separate cards, ordered by severity, each with its own status and thread.
- FR-8: The developer can decide on a finding with one key: `f` fix, `w` wontfix (asks for a reason), `l` later.
- FR-9: Deciding `later` creates a follow-up issue in the backlog, linked to the finding.
- FR-10: Mark a finding as a duplicate of another, across rounds or reviewers.
- FR-11: Show open findings across all rounds of an issue in one list.
- FR-12: Start a later round from the previous round's reviewed commit.

### Identity and history

- FR-13: Every write records its actor. The interface shows the actor's name and an agent badge.
- FR-14: Humans create agent tokens. Each token belongs to one agent actor and can be revoked.
- FR-15: Every change writes an event. Issues and findings show their event history.

### Agent interface

- FR-16: The MCP server exposes these tools: `get_issue_for_branch`, `get_issue`, `get_plan`, `update_plan_checklist`, `start_review_round`, `add_finding`, `finish_review_round`, `list_open_findings`, `set_finding_status`, `add_comment`, `post_fix_summary`, `create_issue`.
- FR-17: Tool inputs are validated. Invalid inputs return a clear error that names the field.
- FR-18: The CLI covers the same operations for scripts and agents without MCP.

### Interface

- FR-19: Views: project board, issue page, review round page, open findings list, activity feed, settings for actors and tokens.
- FR-20: Views update live through the event stream.
- FR-21: Command palette and keyboard shortcuts for navigation and finding decisions.
- FR-22: The desktop app adds a tray icon, native notifications, and a global shortcut to open an issue by key.

### Import and integration

- FR-23: Import projects, milestones, issues, and comments from Linear through its API.
- FR-24: Parse imported review comments in the `## Review round` format into review rounds and findings.
- FR-25: A GitHub webhook closes the linked issue when its pull request merges. The webhook signature is checked.

## 5. Non-functional requirements

- NFR-1 Performance: API reads under 50 ms at the 95th percentile on the staging server. The issue and review pages render in under 200 ms.
- NFR-2 Resources: the server idles under 100 MB of RAM. The desktop app idles under 150 MB.
- NFR-3 Durability: an accepted write is committed to SQLite before the response returns. Litestream replication lag stays under 10 seconds.
- NFR-4 Recovery: restoring from R2 into a fresh server is documented and tested before launch.
- NFR-5 Configuration: every setting comes from environment variables. The server refuses to start when a required setting is missing or malformed. No defaults in code.

## 6. Security

- Humans sign in with email and password (argon2id hashes) and get a session cookie. Agents use bearer tokens.
- Workspace identity comes from the authenticated session or token, never from request input.
- Comments, plans, findings, and imported content are untrusted. Markdown is sanitized before rendering. Agent tools never execute content.
- Roles in version 1: owner, member, agent. Agents cannot create tokens, change roles, or delete projects.
- Rate limits apply per token.
- Backups are encrypted before they leave the server.

## 7. Deployment

- One binary plus Litestream, run as two systemd services or one Docker Compose stack.
- Behind the existing reverse proxy with HTTPS.
- Releases are built by CI as versioned binaries and desktop installers.
- Database migrations run at startup, in order, inside a transaction, and are recorded in a migrations table.
