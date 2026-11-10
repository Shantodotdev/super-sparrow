# Getting started

## Toolchain

Each version is pinned in the repository, so everyone builds with the same one.

| Tool | Version | Pinned in                                                      |
| ---- | ------- | -------------------------------------------------------------- |
| Go   | 1.27.3  | `server/go.mod` (an older Go downloads this one automatically) |
| Bun  | 1.4.4   | `package.json` (`packageManager`)                              |
| Rust | 1.99.1  | `desktop/src-tauri/rust-toolchain.toml` (rustup installs it)   |

Node is not needed. Bun runs the scripts, the tests, and Vite.

The desktop app also needs the system libraries Tauri builds against. On Ubuntu or Debian:

`sudo apt-get install libwebkit2gtk-4.1-dev librsvg2-dev libxdo-dev libssl-dev`

macOS needs the Xcode command line tools. Windows needs the Microsoft C++ build tools and WebView2, which Windows 11 includes.

## First run

1. `cp .env.example .env`, which sets `APP_ENV=development`. The server refuses to start when a setting it needs is missing and names each one. Development leaves `TRUSTED_PROXIES`, `WEB_DIR`, `GITHUB_WEBHOOK_SECRET`, and the backup settings optional, and turns off each feature whose settings are absent; the backup settings go all or none. Production requires every setting; [DEPLOYMENT.md](DEPLOYMENT.md) covers them.
2. Set `RESEND_API_KEY` to a [Resend](https://resend.com/api-keys) API key. Sign-in codes and invitations are sent through Resend, so nobody can sign in without it. Until your domain is verified in Resend, keep the `onboarding@resend.dev` sender, which only delivers to your own Resend account's address.
3. Set `SIGN_IN_CODE_KEY` to the output of `openssl rand -hex 32`.
4. `make install` installs the Bun workspace and downloads the Go modules.
5. `make create-owner NAME="Your Name" EMAIL=you@example.com` adds the first owner. It works only on a database with nobody in it; after that, owners invite teammates.
6. `make dev` builds and runs the Go server and the web dev server together. The web dev server forwards `/api` to the Go server, as production does on one address.

The server creates the SQLite database at `DATABASE_PATH` and applies the migrations in `server/migrations/` when it starts.

## Commands

Run `make help` for the full list.

| Command              | What it does                                                   |
| -------------------- | -------------------------------------------------------------- |
| `make create-owner`  | First owner, from `NAME` and `EMAIL`                           |
| `make dev`           | Go server and web dev server together                          |
| `make dev-server`    | Go server only                                                 |
| `make dev-web`       | Web dev server only                                            |
| `make dev-desktop`   | Tauri app in development mode                                  |
| `make check`         | Go and web checks                                              |
| `make check-go`      | gofmt, `go vet`, and staticcheck (pinned as a Go tool)         |
| `make check-web`     | Route generation, TypeScript, and Prettier for `web/`          |
| `make check-desktop` | `cargo check` of the Tauri app                                 |
| `make test`          | Go tests with the race detector, and web tests with `bun test` |
| `make format`        | gofmt and Prettier over the whole repository                   |
| `make build`         | Web build and server binary                                    |

## Signing in

There is no password. `POST /api/auth/code` with `{"email": "…"}` emails a 6-digit code to an active teammate, and `POST /api/auth/verify` with the email and code sets the session cookie. Agents and scripts send an agent token as `Authorization: Bearer ssp_…`; a person creates one with `POST /api/tokens`, and the secret is shown only in that response.

To try it without the web interface, with `make dev-server` running:

```bash
curl -i -X POST http://127.0.0.1:8090/api/auth/code -H 'Content-Type: application/json' -d '{"email":"you@example.com"}'
```

Then send the emailed code, saving the cookie, and check who you are:

```bash
curl -i -c /tmp/sparrow-cookie -X POST http://127.0.0.1:8090/api/auth/verify -H 'Content-Type: application/json' -d '{"email":"you@example.com","code":"123456"}'
```

```bash
curl -b /tmp/sparrow-cookie http://127.0.0.1:8090/api/me
```

## The API, agents, and the CLI

[API.md](API.md) lists every endpoint. Agents connect to the MCP endpoint at `/mcp` with an agent token, and [DEPLOYMENT.md](DEPLOYMENT.md#connecting-agents) shows the configuration for Claude Code and Codex.

The same `sparrow` binary is also the command-line client. With `SPARROW_URL` and `SPARROW_TOKEN` set, `server/bin/sparrow issue for-branch` finds the issue for the current git branch, and `server/bin/sparrow help` lists every command.

## Quick capture and the desktop app

Ctrl+Q opens the quick capture panel on any page of the web app: a movable panel for a new issue or a new context, with a screenshot button that grabs the screen and lets you drag out the part to keep. In a browser the screenshot asks which screen, window, or tab to share.

The desktop app makes Ctrl+Q work over every application. `make dev-desktop` reads `PUBLIC_URL` from the root `.env` and opens that address directly, such as `http://localhost:3000` with `make dev` running. No address entry is needed. The app starts at login and stays in the tray when its window is closed. Ctrl+Q shows an always-on-top capture window, and its screenshot button captures the monitor natively. While the app runs, its shortcut is taken from other applications: choose another one (Ctrl+Shift+Q, Ctrl+Alt+Q, or Ctrl+Shift+Space) or pause it from the tray, which remembers the choice. The app also shows native notifications for new findings, finding decisions, and new contexts made by others. On Linux, Wayland sessions do not allow global shortcuts, and macOS asks for the screen recording permission on the first screenshot.

Desktop builds embed `PUBLIC_URL` and fail when it is missing or invalid. Only HTTPS origins are accepted, except HTTP on loopback for development; credentials, paths, queries, and fragments are rejected. Changing the deployment address requires rebuilding the desktop app. Previously saved server addresses are ignored. For direct Cargo or Bun builds, export `PUBLIC_URL` first.

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull request. It runs the Go checks, tests, and build; the web checks, tests, and build; the repository-wide format check; and the desktop `cargo check`. Deployment checks also cover secret filtering and failure recovery. After every check succeeds on `main`, the workflow publishes an immutable image to GHCR and deploys production with settings from Doppler. The manual **Apply production settings** workflow refreshes configuration without rebuilding the image. [DEPLOYMENT.md](DEPLOYMENT.md#production-on-wpstaging) describes the setup and recovery behavior.

## Releases

1. Set the GitHub repository variable `PUBLIC_URL` to the deployed server origin (the same value as the server's `PUBLIC_URL`). The release workflow embeds it in every desktop installer.
2. Set the new version in `desktop/src-tauri/tauri.conf.json` and `desktop/src-tauri/Cargo.toml`.
3. Commit, then push a matching tag, such as `v0.2.0`.

`.github/workflows/release.yml` then creates the GitHub Release with Linux server binaries for amd64 and arm64 and the built frontend they serve, and adds desktop installers for Linux, macOS (universal), and Windows. The repository is private, so only teammates with access can download them.

## Browser integration test

Run `SPARROW_E2E_CHROME=/usr/bin/google-chrome make test-e2e`, substituting the installed Chromium browser's absolute path. This builds the SPA, starts the Go HTTP handler with a temporary SQLite database, and drives the built interface with Chromium through Bun. No browser automation dependency or external email service is needed: the test's loopback server uses the existing fake mailbox. It never opens the development database.

The test covers sign-in redirects, emailed-code verification, issue creation, checklist and plan changes, comments after reload, live updates, keyboard finding decisions and their actor, follow-up issues, token creation, and signing out all devices. `make test-go` skips this browser test unless `SPARROW_E2E_CHROME` is set; `make test-web` runs the independent Bun unit tests.
