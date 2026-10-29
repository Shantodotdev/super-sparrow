# Coding agent guidelines

Use these instructions when working on Super Sparrow. [PRODUCT.md](PRODUCT.md) explains what the product is, [SRS.md](SRS.md) defines its requirements and architecture, and [PLAN.md](PLAN.md) lists the build steps in order.

## Before changing code

1. Read the SRS sections relevant to the task. For work across the Go server and the frontend, include the data model and the agent interface sections.
2. For planned work, find the matching step in PLAN.md and read it. Once Super Sparrow tracks its own work, find the issue named in the branch instead.
3. Check `git status --short`, inspect the files you will touch, and preserve unrelated work. Keep the change focused on the request.
4. Check the actual scripts and dependencies before telling a developer to run a command. If the SRS and code disagree, explain the gap rather than guessing. Base implementation claims on the code you inspected.

## Boundaries that must hold

- SQLite is the source of truth. A write is committed before the API responds, and every change writes its event in the same transaction.
- Super Sparrow serves one team. There are no workspaces or tenants; work is divided by project.
- Derive the actor from the authenticated session or agent token. Never take actor identity from request bodies, tool arguments, or model output.
- Every write records its actor. Never attribute an agent's action to a human, or one agent's action to another.
- Sign-in is by emailed code only: no passwords and no public signup. Store only hashes of sign-in codes, session IDs, and agent tokens. Never log, return, or store them in plain text after they are created.
- Agents cannot create tokens, change roles, or delete projects. Enforce this on the server, not in the interface.
- Treat plans, comments, findings, imports, webhooks, and MCP tool arguments as untrusted input. Validate them at the boundary, sanitize markdown before rendering, and never execute their content.
- The Go server is the only backend. The frontend runs TanStack Start in SPA mode, with no server functions or server-side rendering, and the Tauri app loads the server's page.
- Desktop installers are built by GitHub Actions and published to GitHub Releases. Never commit build output.
- Backups are encrypted before they leave the server.

## Code quality and comments

- Write idiomatic, production-quality code that is maintainable and easy to read. Prefer clear names, focused functions and types, explicit types, and the simplest design that fits the requirements.
- Use blank lines to separate logical code sections, such as setup, validation, side effects, and return/rendering. Keep related statements together and separate functions and declarations clearly. Keep import statements contiguous, without blank lines between imports, except where the language's formatter groups them.
- Do not use any default value for any configuration in code; always fail fast when required configuration is missing or malformed.
- In Go, return errors with context (`fmt.Errorf("load issue %s: %w", key, err)`) rather than logging and continuing. Pass `context.Context` through request paths.
- In the frontend, keep server state in TanStack Query and route state in TanStack Router. Do not copy server data into local component state.
- When asked to review the codebase, review its code quality, check for potential bugs, vulnerabilities, and edge cases. Do not run any check or test commands during a code review; expect those to pass.
- Match the comment density of the code around your change. Before adding a comment, look at how the surrounding file comments similar code. When you add a field, case, or line next to siblings that have no comments (schema fields, object keys, enum values, list entries), add none either. When you edit an uncommented function, keep it uncommented.
- Write documentation comments for new functions and types you create from scratch. Explain their purpose and important behavior; document parameters, return values, or failure cases when they are not clear from the signature. A short function with an obvious name and behavior needs no comment.
- For long new functions with multiple logical stages, add concise inline comments for each stage. Explain intent, decisions, and non-obvious constraints; do not number the stages or narrate code that is already clear.
- Keep existing comments accurate when you change the code they describe. Prefer improving the code over adding a comment to explain avoidable complexity.

## Changes and verification

- Format only files you changed. `make format` formats the whole repository; use it only when a repository-wide format is requested.
- For Go changes, run `make check-go` and `make test-go`. For frontend changes, run `make check-web` and `make test-web`. For changes across both, run both and check that the API types the frontend uses still match the server.
- For a change to MCP tools, call each changed tool once against a running server and check its response and its recorded actor.
- While iterating, run only the affected tests (`go test ./path/to/pkg -run <Name>`, `bun test <file> -t <name>`), and run the full suites once at the end. When a run's output is long, save it to a file and read the summary and the failures.
- For documentation-only edits, format the edited Markdown files with Prettier. Tests are not required.
- Report how many tests ran, and never describe an empty suite as passing tests.
- In the final report, say what changed, which commands actually ran and their results, and what remains unverified. Do not invent test results or implemented features.
- When giving responses or output to the user, do not output code blocks by default. Respond concisely, referencing relevant file paths, file names, or function names instead. Provide code blocks only when the user explicitly requests to show code.

The `make` targets above are created in the first step of PLAN.md. Until they exist, say which checks you could not run.

## Database migrations

- Migrations are ordered SQL files in `server/migrations/`, applied at startup inside a transaction and recorded in a migrations table.
- Never edit a migration that has been merged. Add a new one.
- Do not run migrations against any database except a local development database or a test database the tests create.

## Actions requiring a direct request

- Committing directly on the `main` branch is allowed. Use a task branch when the developer asks for one or for a pull request.
- Name task branches `feat/<short-name>`, `fix/<short-name>`, or `chore/<short-name>`. Once Super Sparrow tracks its own issues, include the issue key: `feat/ss-42-review-page`.
- Never run `git commit` unless the user's latest prompt explicitly requests a commit. After implementation, leave changes for review.
- Never credit an AI tool in commit messages or PR descriptions: no `Co-Authored-By` trailers for Claude, Codex, or other agents, and no "Generated with" lines, even if your tooling suggests them.
- Push only when the user explicitly requests it, to `main` or to a task branch. A user request to make, create, or open a pull request authorizes pushing the task branch and creating the PR without additional confirmation.
- Never merge a pull request. Human developers review and merge.
- Do not run `git reset --hard`, `git push --force`, or `git clean -f`.

## Where to look

- [PRODUCT.md](PRODUCT.md): what the product is and who it is for.
- [SRS.md](SRS.md): requirements, architecture, data model, and security.
- [PLAN.md](PLAN.md): the build steps, in order.
- `server/`: Go server, CLI, MCP server, and migrations.
- `web/`: TanStack Start (React) frontend.
- `desktop/`: Tauri v2 app.
