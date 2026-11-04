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

1. `cp .env.example .env`, which sets `APP_ENV=development`. The server refuses to start when a setting it needs is missing and names each one. Development leaves `TRUSTED_PROXIES`, `WEB_DIR`, `CLI_DIR`, `GITHUB_WEBHOOK_SECRET`, and the backup settings optional, and turns off each feature whose settings are absent; the backup settings go all or none. Production requires every setting; [DEPLOYMENT.md](DEPLOYMENT.md) covers them.
2. Set `RESEND_API_KEY` to a [Resend](https://resend.com/api-keys) API key. Sign-in codes and invitations are sent through Resend, so nobody can sign in without it. Until your domain is verified in Resend, keep the `onboarding@resend.dev` sender, which only delivers to your own Resend account's address.
3. Set `SIGN_IN_CODE_KEY` to the output of `openssl rand -hex 32`.
4. Set `SECRETS_ENCRYPTION_KEY` to the output of `openssl rand -base64 32`. It is required even in development, because the server encrypts stored secrets with it. A development database encrypted with one key cannot be read with another, so keep the key if you keep the database.
5. `make install` installs the Bun workspace and downloads the Go modules.
6. `make create-owner NAME="Your Name" EMAIL=you@example.com` adds the first owner. It works only on a database with nobody in it; after that, owners invite teammates.
7. `make dev` builds and runs the Go server and the web dev server together. The web dev server forwards `/api` to the Go server, as production does on one address.

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

There is no password. `POST /api/auth/code` with `{"email": "…"}` emails a 6-digit code to an active teammate, and `POST /api/auth/verify` with the email and code sets the session cookie. The CLI signs in the same way through `POST /api/auth/cli/verify` and sends the result as `Authorization: Bearer ssc_…`. Agents in cloud sessions and scripts send an agent token as `Authorization: Bearer ssp_…`; a person creates one with `POST /api/tokens`, and the secret is shown only in that response.

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

The same `sparrow` binary is also the command-line client. A person's local agents use it through that person's sign-in and need no token; agent tokens are for cloud sessions.

### CLI quick start

With `make dev` running and the binary built by `make build` (or downloaded as [DEPLOYMENT.md](DEPLOYMENT.md#installing-and-updating) describes):

1. `server/bin/sparrow login --url http://localhost:8090` emails a 6-digit code to the address you give, asks for it, and stores the session in the operating system's keychain, or in a private file when there is none. Use the `LISTEN_ADDR` from your `.env`. The CLI refuses plain `http` except for `localhost`.
2. `server/bin/sparrow whoami` shows who you are and which agent the CLI acts as. Inside Claude Code or Codex it acts as your agent of that name; elsewhere it acts as you. `--agent claude-code|codex` before the command, or `SPARROW_AGENT`, chooses one.
3. `server/bin/sparrow tools` lists every tool the agents have. Each is also a command, such as `server/bin/sparrow get-issue --key WP-1`, and `--help` after the name lists its flags. Put `--json` before the command for raw JSON. `issue for-branch` finds the issue for the current git branch, and `help` lists every command.
4. `server/bin/sparrow logout` ends the session on the server and forgets it here.

For a project with secrets turned on, `sparrow secrets list --project WP` shows the key names of its configs, `sparrow secrets download --project WP --format env` prints your dev, and `sparrow run --project WP -- COMMAND` runs a command with your dev in its environment. `secrets set`, `unset`, and `import` change your own dev overrides. Pass values on standard input, as in `printf %s "$VALUE" | sparrow secrets set --project WP NAME` or `sparrow secrets import --project WP --file -`, so they stay out of your shell history and process list. Staging and production are managed in the browser. The Secrets page of a project has the rest, and [API.md](API.md#secrets) the rules.

Cloud sessions set `SPARROW_URL` and `SPARROW_TOKEN` (an `ssp_` agent token) instead of signing in. `sparrow update` replaces the CLI with the latest signed release from the server.

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull request. It runs the Go checks, tests, and build; the web checks, tests, and build; the repository-wide format check; and the desktop `cargo check`. Deployment checks also cover secret filtering and failure recovery. After every check succeeds on `main`, the workflow adds the latest signed CLI release to the image, publishes the immutable image to GHCR, and deploys production with settings from Doppler. The manual **Apply production settings** workflow refreshes configuration without rebuilding the image. [DEPLOYMENT.md](DEPLOYMENT.md#production-on-wpstaging) describes the setup and recovery behavior.

## Releases

1. Set the new version in `desktop/src-tauri/tauri.conf.json` and `desktop/src-tauri/Cargo.toml`.
2. Commit, then push a matching tag, such as `v0.2.0`.

`.github/workflows/release.yml` then creates the GitHub Release with Linux server binaries for amd64 and arm64 and the built frontend they serve, and adds desktop installers for Linux, macOS (universal), and Windows. It also builds the `sparrow` CLI for Linux, macOS, and Windows, and a reviewer approves the `release` environment before the CLI is signed and added with its checksums. [DEPLOYMENT.md](DEPLOYMENT.md#cli-releases) covers the one-time signing setup. The repository is private, so only teammates with access can download them.

## Browser integration test

Run `SPARROW_E2E_CHROME=/usr/bin/google-chrome make test-e2e`, substituting the installed Chromium browser's absolute path. This builds the SPA, starts the Go HTTP handler with a temporary SQLite database, and drives the built interface with Chromium through Bun. No browser automation dependency or external email service is needed: the test's loopback server uses the existing fake mailbox. It never opens the development database.

The test covers sign-in redirects, emailed-code verification, issue creation, checklist and plan changes, comments after reload, live updates, keyboard finding decisions and their actor, follow-up issues, token creation, and signing out all devices. `make test-go` skips this browser test unless `SPARROW_E2E_CHROME` is set; `make test-web` runs the independent Bun unit tests.
