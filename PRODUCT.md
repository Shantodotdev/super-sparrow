# Product

Super Sparrow is a task and code-review tracker for small teams that build software with AI coding agents such as Claude Code and Codex. It treats agents as team members: they plan, review, and fix through their own identities, and every action shows who did it.

## The problem

Our code is now written at agent speed, but our tracking tools still work at human speed.

- **Reviews are hard to read.** A feature's review rounds are long comments on one issue. Finding R3 from round 2 means scrolling through everything.
- **Findings can't be acted on one by one.** A finding is text inside a comment, so it can't have its own status, owner, or discussion.
- **Agent work is invisible.** Agents use the developer's account, so the history shows the developer doing everything. Nobody can tell which agent found a bug or which one fixed it.
- **Agents fight the tool.** They have to write strictly formatted markdown and hope it stays consistent, because the tool has no place for review data.
- **No native Linux app.** Every link opens a browser tab.
- **The cost grows with the team.** Free plans run out as issues grow, and paid plans charge per seat.

## Who it is for

- **Developers** who plan features, ask agents for reviews, and decide what to fix.
- **AI coding agents** (Claude Code, Codex, and others) that read plans, post findings, fix code, and report results.
- **Team leads** who want to see where each feature stands and how agents are performing.

## What it does

1. **Plans live on issues.** Each feature has an issue whose description is its plan: goal, decisions, features, edge cases, and checks.
2. **Each review has its own page.** A review round records which commits it covered, and lists its findings as separate cards.
3. **Each finding stands alone.** A finding has a number, severity, problem, fix, evidence (file, line, commit), a status, and its own discussion thread. The developer decides fix, wontfix, or later per finding, with one keystroke.
4. **Every action has an author.** Humans and agents each have their own identity. The history reads "Found by Codex, fixed by Claude, approved by Shanto."
5. **Agents connect directly.** A built-in MCP server and a CLI give agents tools shaped around the workflow: find the issue for this branch, start a review, add a finding, mark it fixed.
6. **It is fast everywhere.** A native desktop app for Linux, macOS, and Windows, plus the same interface in a browser, with live updates and keyboard shortcuts.
7. **We host it.** It runs as one small service on our own server, with an hourly encrypted sync and daily encrypted backups.

## What it is not

- Not a general project management suite. No cycles, estimates, roadmaps, time tracking, or custom fields.
- Not a document store or a secrets manager.
- Not a CI/CD system, but it is built so one can connect to it later.

## Success looks like

- A developer reaches any finding in two clicks or fewer, with no scrolling through other rounds.
- Every finding, fix, and decision shows whether a human or which agent made it.
- Agents run the full plan, review, and fix loop through the tool without formatting instructions in their prompts.
- The team stops using Linear for this project.
- The service runs comfortably on a 2 vCPU, 4 GB server next to other services.

## Later

- Connection to our own CI/CD platform, or one combined product.
- GitHub pull request sync beyond closing issues on merge.
- Reports on agent performance: findings per agent, confirmed rate, fix rate.
- Possibly offering it to other teams who build with agents.
