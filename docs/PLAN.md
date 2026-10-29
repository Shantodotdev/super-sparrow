# Plan

Steps to build the first version of Super Sparrow, in order. Requirements are in [SRS.md](SRS.md). Start using the tool for its own development as soon as the agent tools work.

## 1. Foundation

Done in the initial scaffold.

- [x] Create the `server/` (Go), `web/` (TanStack Start), and `desktop/` (Tauri) folders.
- [x] Add a Makefile with `check-go`, `test-go`, `check-web`, `test-web`, `format`, and `dev` targets, as AGENTS.md describes.
- [x] Set up CI to run those checks and tests.
- [x] Write the configuration loader. It reads environment variables and stops the server when a required one is missing or malformed.
- [x] Set up SQLite in WAL mode with an ordered migration runner.
- [x] Add structured logging and a health check endpoint.

## 2. Data model

Done. The schema is in `server/migrations/`; the data access layer is `server/internal/store`.

- [x] Write migrations for actors, tokens, projects, milestones, and issues.
- [x] Write migrations for review rounds, findings, comments, and events.
- [x] Build the data access layer.
- [x] Write each change and its event in the same transaction.
- [x] Add tests for issue numbering, round and finding numbering, and status transitions.

## 3. Authentication and identity

Done. Secrets and emails are in `server/internal/auth` and `server/internal/email`; the endpoints are in `server/internal/httpserver`.

- [x] Add the Resend email client, with the API key and sender address as required settings.
- [x] Add email sign-in codes: request, send, verify, expire after 10 minutes, cancel after five wrong attempts, and store only hashes.
- [x] Add rate limits on code requests per email and per IP address.
- [x] Add database sessions with secure cookies, 30-day renewal, and sign-out for one device or all.
- [x] Add a command to create the first owner.
- [x] Add teammate invitations by email, and disabling a teammate, which ends their sessions and revokes their tokens.
- [x] Add agent tokens with the `ssp_` prefix: create, list, renew, revoke, 90-day expiry, and SHA-256 hashes only.
- [x] Add middleware that resolves the actor from the session or token.
- [x] Enforce the owner, member, and agent permissions.
- [x] Add per-token rate limits.

## 4. HTTP API

Done. The endpoints are in `server/internal/httpserver` and listed in [API.md](API.md).

- [x] Add endpoints for projects, milestones, and issues.
- [x] Add the branch-to-issue lookup.
- [x] Add endpoints for review rounds and findings, including decisions and duplicates.
- [x] Create the follow-up issue when a finding is decided `later`.
- [x] Add endpoints for comments and the event history.
- [x] Validate every request body and return errors that name the field.
- [x] Add the Server-Sent Events stream for live updates.
- [x] Add API tests for every endpoint, including tests that agents are refused owner-only actions.

## 5. Agent interface

The MCP server is in `server/internal/mcp` and the CLI in `server/internal/cli`. The last three items need the team's agents and repositories.

- [x] Add the MCP server over streamable HTTP, using agent tokens.
- [x] Implement the MCP tools listed in SRS FR-16.
- [x] Write each tool's description so an agent can use it without extra instructions.
- [x] Build the CLI on the HTTP API, covering the same operations.
- [ ] Connect Claude Code and Codex to the server and run one full plan, review, and fix loop.
- [ ] Write the new `review`, `address-review`, and `plan` skills to use the MCP tools.
- [ ] Move this project's own issues and reviews into the tool.

## 6. Web interface

- [ ] Set up TanStack Start (React) in SPA mode with TanStack Router, TanStack Query, the API client, and the event stream.
- [ ] Build the email sign-in screen: enter email, then the 6-digit code.
- [ ] Build the review round page with finding cards, threads, and keyboard decisions.
- [ ] Build the issue page with the plan, checklist, review rounds, open findings, and history.
- [ ] Build the project issue list, grouped by status and filtered by milestone.
- [ ] Build the activity feed, with actor names and agent badges.
- [ ] Build settings for teammates, signed-in devices, and agent tokens.
- [ ] Add the command palette and keyboard shortcuts.
- [ ] Sanitize all rendered markdown.
- [x] Serve the built frontend from the Go server. It serves the build in `SPARROW_WEB_DIR`.

## 7. Desktop app

- [ ] Create the Tauri v2 app that opens the server's URL in its window.
- [ ] Add first-run setup for the server address, and grant native features to that address only.
- [ ] Add the tray icon and native notifications for new findings and decisions.
- [ ] Add the global shortcut to open an issue by key.
- [x] Add a GitHub Actions workflow that builds the installers for Linux, macOS, and Windows and publishes them to GitHub Releases when a version tag is pushed.

## 8. Integrations

The importer is in `server/internal/linear` and the webhook in `server/internal/httpserver`. The import run needs a Linear API key and the real project.

- [x] Build the Linear importer for projects, milestones, issues, and comments.
- [x] Parse `## Review round` comments into review rounds and findings.
- [ ] Run the import for WP Agent's Version 01 project and check the result against Linear.
- [x] Add the GitHub webhook that closes an issue when its pull request merges, with signature checks.

## 9. Deployment and backups

`deploy/` holds the Docker and systemd files, `server/internal/backup` the jobs and restore, and [DEPLOYMENT.md](DEPLOYMENT.md) the steps. The last three items need the staging server and an R2 bucket.

- [x] Write the systemd unit or Docker Compose file for the server.
- [x] Build the backup jobs: a consistent copy with `VACUUM INTO`, compression, age encryption, and upload to R2.
- [x] Schedule the hourly sync, which replaces its copy only after a successful upload.
- [x] Schedule the daily backup, with dated copies and deletion of copies older than the configured number of days.
- [ ] Record each backup run and show the last successful sync and backup in settings. Recording and `GET /api/backups` are done; the settings page is in step 6.
- [x] Add the `restore` CLI command, with an integrity check before it writes the database file.
- [ ] Deploy to the staging server behind the reverse proxy with HTTPS.
- [ ] Restore both the latest sync and a daily backup into a fresh machine, and document the steps. The steps are in [DEPLOYMENT.md](DEPLOYMENT.md#restoring); the restore on a fresh machine is not done yet.
- [ ] Measure memory and response times against the SRS limits.

## 10. Switch over

- [ ] Update WP Agent's `AGENTS.md`, `docs/REVIEW.md`, and its `plan`, `review`, and `address-review` skills to use the tool.
- [ ] Invite each teammate, and have each one create tokens for their Claude Code and Codex.
- [ ] Commit the shared MCP configuration, which reads `SPARROW_TOKEN`, to each repository that uses Super Sparrow.
- [ ] Install the desktop app for the team from GitHub Releases.
- [ ] Run new features through the tool and keep Linear read-only for history.
- [ ] Collect what slows the team down and plan the next version from it.
