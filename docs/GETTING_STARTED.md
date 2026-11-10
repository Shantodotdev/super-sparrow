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

`sudo apt-get install libwebkit2gtk-4.1-dev librsvg2-dev libxdo-dev libssl-dev libclang-dev libxcb1-dev libxrandr-dev libdbus-1-dev libpipewire-0.3-dev libwayland-dev libegl-dev libgbm-dev libayatana-appindicator3-dev`

macOS needs the Xcode command line tools. Windows needs the Microsoft C++ build tools and WebView2, which Windows 11 includes.

## First run

1. `cp .env.example .env`, which sets `APP_ENV=development`. The server refuses to start when a setting it needs is missing and names each one. Development leaves `TRUSTED_PROXIES`, `WEB_DIR`, `GITHUB_WEBHOOK_SECRET`, and the backup settings optional, and turns off each feature whose settings are absent; the backup settings go all or none. The four `ATTACHMENTS_R2_` settings are required in development too. Production requires every setting; [DEPLOYMENT.md](DEPLOYMENT.md) covers them.
2. Set `RESEND_API_KEY` to a [Resend](https://resend.com/api-keys) API key. Sign-in codes and invitations are sent through Resend, so nobody can sign in without it. Until your domain is verified in Resend, keep the `onboarding@resend.dev` sender, which only delivers to your own Resend account's address.
3. Set `SIGN_IN_CODE_KEY` to the output of `openssl rand -hex 32`.
4. `make install` installs the Bun workspace and downloads the Go modules.
5. `make create-owner NAME="Your Name" EMAIL=you@example.com` adds the first owner. It works only on a database with nobody in it; after that, owners invite teammates.
6. `make dev` builds and runs the Go server and the web dev server together. The web dev server forwards `/api` to the Go server, as production does on one address.

The server creates the SQLite database at `DATABASE_PATH` and applies the migrations in `server/migrations/` when it starts. Attached files are kept in a private R2 bucket named by the `ATTACHMENTS_R2_` settings; create a development bucket and token as described in [DEPLOYMENT.md](DEPLOYMENT.md).

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

Meta+Shift+Q opens the quick capture panel on any page of the web app: a movable, rounded panel in the app’s dark theme for a new issue (Ctrl+Q closes it). It opens with the issue title focused, shows attached files above the option buttons, and has a screenshot button that grabs the screen and lets you drag out the part to keep, and a way to attach files. In a browser the screenshot asks which screen, window, or tab to share.

The desktop app makes Meta+Shift+Q work over every application. `make dev-desktop` reads `PUBLIC_URL` from the root `.env` and opens that address directly, such as `http://localhost:3000` with `make dev` running. No address entry is needed. On Linux, the development command registers a hidden desktop entry for the taskbar icon. Development uses a separate app identifier, so its launcher and settings do not override an installed release. The app starts at login and stays in the tray when its window is closed. Meta+Shift+Q shows an always-on-top capture window, and its screenshot button captures the monitor natively. Open Settings → Keyboard shortcuts to edit, disable, or reset your account’s bindings. The desktop applies your quick-capture binding system-wide; its tray menu opens the same settings page. Edits synchronize across sessions and devices. If a key is reserved or already registered by another app, the page reports the registration failure and preserves the previous working global shortcut. The app also shows native notifications for new findings, and finding decisions made by others. On Linux, Wayland sessions do not allow global shortcuts, and macOS asks for the screen recording permission on the first screenshot.

Desktop builds embed `PUBLIC_URL` and fail when it is missing or invalid. Only HTTPS origins are accepted, except HTTP on loopback for development; credentials, paths, queries, and fragments are rejected. Changing the deployment address requires rebuilding the desktop app. Previously saved server addresses are ignored. For direct Cargo or Bun builds, export `PUBLIC_URL` first.

Desktop icons are generated from the shared brand source at `web/public/icon-512.png`. Run `bun --filter @super-sparrow/desktop icons` after changing that source.

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull request. It runs the Go checks, tests, and build; the web checks, tests, and build; the repository-wide format check; and the desktop `cargo check`. Deployment checks also cover secret filtering and failure recovery. After every check succeeds on `main`, the workflow publishes an immutable image to GHCR and deploys production with settings from Doppler. The manual **Apply production settings** workflow refreshes configuration without rebuilding the image. [DEPLOYMENT.md](DEPLOYMENT.md#production-on-wpstaging) describes the setup and recovery behavior.

## Releases

1. One-time setup: add the repository secrets `TAURI_SIGNING_PRIVATE_KEY` (the contents of the updater private key) and, if the key has a password, `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`. Generate a key with `bunx tauri signer generate -w <path>` from `desktop/`, put its public key in `plugins.updater.pubkey` in `desktop/src-tauri/tauri.conf.json`, and keep the private key safe: without it, installed apps cannot accept updates.
2. Set the GitHub repository variable `PUBLIC_URL` to the deployed server origin (the same value as the server's `PUBLIC_URL`). The release workflow embeds it in every desktop installer.
3. Set the new version in `desktop/src-tauri/tauri.conf.json` and `desktop/src-tauri/Cargo.toml`.
4. Commit and merge the version change, then tag that commit and push the tag: `git tag v0.2.0 && git push origin v0.2.0`. The tag must match the version, or the workflow fails.

Only a pushed `v*` tag releases the desktop app. Merging to `main` deploys the server but builds no installers and updates no installed apps.

`.github/workflows/release.yml` then creates the GitHub Release with Linux server binaries for amd64 and arm64 and the built frontend they serve, and adds desktop installers for Linux, macOS (universal), and Windows. The release also carries signed update packages and `latest.json`. Installed apps check that file shortly after launch and every six hours, then download, install, and restart on their own, but only while the main window is hidden in the tray. Updates apply to the Windows installer, the macOS app, and the Linux AppImage; `.deb` and `.rpm` installs must be upgraded by hand. Development builds never update. Installers are not code-signed with Apple or Windows certificates, so macOS and Windows may warn on the first install; updates are still verified by the update signature.

## Browser integration test

Run `SPARROW_E2E_CHROME=/usr/bin/google-chrome make test-e2e`, substituting the installed Chromium browser's absolute path. This builds the SPA, starts the Go HTTP handler with a temporary SQLite database, and drives the built interface with Chromium through Bun. No browser automation dependency or external email service is needed: the test's loopback server uses the existing fake mailbox. It never opens the development database.

The test covers sign-in redirects, emailed-code verification, issue creation, checklist and plan changes, comments after reload, live updates, keyboard finding decisions and their actor, follow-up issues, token creation, and signing out all devices. `make test-go` skips this browser test unless `SPARROW_E2E_CHROME` is set; `make test-web` runs the independent Bun unit tests.
