# Plan

Steps to build the first version of Tracker, in order. Requirements are in [SRS.md](SRS.md). Start using the tool for its own development as soon as the agent tools work.

## 1. Foundation

1. Create the repository with `server/` (Go), `web/` (frontend), and `desktop/` (Tauri).
2. Add an `AGENTS.md` with the coding rules, checks, and the review workflow.
3. Set up CI to run lint, type checks, and tests for Go and the frontend.
4. Write the configuration loader. It reads environment variables and stops the server when a required one is missing or malformed.
5. Set up SQLite in WAL mode with an ordered migration runner.
6. Add structured logging and a health check endpoint.

## 2. Data model

1. Write migrations for workspaces, actors, tokens, projects, milestones, and issues.
2. Write migrations for review rounds, findings, comments, and events.
3. Build the data access layer, with `workspace_id` required on every query for workspace-owned records.
4. Write each change and its event in the same transaction.
5. Add tests for issue numbering, round and finding numbering, and status transitions.

## 3. Authentication and identity

1. Add human sign-in with email and password, argon2id hashing, and session cookies.
2. Add agent tokens: create, list, revoke, stored as hashes only.
3. Add middleware that resolves the actor and workspace from the session or token.
4. Enforce the owner, member, and agent roles.
5. Add per-token rate limits.
6. Add a command to create the first workspace and owner.

## 4. HTTP API

1. Add endpoints for projects, milestones, and issues.
2. Add the branch-to-issue lookup.
3. Add endpoints for review rounds and findings, including decisions and duplicates.
4. Create the follow-up issue when a finding is decided `later`.
5. Add endpoints for comments and the event history.
6. Validate every request body and return errors that name the field.
7. Add the Server-Sent Events stream for live updates.
8. Add API tests for every endpoint, including tests that one workspace can't read another's data.

## 5. Agent interface

1. Add the MCP server over streamable HTTP, using agent tokens.
2. Implement the MCP tools listed in SRS FR-16.
3. Write each tool's description so an agent can use it without extra instructions.
4. Build the CLI on the HTTP API, covering the same operations.
5. Connect Claude Code and Codex to the server and run one full plan, review, and fix loop.
6. Write the new `review`, `address-review`, and `plan` skills to use the MCP tools.
7. Move this project's own issues and reviews into the tool.

## 6. Web interface

1. Set up the frontend with routing, the API client, and the event stream.
2. Build sign-in.
3. Build the review round page with finding cards, threads, and keyboard decisions.
4. Build the issue page with the plan, checklist, review rounds, open findings, and history.
5. Build the project board, grouped by status and filtered by milestone.
6. Build the activity feed, with actor names and agent badges.
7. Build settings for actors and agent tokens.
8. Add the command palette and keyboard shortcuts.
9. Sanitize all rendered markdown.
10. Serve the built frontend from the Go server.

## 7. Desktop app

1. Wrap the frontend build in Tauri v2.
2. Add server address setup and sign-in.
3. Add the tray icon and native notifications for new findings and decisions.
4. Add the global shortcut to open an issue by key.
5. Build installers for Linux, macOS, and Windows in CI.

## 8. Integrations

1. Build the Linear importer for projects, milestones, issues, and comments.
2. Parse `## Review round` comments into review rounds and findings.
3. Run the import for WP Agent's Version 01 project and check the result against Linear.
4. Add the GitHub webhook that closes an issue when its pull request merges, with signature checks.

## 9. Deployment and backups

1. Write the systemd units or Docker Compose file for the server and Litestream.
2. Configure Litestream to replicate the encrypted database to R2.
3. Deploy to the staging server behind the reverse proxy with HTTPS.
4. Restore from R2 into a fresh machine and document the steps.
5. Measure memory and response times against the SRS limits.

## 10. Switch over

1. Update WP Agent's `AGENTS.md`, `docs/REVIEW.md`, and its `plan`, `review`, and `address-review` skills to use the tool.
2. Create agent tokens for each teammate's Claude Code and Codex.
3. Install the desktop app for the team.
4. Run new features through the tool and keep Linear read-only for history.
5. Collect what slows the team down and plan the next version from it.
