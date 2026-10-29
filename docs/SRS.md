# Software requirements specification

This document defines the requirements and architecture of the first version of Super Sparrow. [PRODUCT.md](PRODUCT.md) explains what the product is and why it exists.

## 1. Scope

Version 1 covers projects, milestones, issues with plans, review rounds, findings, comments, actor identity for humans and agents, an activity history, the MCP server, the CLI, the web and desktop interfaces, a Linear importer, deployment, and backups.

Out of scope for version 1: documents, secrets, CI/CD, cycles, estimates, roadmaps, custom fields, email notifications, fine-grained permissions, and GitHub sync beyond closing an issue when its pull request merges.

## 2. Architecture

| Part       | Choice                                         | Why                                                              |
| ---------- | ---------------------------------------------- | ---------------------------------------------------------------- |
| Server     | Go, one binary                                 | Low memory, fast builds, simple deployment, agents write it well |
| Database   | SQLite in WAL mode                             | No separate database process; enough for a small team            |
| Backups    | Built into the Go server, encrypted, to R2     | Hourly sync and daily full backups, no extra process             |
| Agent API  | MCP server (streamable HTTP) inside the binary | Claude Code and Codex connect with no glue code                  |
| CLI        | Go, same repository, calls the HTTP API        | For scripts, hooks, and agents without MCP                       |
| Frontend   | TanStack Start (React) in SPA mode             | Type-safe routing and data loading; builds to static files       |
| Live views | Server-Sent Events                             | One-way updates are all the interface needs                      |
| Desktop    | Tauri v2 window that loads the server's page   | Native app with low resource use; never out of date              |

The Go server serves the HTTP API, the MCP endpoint, the event stream, and the built frontend. The desktop app opens the server's URL in its window, so it always runs the current frontend and signs in exactly like the browser. Tauri grants the tray, notification, and shortcut features to that URL only.

The frontend runs TanStack Start in SPA mode: it builds to static files that the Go server serves. It uses no server functions or server-side rendering. The Go API is the only backend, and TanStack Query handles data fetching and caching.

```
Browser / Tauri app ──HTTP + SSE──┐
Claude Code / Codex ──MCP─────────┼──> Go server ──> SQLite ──hourly sync + daily backup──> R2
CLI / scripts ───────HTTP─────────┘
```

## 3. Data model

All tables have `id`, `created_at`, and `updated_at`, except events, which never change. IDs are UUIDv7. Super Sparrow serves one team, so there are no workspaces or tenants. Work is divided by project.

The schema lives in `server/migrations/`, and `server/internal/store` is the only code that reads or writes it. CHECK constraints, foreign keys, and triggers enforce the rules below in the database as well as in Go.

- **Actor:** kind (`human`, `agent`, or `system`), display name, disabled at. Humans also have an email and a role (`owner` or `member`). Agents also have an agent name (`claude-code`, `codex`) and the human who owns them. A person has at most one agent per agent name, and all of their tokens for that tool act as it. One built-in system actor records actions no person or agent took, such as creating the first owner. Every write records its actor. Disabling a person also disables their agents and revokes their sessions and tokens. The team always keeps at least one active owner.
- **Session:** human actor, hashed session ID, device label, last seen, expires at, revoked at.
- **Sign-in code:** email, hashed code, expires at, failed attempts, used at.
- **Token:** agent actor, hashed secret, label, last used, expires at, revoked at. Only the hash is stored.
- **Project:** key (`WP`), name, next issue number.
- **Milestone:** project, name, position, status (`planned`, `active`, `done`).
- **Label:** project, name. Names are unique within a project.
- **Issue:** project, number (shown as `WP-142`), title, plan (markdown), status, label set, parent issue, milestone, assignee, branch name, pull request URL, created by, completed at. The parent, milestone, and labels must be in the same project, and parent links never form a cycle. A branch name belongs to at most one issue per project.
- **Review round:** issue, round number, reviewed range (from commit, to commit), whether uncommitted changes were included, plan coverage note, summary, author actor, finished at. An issue has at most one round in progress. A round without a from commit starts at the previous round's to commit. A finished round takes no new findings.
- **Finding:** round, number within the round (R1, R2), severity, title, problem, fix, evidence, file path, line, commit, status, decision reason, decided by actor, found by actor, fixed by actor, fixed in commit, verified by actor, duplicate-of finding, follow-up issue. A duplicate points to a finding on the same issue, and duplicates never chain.
- **Comment:** target (issue or finding), body (markdown), author actor, parent comment.
- **Event:** project, target, kind, actor, reported model, payload (JSON), sequence number. Append-only: database triggers refuse updates and deletes. Every change writes its event in the same transaction. Events keep their project as a plain value, so a project's history outlives its deletion. The reported model is whatever the agent says it is; the server cannot verify it.

Issue, round, and finding numbers come from counters on their parent, so a number is never reused.

### Statuses

- **Issue:** Backlog, Todo, In Progress, In Review, Done, Canceled.
- **Finding severity:** Blocking, Should fix, Nit.
- **Finding status:** Open → Fix, Wontfix, or Later. Fix → Fixed → Verified. Only a person decides Fix, Wontfix, or Later, or withdraws a decision back to Open. Agents and people can mark a finding Fixed or Verified, and a fix that did not hold moves from Fixed back to Fix. Later creates one follow-up issue in the backlog, linked to the finding. Wontfix requires a reason.

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
- FR-14: Each person creates their own agent tokens. A token belongs to one agent actor owned by that person, expires after 90 days, and can be renewed or revoked.
- FR-15: Every change writes an event. Issues and findings show their event history.

### Agent interface

- FR-16: The MCP server exposes these tools: `get_issue_for_branch`, `get_issue`, `get_plan`, `update_plan_checklist`, `start_review_round`, `add_finding`, `finish_review_round`, `list_open_findings`, `set_finding_status`, `add_comment`, `post_fix_summary`, `create_issue`.
- FR-17: Tool inputs are validated. Invalid inputs return a clear error that names the field.
- FR-18: The CLI covers the same operations for scripts and agents without MCP.

### Interface

- FR-19: Views: project issue list, issue page, review round page, open findings list, activity feed, settings for actors and tokens.
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
- NFR-3 Durability: an accepted write is committed to SQLite before the response returns. A server loss loses at most one hour of data.
- NFR-4 Recovery: restoring from the latest sync and from any daily backup into a fresh server is documented and tested before launch.
- NFR-5 Configuration: every setting comes from environment variables. The server refuses to start when a required setting is missing or malformed. No defaults in code.

## 6. Authentication and security

Super Sparrow serves one team. There is no public signup and no password.

### People

- The first owner is created with a server command. After that, an owner adds a teammate by name and email, and Resend sends them an invitation email.
- Sign-in is by email only. A person enters their email, Resend sends a 6-digit code, and they enter it on the same screen. A code works in the browser and in the desktop app, where a link would open the wrong window.
- A code expires after 10 minutes, works once, and is stored only as a hash. Five wrong attempts cancel it.
- An email's three newest codes work at once, so someone else requesting codes for that email cannot cancel the one its owner is entering. A wrong guess counts against all of them, and signing in cancels the rest.
- The sign-in screen shows the same message whether or not the email belongs to a teammate, so it never reveals who has an account. Codes are sent only to active teammates.
- Code requests are rate-limited per email and per IP address.
- A successful sign-in creates a session stored in the database. The browser gets an `HttpOnly`, `SameSite=Lax` cookie. When the public URL uses HTTPS, the cookie is also `Secure` and named with the `__Host-` prefix; over `http://localhost` in development it is not, so sign-in works there. A session lasts 30 days and renews while in use.
- Settings lists each person's signed-in devices, with sign-out for one device or all of them.
- Disabling a teammate ends all their sessions and revokes all their agent tokens at once.
- Signing in and out and creating, renewing, or revoking a token write events on the person or agent. Sign-in codes, session renewals, and token use are credential bookkeeping and write none.
- The Resend API key, the sender address, and the server's public URL are required settings.

### Agents

- Each person creates their own agent tokens in settings, one per agent and machine, such as "Shanto · Claude Code · laptop". The token is shown once.
- Tokens start with `ssp_` so secret scanners can recognize them. Only a SHA-256 hash is stored.
- A token expires after 90 days. Settings shows its expiry and last use, and the person can renew or revoke it.
- Claude Code, Codex, and the CLI send the token as a bearer token on the MCP endpoint and the API. A repository commits one MCP configuration that reads the token from the `SPARROW_TOKEN` environment variable, so no token is ever committed.
- Every action through a token is recorded as that agent, owned by that person, such as "Claude Code (Shanto)".

### Roles and limits

- Owners add and disable teammates, delete projects, and can revoke anyone's tokens.
- Members do all day-to-day work and manage their own tokens.
- Agents can read and write issues, plans, reviews, findings, and comments. They cannot create tokens, manage people, or delete projects.
- Actor identity comes from the session or token, never from request input.
- Rate limits apply per token.

### Other

- Comments, plans, findings, and imported content are untrusted. Markdown is sanitized before rendering. Agent tools never execute content.
- Backups are encrypted before they leave the server.

## 7. Backups

The Go server runs two jobs that write to Cloudflare R2. Both take a consistent copy of the database with SQLite's `VACUUM INTO`, never by copying the live file, then compress it, encrypt it with [age](https://age-encryption.org), and upload it.

| Job          | When       | What it keeps in R2                                       | Protects against                                   |
| ------------ | ---------- | --------------------------------------------------------- | -------------------------------------------------- |
| Sync         | Every hour | One copy, replaced each hour                              | Losing the server: at most one hour of data lost   |
| Daily backup | Once a day | A dated full copy, kept for the configured number of days | Mistakes the sync would copy, such as a bad delete |

- The sync replaces its copy only after the new upload succeeds, so a failed run never leaves R2 without a copy.
- Both jobs record each run (start, finish, size, result) and log a failure. The settings page shows the last successful run of each.
- The encryption public key, R2 credentials, bucket, the daily backup time, and the number of days to keep are required settings. The private key that decrypts backups is never stored on the server. Staging keeps daily backups for 30 days.
- A `restore` CLI command downloads a chosen copy, decrypts it with the private key, checks it with `PRAGMA integrity_check`, and writes it as the database file.

## 8. Deployment

- One binary, run as a systemd service or one Docker Compose service.
- Behind the existing reverse proxy with HTTPS.
- GitHub Actions builds versioned server binaries and the desktop installers for Linux, macOS, and Windows, and publishes them to GitHub Releases when a version tag is pushed.
- The repository is private, so only team members with repository access can download the installers.
- Database migrations run at startup, in order, inside a transaction, and are recorded in a migrations table.
