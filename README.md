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

Known production performance findings and deferred improvements are recorded in [docs/PERFORMANCE_ISSUES.md](docs/PERFORMANCE_ISSUES.md).

## License

Super Sparrow is source-available, not open source. KR Shanto and authorized members of the Anthillock team may use and modify it for internal development. Anyone else may run an unmodified copy for their own work, including business use and use by their own team. Selling or reselling it, offering it as a service or product to others, modifying it, and sharing copies or modifications outside your team require prior written permission. See [LICENSE.md](LICENSE.md). Outside contributions are not accepted; see [CONTRIBUTING.md](CONTRIBUTING.md).
