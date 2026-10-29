# Super Sparrow

A task and code-review tracker for a team that builds software with AI coding agents. Agents plan, review, and fix through their own identities, and every action shows who did it. See [docs/PRODUCT.md](docs/PRODUCT.md).

## Repository layout

| Folder     | What it holds                                                     |
| ---------- | ----------------------------------------------------------------- |
| `server/`  | Go server: HTTP API, MCP endpoint, migrations, and the CLI        |
| `web/`     | TanStack Start (React) frontend in SPA mode, served by the server |
| `desktop/` | Tauri v2 desktop app that opens the server's page                 |
| `docs/`    | Product, requirements (SRS), and the build plan                   |

## Quick start

Install Go, Bun, and Rust, then:

1. `cp .env.example .env`
2. `make install`
3. `make dev`, then open the web address printed in the terminal.

[docs/GETTING_STARTED.md](docs/GETTING_STARTED.md) covers the toolchain versions, every `make` target, and releases. Coding agents start from [AGENTS.md](AGENTS.md).
