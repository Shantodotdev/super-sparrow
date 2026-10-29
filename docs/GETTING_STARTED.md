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

1. `cp .env.example .env`. Every setting is required; the server stops and names any that is missing.
2. `make install` installs the Bun workspace and downloads the Go modules.
3. `make dev` builds and runs the Go server and the web dev server together. The web dev server forwards `/api` to the Go server, as production does on one address.

The server creates the SQLite database at `SPARROW_DATABASE_PATH` and applies the migrations in `server/migrations/` when it starts.

## Commands

Run `make help` for the full list.

| Command              | What it does                                                   |
| -------------------- | -------------------------------------------------------------- |
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

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull request. It runs the Go checks, tests, and build; the web checks, tests, and build; the repository-wide format check; and the desktop `cargo check`.

## Releases

1. Set the new version in `desktop/src-tauri/tauri.conf.json` and `desktop/src-tauri/Cargo.toml`.
2. Commit, then push a matching tag, such as `v0.2.0`.

`.github/workflows/release.yml` then creates the GitHub Release with Linux server binaries for amd64 and arm64, and adds desktop installers for Linux, macOS (universal), and Windows. The repository is private, so only teammates with access can download them.
