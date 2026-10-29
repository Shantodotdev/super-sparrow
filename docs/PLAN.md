# Plan

Steps to build the first version of Super Sparrow, in order. Requirements are in [SRS.md](SRS.md). Start using the tool for its own development as soon as the agent tools work.

## 1. Foundation

Done in the initial scaffold.

1. Create the `server/` (Go), `web/` (TanStack Start), and `desktop/` (Tauri) folders.
2. Add a Makefile with `check-go`, `test-go`, `check-web`, `test-web`, `format`, and `dev` targets, as AGENTS.md describes.
3. Set up CI to run those checks and tests.
4. Write the configuration loader. It reads environment variables and stops the server when a required one is missing or malformed.
5. Set up SQLite in WAL mode with an ordered migration runner.
6. Add structured logging and a health check endpoint.

## 2. Data model

Done. The schema is in `server/migrations/`; the data access layer is `server/internal/store`.

1. Write migrations for actors, tokens, projects, milestones, and issues.
2. Write migrations for review rounds, findings, comments, and events.
3. Build the data access layer.
4. Write each change and its event in the same transaction.
5. Add tests for issue numbering, round and finding numbering, and status transitions.

## 3. Authentication and identity

Done. Secrets and emails are in `server/internal/auth` and `server/internal/email`; the endpoints are in `server/internal/httpserver`.

1. Add the Resend email client, with the API key and sender address as required settings.
2. Add email sign-in codes: request, send, verify, expire after 10 minutes, cancel after five wrong attempts, and store only hashes.
3. Add rate limits on code requests per email and per IP address.
4. Add database sessions with secure cookies, 30-day renewal, and sign-out for one device or all.
5. Add a command to create the first owner.
6. Add teammate invitations by email, and disabling a teammate, which ends their sessions and revokes their tokens.
7. Add agent tokens with the `ssp_` prefix: create, list, renew, revoke, 90-day expiry, and SHA-256 hashes only.
8. Add middleware that resolves the actor from the session or token.
9. Enforce the owner, member, and agent permissions.
10. Add per-token rate limits.

## 4. HTTP API

1. Add endpoints for projects, milestones, and issues.
2. Add the branch-to-issue lookup.
3. Add endpoints for review rounds and findings, including decisions and duplicates.
4. Create the follow-up issue when a finding is decided `later`.
5. Add endpoints for comments and the event history.
6. Validate every request body and return errors that name the field.
7. Add the Server-Sent Events stream for live updates.
8. Add API tests for every endpoint, including tests that agents are refused owner-only actions.

## 5. Agent interface

1. Add the MCP server over streamable HTTP, using agent tokens.
2. Implement the MCP tools listed in SRS FR-16.
3. Write each tool's description so an agent can use it without extra instructions.
4. Build the CLI on the HTTP API, covering the same operations.
5. Connect Claude Code and Codex to the server and run one full plan, review, and fix loop.
6. Write the new `review`, `address-review`, and `plan` skills to use the MCP tools.
7. Move this project's own issues and reviews into the tool.

## 6. Web interface

1. Set up TanStack Start (React) in SPA mode with TanStack Router, TanStack Query, the API client, and the event stream.
2. Build the email sign-in screen: enter email, then the 6-digit code.
3. Build the review round page with finding cards, threads, and keyboard decisions.
4. Build the issue page with the plan, checklist, review rounds, open findings, and history.
5. Build the project issue list, grouped by status and filtered by milestone.
6. Build the activity feed, with actor names and agent badges.
7. Build settings for teammates, signed-in devices, and agent tokens.
8. Add the command palette and keyboard shortcuts.
9. Sanitize all rendered markdown.
10. Serve the built frontend from the Go server.

## 7. Desktop app

1. Create the Tauri v2 app that opens the server's URL in its window.
2. Add first-run setup for the server address, and grant native features to that address only.
3. Add the tray icon and native notifications for new findings and decisions.
4. Add the global shortcut to open an issue by key.
5. Add a GitHub Actions workflow that builds the installers for Linux, macOS, and Windows and publishes them to GitHub Releases when a version tag is pushed.

## 8. Integrations

1. Build the Linear importer for projects, milestones, issues, and comments.
2. Parse `## Review round` comments into review rounds and findings.
3. Run the import for WP Agent's Version 01 project and check the result against Linear.
4. Add the GitHub webhook that closes an issue when its pull request merges, with signature checks.

## 9. Deployment and backups

1. Write the systemd unit or Docker Compose file for the server.
2. Build the backup jobs: a consistent copy with `VACUUM INTO`, compression, age encryption, and upload to R2.
3. Schedule the hourly sync, which replaces its copy only after a successful upload.
4. Schedule the daily backup, with dated copies and deletion of copies older than the configured number of days.
5. Record each backup run and show the last successful sync and backup in settings.
6. Add the `restore` CLI command, with an integrity check before it writes the database file.
7. Deploy to the staging server behind the reverse proxy with HTTPS.
8. Restore both the latest sync and a daily backup into a fresh machine, and document the steps.
9. Measure memory and response times against the SRS limits.

## 10. Switch over

1. Update WP Agent's `AGENTS.md`, `docs/REVIEW.md`, and its `plan`, `review`, and `address-review` skills to use the tool.
2. Invite each teammate, and have each one create tokens for their Claude Code and Codex.
3. Commit the shared MCP configuration, which reads `SPARROW_TOKEN`, to each repository that uses Super Sparrow.
4. Install the desktop app for the team from GitHub Releases.
5. Run new features through the tool and keep Linear read-only for history.
6. Collect what slows the team down and plan the next version from it.
