# Software requirements specification

This document defines the requirements and architecture of the first version of Super Sparrow. [PRODUCT.md](PRODUCT.md) explains what the product is and why it exists.

## 1. Scope

Version 1 covers projects, milestones, issues with plans and lightweight subissues, review rounds, findings, comments, actor identity for humans and agents, an activity history, the MCP server, the CLI, the web and desktop interfaces, a Linear importer, deployment, and backups.

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

The Go server serves the HTTP API, the MCP endpoint, the event stream, and the built frontend. The desktop app opens the server's URL in its window, so it always runs the current frontend and signs in exactly like the browser. Desktop builds require `PUBLIC_URL` and embed that origin; there is no address entry or saved server override. Development reads it from the repository `.env`, and release builds read the deployment's GitHub repository variable `PUBLIC_URL`. Tauri grants the tray, notification, and shortcut features to that URL only.

The frontend runs TanStack Start in SPA mode: it builds to static files that the Go server serves. It uses no server functions or server-side rendering. The Go API is the only backend, and TanStack Query handles data fetching and caching.

```
Browser / Tauri app ──HTTP + SSE──┐
Claude Code / Codex ──MCP─────────┼──> Go server ──> SQLite ──hourly sync + daily backup──> R2
CLI / scripts ───────HTTP─────────┘
```

## 3. Data model

All tables have `id`, `created_at`, and `updated_at`, except events, which never change. IDs are UUIDv7. Super Sparrow serves one team, so there are no workspaces or tenants. Work is divided by project.

The schema lives in `server/migrations/`, and `server/internal/store` is the only code that reads or writes it. CHECK constraints, foreign keys, and triggers enforce the rules below in the database as well as in Go.

- **Actor:** kind (`human`, `agent`, or `system`), display name, disabled at. Humans also have an optional built-in avatar identifier, an email and a role (`owner` or `member`). Agents also have an agent name (`claude-code`, `codex`) and the human who owns them. A person has at most one agent per agent name, and all of their tokens for that tool act as it. One built-in system actor records actions no person or agent took, such as creating the first owner. Every write records its actor. Disabling a person also disables their agents and revokes their sessions and tokens. The team always keeps at least one active owner.
- **Session:** human actor, hashed session ID, device label, last seen, expires at, revoked at.
- **Sign-in code:** email, hashed code, expires at, failed attempts, used at.
- **Token:** agent actor, hashed secret, label, last used, expires at, revoked at. Only the hash is stored.
- **Project:** key (`WP`), name, unique immutable URL slug generated from the name, next issue number. Slug collisions receive numeric suffixes; renaming keeps existing links working.
- **Milestone:** project, name, position. Status is calculated from assigned full issues, never stored or set manually: `planned` for no issues or only Backlog/Todo; `done` when all are Done/Canceled; otherwise `active` when any issue has started or finished. Reopening and reassigning issues immediately affect the calculated status. Subissues do not affect it.
- **Label:** project, name, six-digit hexadecimal color. Names are unique within a project.
- **Issue:** project, number (shown as `WP-142`), title, plan (markdown), plan revision, status, priority (urgent, high, medium, low, or unset), label set, parent issue, milestone, assignee, branch name, pull request URL, created by, completed at. The parent, milestone, and labels must be in the same project, and parent links never form a cycle. A branch name belongs to at most one issue per project. The plan revision goes up with every change to the plan, ticked checklist items included; saving an edited plan names the revision it started from and is refused when the plan has changed since, so no edit silently undoes another.
- **Subissue:** parent issue, number within that issue (shown as `WP-142.1`), title, description (markdown), and progress status using the issue statuses. Subissues have no assignee, priority, labels, milestone, branch, pull request, reviews, findings, comments, or nested subissues. They are stored separately from full issues and do not appear in project issue lists. IDs are immutable; numbering increases per parent and never reuses a number. Description revisions prevent concurrent edits from silently overwriting each other. Creator and timestamps are persistence metadata; every change records its authenticated actor in the parent issue's history in the same transaction. Existing parent links between full issues remain separate from lightweight subissues.
- **Review round:** issue, round number, plan coverage note, summary, author actor, finished at. An issue has at most one round in progress. A finished round takes no new findings.
- **Finding:** round, number within the round (R1, R2), severity, title, problem, fix, evidence, file path, line, status, decision reason, decided by actor, found by actor, fixed by actor, verified by actor, duplicate-of finding, follow-up issue. A duplicate points to a finding on the same issue, and duplicates never chain.
- **Attachment:** project, content type (PNG, JPEG, GIF, or WebP, recognized from the bytes), size up to 10 MiB, the image bytes, created by. Images live in SQLite, so backups include them. Markdown shows an image by its URL; agent tools show only images of the record's own project.
- **Context:** project, number (shown as `WP-C3`), title, body (markdown), body revision, optional linked issue in the same project, created by. A note kept outside the issue tracker: inspiration, research, and screenshots that a person hands to an agent by key. Body revisions refuse stale edits like plan revisions. Only people delete contexts. Context and attachment changes write their events on the project.
- **Comment:** target (issue or finding), body (markdown), author actor, parent comment.
- **Event:** project, target, kind, actor, reported model, payload (JSON), sequence number. Append-only: database triggers refuse updates and deletes. Every change writes its event in the same transaction. Events keep their project as a plain value, so a project's history outlives its deletion. The reported model is whatever the agent says it is; the server cannot verify it.

Issue, round, and finding numbers come from counters on their parent, so a number is never reused.

Reviews and findings do not store commit hashes or reviewed revision ranges. Starting a review or reporting a fix never requires a Git commit.

### Statuses

- **Issue:** Backlog, Todo, In Progress, In Review, Done, Canceled.
- **Finding severity:** Blocking, Should fix, Nit.
- **Finding status:** Open → Fix, Wontfix, or Later. Fix → Fixed → Verified. Only a person decides Fix, Wontfix, or Later, or withdraws a decision back to Open. Agents and people can mark a finding Fixed or Verified, and a fix that did not hold moves from Fixed back to Fix. Later creates one follow-up issue in the backlog, linked to the finding. Wontfix requires a reason.

## 4. Functional requirements

### Issues and plans

- FR-1: Create, edit, and move issues between statuses. Issue numbers are unique and sequential within a project.
- FR-2: Store the plan as markdown in the issue description, with checklists that can be ticked. The inline editor formats Markdown as it is typed and automatically saves with revision checks; unsaved drafts are kept in the browser tab for recovery.
- FR-3: Link an issue to a branch name and a pull request URL.
- FR-4: Find an issue from a branch name that contains its key, such as `feat/wp-142-meta-comments`.
- FR-27: Paste, drop, or capture images into plans, descriptions, and contexts. A screenshot button grabs the screen and lets the person drag out the region to keep; the desktop app captures the monitor natively, and a browser uses screen sharing.
- FR-28: Ctrl+Q opens a movable quick capture panel for a new issue (with status, priority, assignee, milestone, and labels) or a new context. It stays open until the record is saved, its close button is pressed, or Ctrl+Q is pressed again, and keeps its draft while closed.
- FR-29: Keep contexts per project, readable and editable in the web interface and through the agent tools by key.
- FR-26: Create, list, and edit lightweight subissues under an issue. Each exposes only a title, Markdown description, ID, and progress status. Reviews, findings, and discussions stay attached to full issues (including existing finding threads); subissues cannot receive them. Display subissues below the main issue description and in the right sidebar in place of review rounds. Clicking a subissue opens a right-side drawer with inline editing: title saves on blur, status saves immediately, and the Markdown description autosaves with revision checks and recoverable drafts. Creation uses the existing form. Review rounds remain available in their issue tab. Members and agents can manage subissues through the authenticated HTTP API.

### Reviews and findings

- FR-5: Each review round has its own URL, such as `/WP-142/reviews/2`.
- FR-6: Round numbers increase per issue. Finding numbers restart at R1 in each round.
- FR-7: Show findings as separate cards, ordered by severity, each with its own status and thread.
- FR-8: The developer can decide on a finding with one key: `f` fix, `w` wontfix (asks for a reason), `l` later.
- FR-9: Deciding `later` creates a follow-up issue in the backlog, linked to the finding.
- FR-10: Mark a finding as a duplicate of another, across rounds or reviewers.
- FR-11: Show open findings across all rounds of an issue in one list.
- FR-12: Start later review rounds on the same issue, keeping findings and summaries associated with their original round.

The issue page orders its tabs Plan, Review rounds, Findings, Activity, Comments. Review rounds show the active round and finished reviews with their reviewer, coverage or summary, and finding counts. Selecting a round opens a side drawer with its coverage, summary, and severity-ordered findings. A finding opens in the same drawer with navigation back to its round, evidence, actor attribution, valid lifecycle actions, and discussion. The Findings tab shows unresolved findings across rounds, reveals resolved findings only through “Show N resolved findings”, and keeps Add finding in a sticky bottom action bar; decisions are available inside the drawer, not in list rows. Selected rounds, findings, and resolved-finding visibility live in the URL. People can start and finish rounds and add findings to an active round from these views. Finished rounds take no new findings, but their findings remain actionable. Agent names in these views are shown as Claude or Codex while actor IDs retain individual attribution.

### Identity and history

- FR-13: Every write records its actor. The interface shows the actor's name and an agent badge.
- FR-14: Each person creates their own agent tokens. A token belongs to one agent actor owned by that person, expires after 90 days, and can be renewed or revoked.
- FR-15: Every change writes an event. Issues and findings show their event history.

### Agent interface

- FR-16: The MCP server exposes these tools: `get_issue_for_branch`, `get_issue`, `get_plan`, `update_plan_checklist`, `start_review_round`, `add_finding`, `finish_review_round`, `list_open_findings`, `set_finding_status`, `mark_duplicate`, `add_comment`, `list_comments`, `post_fix_summary`, `create_issue`, `list_projects`, `list_issues`, `update_plan`, `list_subissues`, `create_subissue`, `update_subissue`, `list_review_rounds`, `get_review_round`, `get_finding`, `list_labels`, `create_label`, `update_label`, `set_labels`, `list_contexts`, `get_context`, `create_context`, `update_context`, and `get_issue_images`. Findings include a bounded discussion page unless explicitly omitted. List tools support cursors, writes accept actor-scoped retry keys, and plan/description edits require revisions. [MCP.md](MCP.md) defines the complete tool behavior.
- FR-17: Tool inputs are validated. Invalid inputs return a clear error that names the field. Tool results include structured JSON, and failures include stable error codes.
- FR-18: The CLI covers the same operations for scripts and agents without a configured MCP connection, including generic `tool list` and `tool call` commands.

Successful write results and optional retry receipts are stored with their events in the same SQLite transaction. Retry keys are scoped to the authenticated agent and refuse changed requests.

The MCP endpoint uses the streamable HTTP transport without sessions: each request is answered with JSON, and the server sends nothing unprompted.

### Interface

- FR-19: Views: project issue list, issue page, review round page, activity feed, settings for actors and tokens.
- FR-20: Views update live through the event stream.
- FR-21: Command palette and keyboard shortcuts for navigation and finding decisions.
- FR-22: The desktop app adds a tray icon, native notifications, and a global shortcut to open an issue by key. It starts at login and stays in the tray, and the system-wide Ctrl+Q opens an always-on-top quick capture window over any application.

### Import and integration

- FR-23: Import projects, milestones, issues, and comments from Linear through its API.
- FR-24: Parse imported review comments in the `## Review round` format into review rounds and findings. Imported issues keep their Linear numbers. Imported records are written by the system actor, each comment notes its original author and time, and an imported `later` finding creates no follow-up issue. Running an import again brings in only what is new.
- FR-25: A GitHub webhook closes the linked issue when its pull request merges. The webhook signature is checked.

## 5. Non-functional requirements

- NFR-1 Performance: API reads under 50 ms at the 95th percentile on the production server. The issue and review pages render in under 200 ms.
- NFR-2 Resources: the server idles under 100 MB of RAM. The desktop app idles under 150 MB.
- NFR-3 Durability: an accepted write is committed to SQLite before the response returns. A server loss loses at most one hour of data.
- NFR-4 Recovery: restoring from the latest sync and from any daily backup into a fresh server is documented and tested before launch.
- NFR-5 Configuration: every setting comes from environment variables. The server refuses to start when a required setting is missing or malformed. No defaults in code.

## 6. Authentication and security

Super Sparrow serves one team. There is no public signup and no password.

### People

- The first owner is created with a server command. After that, an owner adds a teammate by name and email, and Resend sends them an invitation email.
- Sign-in is by email only. A person enters their email, Resend sends a 6-digit code, and they enter it on the same screen. A code works in the browser and in the desktop app, where a link would open the wrong window.
- A code expires after 10 minutes, works once, and is stored only as a hash keyed with a server secret that is kept out of the database, so a copy of the database does not reveal codes. Five wrong attempts cancel it.
- An email's three newest codes work at once, so someone else requesting codes for that email cannot cancel the one its owner is entering. A wrong guess counts against all of them, and signing in cancels the rest.
- The sign-in screen shows the same message whether or not the email belongs to a teammate, so it never reveals who has an account. Codes are sent only to active teammates.
- Code requests are rate-limited per email and per IP address, and wrong codes per IP address, so nobody can keep cancelling a teammate's codes from one address. Right codes do not count, so a team signing in from one office address does not use up its tries.
- A successful sign-in creates a session stored in the database. The browser gets an `HttpOnly`, `SameSite=Lax` cookie. When the public URL uses HTTPS, the cookie is also `Secure` and named with the `__Host-` prefix; over `http://localhost` in development it is not, so sign-in works there. A session lasts 30 days and renews while in use.
- Settings lists each person's signed-in devices, with sign-out for one device or all of them.
- Disabling a teammate ends all their sessions and revokes all their agent tokens at once.
- Signing in and out and creating, renewing, or revoking a token write events on the person or agent. Sign-in codes, session renewals, and token use are credential bookkeeping and write none.
- The Resend API key, the sender address, and the server's public URL are required settings.

### Agents

- Each person creates their own agent tokens in settings, one per agent and machine, such as "Shanto · Claude Code · laptop". The token is shown once.
- Tokens start with `ssp_` so secret scanners can recognize them. Only a SHA-256 hash is stored.
- A token expires after 90 days. Settings shows its expiry and last use, and the person can renew or revoke it.
- Claude Code, Codex, and the CLI send the token as a bearer token on the MCP endpoint and the API. The token creation form offers a secret-free prompt for permanent, user-scoped client setup using an OS credential store and a local HTTP credential helper. The token is supplied through private local input, never in agent chat, setup prompts, or committed configuration. The CLI reads `SPARROW_TOKEN` from its execution environment.
- Every action through a token is recorded as that agent, owned by that person. An agent's name is its tool, such as "Claude Code"; its owner is stored and shown separately, so the interface and the agent tools can show "Claude Code" with "Shanto" as its owner.
- An agent may name the model it runs in a `Sparrow-Model` header, which events record as reported.
- Agents read project history, but not events about people, sessions, or tokens, which hold email addresses and device names.

### Roles and limits

- Owners add and disable teammates, delete projects, and can revoke anyone's tokens.
- Members do all day-to-day work and manage their own tokens.
- Agents can read and write issues, plans, reviews, findings, and comments. They cannot create tokens, manage people, or delete projects.
- Actor identity comes from the session or token, never from request input.
- Rate limits apply per token.

### Other

- Comments, plans, findings, and imported content are untrusted. Markdown is sanitized before rendering. Agent tools never execute content.
- The app's page is served with a Content-Security-Policy that allows only its own files and, by hash, the inline scripts its build writes, so content that slips past sanitizing cannot run scripts.
- Backups are encrypted before they leave the server.

## 7. Backups

The Go server runs two jobs that write to Cloudflare R2. Both take a consistent copy of the database with SQLite's `VACUUM INTO`, never by copying the live file, then compress it, encrypt it with [age](https://age-encryption.org) to the team's public key, and upload it.

| Job          | When       | What it keeps in R2                                       | Protects against                                   |
| ------------ | ---------- | --------------------------------------------------------- | -------------------------------------------------- |
| Sync         | Every hour | One copy, replaced each hour                              | Losing the server: at most one hour of data lost   |
| Daily backup | Once a day | A dated full copy, kept for the configured number of days | Mistakes the sync would copy, such as a bad delete |

- The sync replaces its copy only after the new upload succeeds, so a failed run never leaves R2 without a copy.
- Both jobs record each run (start, finish, size, result) and log a failure.
- `APP_ENV=production` requires the public key, R2 credentials, bucket, the daily backup time, and the number of days to keep, so a production server never runs without backups. In development they are optional, all or none, so a development machine runs without R2. The server holds only the public key and cannot decrypt its own backups; the matching private key, which the team keeps outside the server, decrypts them. Production keeps daily backups for 30 days.
- A `restore` CLI command downloads a chosen copy, decrypts it with the private key, checks it with `PRAGMA integrity_check`, and writes it as the database file.

## 8. Deployment

Super Sparrow production runs on the host reached over SSH as `wpstaging`. That host is staging
for WP Agent and production for Super Sparrow; the applications have independent environments.

- One binary, run as a systemd service or one Docker Compose service.
- Behind the existing reverse proxy with HTTPS.
- Doppler holds production configuration and a separate recovery project holds the backup private key, outside CI and the VPS.
- Successful GitHub Actions checks on `main` publish an immutable GHCR image and deploy it over verified SSH. Settings-only updates are available through a manual workflow.
- GitHub Actions builds versioned server binaries and the desktop installers for Linux, macOS, and Windows, and publishes them to GitHub Releases when a version tag is pushed.
- The repository is private, so only team members with repository access can download the installers.
- Database migrations run at startup, in order, inside a transaction, and are recorded in a migrations table.
