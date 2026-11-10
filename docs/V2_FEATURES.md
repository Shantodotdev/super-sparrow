# V2 features

Status: proposal for product discussion, not an implementation plan or a committed release scope.

Date: November 1, 2026.

## Product direction

Extend Super Sparrow from a task and code-review tracker into the team's shared system for taking work from a plan to a verified release. Connect the issue, code revision, sandbox, checks, review evidence, approvals, and deployed release in one place.

WP Agent is the first use case. Developers and coding agents should use one CLI for everyday development and operations, with the same authorized operations and history available through Super Sparrow's web interface and MCP tools where appropriate.

The value is a consistent workflow and reliable shared context. A command wrapper around today's scripts would make commands easier to find, but would not address fragmented state, unclear ownership, or recovery.

This proposal expands the direction described under “Later” in [PRODUCT.md](PRODUCT.md). It does not change the V1 requirements in [SRS.md](SRS.md) or the build steps in [PLAN.md](PLAN.md).

## Problems to address

WP Agent already has useful sandbox helpers, worktree slots, fake providers, automated tests, model evaluations, deployment safeguards, and an emerging server admin CLI. These capabilities are spread across Make targets, repository scripts, local state files, CI, deployment hosts, and documentation.

- Developers and agents must rediscover which commands, ports, environments, and settings apply.
- Local development, agent sandboxes, browser tests, and the local production stack have different lifecycles and configuration behavior.
- Sandbox ownership and status are difficult for another teammate or agent to discover.
- Deployment logic crosses Bash scripts, sourced libraries, remote commands, generated settings, and release marker files. Following the whole operation is difficult.
- Verification results and operational history are disconnected from issues and findings.
- Parallel work can leave behind processes, environments, and resources whose ownership is unclear.
- Sharing local work requires additional setup or using the shared staging environment.

The assessment included WP Agent's main checkout and the in-progress admin CLI in Claude's `admin-panel-businesses-baa653` worktree. That worktree contained substantial uncommitted changes; its behavior is not assumed to be a released capability.

## 1. One CLI for the workflow

Provide one discoverable entry point for:

- Finding the issue and plan for the current branch.
- Creating, reusing, inspecting, stopping, resetting, and retiring sandboxes.
- Starting the services needed for a development or verification session.
- Running checks, tests, browser scenarios, and model evaluations.
- Reading service status, failures, and relevant logs.
- Sharing a sandbox through a temporary preview URL.
- Attaching verification evidence to reviews and findings.
- Requesting and inspecting deployments, configuration changes, and authorized recovery.
- Performing authorized server administration, job operations, and backup or restore operations.

Give developers readable output and agents structured, compact results. Interactive flows can serve human tasks, while automation has explicit inputs, outcomes, and exit statuses. Sensitive human-only flows remain restricted.

The CLI, web interface, and MCP tools should agree on permissions, operation results, and history. An operation started from one interface should be inspectable from another.

## 2. Managed sandbox lifecycle

The CLI takes full ownership of sandbox orchestration. Agents request an environment suitable for their task; they do not choose worktree slots, find free ports, inspect Docker or process lists, decide which services to start, or manually repair infrastructure. These responsibilities belong to the CLI throughout the sandbox's lifetime.

Make each sandbox a discoverable environment associated with a project, issue or branch, owner, machine, and code revision.

- Show the environment's purpose, services, addresses, health, provider modes, and configuration identity without revealing secrets.
- Automatically allocate and track ports, database identities, container and network names, storage, and other resources so parallel worktrees and teammates do not accidentally collide. Agents do not supply or calculate resource slots.
- Coordinate simultaneous requests so two agents cannot allocate the same resource or independently start competing copies of a shared service.
- Resolve required services and their dependencies, start what is missing, and wait for actual readiness before reporting the environment as ready.
- Reuse healthy environments and preserve their data when that is the requested behavior.
- Detect unhealthy services, interrupted startup, stale tracking records, and changed configuration; reconcile them with the actual running environment and perform safe, bounded recovery automatically.
- Handle occupied ports without asking the agent to find replacements or kill processes. Preserve unrelated processes and update dependent service configuration when resource assignments change.
- Distinguish restart, recovery, reseed, reset, and deletion so their effects are clear.
- Track resource ownership and clean up only resources belonging to that environment.
- Manage shared infrastructure lifetimes so stopping one sandbox does not interrupt another sandbox that still needs it.
- Support explicit expiration and retirement, with visibility into stale environments and cleanup failures.
- Preserve enough state for another session to understand and continue the work.

### Agent-facing contract

An agent asks for the desired capability, such as a ready application for browser verification or a deterministic messaging scenario. The CLI resolves the environment, prepares its prerequisites, and returns an environment identifier, usable entry points, provider modes, and a concise readiness result. Subsequent API calls, tests, logs, and previews resolve their services through that environment rather than requiring the agent to reconstruct addresses or maintain process state.

Repeated requests should converge on a suitable ready environment without duplicate launches or unintended reseeding. Service supervision and lifecycle tracking must survive the command that initiated startup; the agent should not need to keep a chat or terminal session alive to manage the stack.

If automatic recovery cannot safely succeed, return a precise blocked or failed result naming the cause and any required human action. Do not silently reset data, terminate unrelated processes, enable paid providers, or weaken isolation to make startup succeed. Full operational ownership includes reporting failure honestly; it does not grant authority to cross permission or data-safety boundaries.

Local development, deterministic testing, real-provider evaluation, and production-like verification have different purposes. Make those purposes visible while giving them consistent lifecycle controls.

## 3. Stronger sandbox isolation

Treat application test isolation and coding-agent security isolation as separate requirements.

The current WP Agent sandbox separates application databases, Redis use, ports, and generated secrets. Its application services run as ordinary host processes with inherited environment variables plus overrides. This is useful test isolation, but does not establish a security boundary around an autonomous coding agent.

The proposed sandbox should have explicit, enforced limits for:

- Accessible files, writable locations, and persisted data.
- Credentials available to each service or operation.
- Network destinations and external integrations.
- CPU, memory, disk, and execution time.
- Actions that can spend money, send messages, or change external state.
- Access to unrelated worktrees, environments, hosts, and production resources.

Show which providers are simulated and which are live for each integration. A single “fake mode” label is insufficient when voice, models, scraping, email, or other providers can behave differently.

Super Sparrow records and coordinates these boundaries; the execution environment must actually enforce them. A sandbox record or a preview tunnel does not itself provide isolation.

## 4. Unified verification and evidence

Make checks and their results part of the issue's history, with evidence linked to the exact code and environment used.

| Verification                   | What its result establishes                                        |
| ------------------------------ | ------------------------------------------------------------------ |
| Static checks and unit tests   | Code quality and focused behavior within the checked scope         |
| Integration and contract tests | Behavior across databases, services, and language boundaries       |
| Fake-provider scenarios        | Application behavior under controlled provider responses           |
| Browser scenarios              | User-visible behavior for the services and flows exercised         |
| Real-model evaluations         | Observed language, grounding, tool use, quality, latency, and cost |
| Production-like checks         | Behavior of release packaging and configuration                    |
| Live integration checks        | Observed behavior against a real external provider                 |

- Record executed checks, counts, failures, skipped coverage, duration, and relevant artifacts.
- Preserve model and provider configuration for evaluations, with usage and cost where available.
- Keep concise summaries connected to detailed logs, traces, screenshots, and reports.
- Allow evidence to support a review round or the verification of a particular finding.
- Distinguish an environment being healthy from a feature having been verified.
- Make missing services and unexercised flows visible rather than implying complete coverage.

For example, WP Agent's inspected shared sandbox service list omitted the operations worker, while its Playwright setup excluded admin, scheduler, and live calls. Those exclusions do not establish that the features are untested elsewhere; they illustrate why verification scope must be explicit.

## 5. Clear deployment and configuration operations

Replace the developer-facing maze of deployment scripts with complete, understandable operations.

- Show the target environment, selected release, affected services, and configuration change before an authorized action.
- Track progress, failure stage, diagnostics, and recovery outcome as one operation.
- Distinguish deploying code from applying settings without rebuilding or changing the release.
- Preserve existing protections: configuration validation, deployment serialization, release ordering, smoke tests, and records of known-good releases.
- Show platform and voice deployment outcomes separately, including partial success and incompatible component states.
- Record the release actually observed on each target, rather than treating a request as proof of success.
- Expose authorized rollback and recovery with clear limits.

Rolling back images or settings does not automatically reverse database migrations, sent messages, or other external side effects. The interface must make these differences understandable.

The intended outcome is maintainable operational behavior with clear inputs and results. The choice of language, runtime, deployment transport, and relationship to the existing CI system remains for later design.

## 6. Organized server administration

Give developers one consistent interface to approved server operations, including service diagnostics, job inspection and recovery, backups and restores, and platform administrator management.

WP Agent's emerging `wp` CLI already includes administrator setup and recovery, schema compatibility checks, and transactional audit records. Preserve those protections and shared application security rules when defining its relationship to Sparrow.

- Distinguish routine diagnostics, operational mutations, production deployment, and administrator recovery.
- Keep authenticator enrollment, reset, and other sensitive recovery actions under explicit human control.
- Derive identity from trusted authentication or the authorized execution context.
- Record who requested, approved, and executed an action without attributing an agent's work to a human.
- Keep business data and its authorization rules owned by WP Agent.

One CLI does not mean one permission level, unrestricted shell access, or giving agents database credentials and administrative authority.

## 7. Shared operation history and agent handoffs

Make operational context survive individual chats, terminals, and agent sessions.

- Give operations stable identifiers and inspectable outcomes.
- Distinguish accepted, queued, running, completed, failed, canceled, and unknown outcomes as applicable.
- Track connection loss and stale observations instead of showing an old healthy state as current.
- Link requests, approvals, execution, logs, results, and recovery to the relevant issue and environment.
- Let another authorized developer or agent continue from recorded evidence without repeating setup or investigation.
- Preserve the actual actors involved, including system actions and automation.

An accepted operation must be durably recorded, but acceptance is not completion. External execution and deployment cannot be made part of a SQLite transaction; their observed results need their own history.

## 8. Token-efficient agent interactions

Reduce the context agents need to discover commands, reconstruct environment state, diagnose failures, and report results.

- Return concise summaries by default, with structured fields and references to full artifacts.
- Support targeted retrieval of a service's failure, an operation stage, or selected result fields.
- Avoid repeatedly emitting unchanged status and large logs.
- Reuse healthy environments when appropriate instead of repeatedly starting and seeding the stack.
- Keep command behavior and errors consistent across interfaces.
- Retain context about the revision, configuration, provider modes, and completed verification.

The expected savings come from fewer discovery steps, repeated setups, failed attempts, and handoff explanations. A single command that emits large output does not automatically save tokens. Measure token usage, tool calls, and completion time before claiming a reduction.

## 9. Temporary sandbox previews and port forwarding

Let a teammate share a local sandbox through the team's domain without separately installing or configuring `cloudflared` on their desktop.

### User experience

1. A teammate starts a sandbox and requests sharing through the unified CLI.
2. The CLI authenticates and maintains an encrypted outgoing connection to the preview service.
3. Sparrow returns a preview URL, such as `wp-354-alice.preview.example.com`.
4. Authorized reviewers open the URL in an ordinary browser without installing a client.
5. The issue shows the preview's owner, environment, connection status, access policy, and expiry.
6. The owner or another authorized person can stop sharing, and expired previews stop accepting traffic.

The conceptual request path is: reviewer → Cloudflare → preview gateway → desktop connection → local application. This is reverse tunneling: the desktop initiates the connection, avoiding the need to configure incoming router port forwarding in typical networks that allow the outgoing connection.

### Scope and behavior

- Share the sandbox's related web services together, including the dashboard, API, authentication flows, media, streaming updates, and shop where required.
- Support independent previews for parallel teammates and issues.
- Restrict access to intended reviewers, with explicit policies for guest access or provider callbacks when needed.
- Allow controlled webhook development where provider callback and verification requirements permit it.
- Publish only explicitly authorized sandbox services; do not expose database ports or arbitrary desktop services.
- Revoke preview access when sharing stops or authorization expires.
- Make disconnects and unavailable applications visible.

The desktop must remain awake, connected, and running the CLI connection and application. A preview shares local execution; it is not a deployment.

### Cloudflare's role

The existing server and domain can provide a shared entry point. Cloudflare can proxy that entry point, or a centrally operated Cloudflare Tunnel can connect the preview gateway to Cloudflare. In the latter arrangement, `cloudflared` runs on the server rather than requiring each teammate to install it.

A server-side Cloudflare connector cannot reach a teammate's `localhost` by itself. The desktop still needs an active connection, which the unified CLI can provide. The transport and gateway design remain open.

Start the product scope with web applications and APIs. Live voice, SIP, WebRTC media, and other protocols require separate assessment; a web preview connection does not automatically support them.

Reference behavior: [Cloudflare Tunnel fundamentals](https://developers.cloudflare.com/tunnel/concepts/), [Cloudflare Access for web applications](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/), and [OpenSSH remote forwarding](https://man.openbsd.org/ssh).

## 10. Replace most Makefile workflows

Move most of WP Agent's developer automation from its Makefile into the Super Sparrow CLI. The CLI becomes the documented entry point for developers, agents, and applicable CI operations. This is an explicit replacement goal, beyond adding a CLI that invokes the same Make targets.

### Problems in the current Makefile

The inspected Makefile combines environment setup, dependency installation, infrastructure, development processes, sandbox management, verification, container builds, seeding, voice infrastructure, and cleanup. Its handwritten help menu repeats information maintained elsewhere, including fixed development ports even though addresses depend on the worktree slot.

- `dev` starts infrastructure and the Bun workspace development command; Python workers and voice have separate targets. A developer still needs repository instructions to know how to start the complete application.
- Prerequisites vary between service targets. API and dashboard startup prepare infrastructure, while several workers and other services assume prerequisites already exist.
- Infrastructure `down` and `logs` depend on `ts-ready`, which can install dependencies and regenerate Prisma before stopping services or reading logs.
- Combined checks use Make's keep-going behavior, while individual recipes have their own failure behavior. Test collection and empty-suite handling differ between runtimes.
- Formatting targets cover different scopes, and the general format command intentionally touches the whole repository.
- Environment setup, generated data, real-model evaluations, and cleanup have materially different effects but share the same flat target interface.
- Voice startup contains conditional shell orchestration in addition to invoking Compose.

These are workflow and discoverability problems as well as readability problems. Moving the same recipes into another large file would not resolve them.

### Responsibilities to move

| Current Makefile area                                            | Responsibility of the proposed CLI                                                                                         |
| ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `env`, `install`, `install-ts`, `install-py`, `ts-ready`         | Explain prerequisites, prepare checkout settings, install the selected dependencies, and manage required generated clients |
| `up`, `down`, `logs`, `infra-up`                                 | Manage and diagnose the selected environment's infrastructure with clear ownership                                         |
| `dev`, `dev-*`                                                   | Start the full development environment or selected services, with explicit dependencies and lifecycle behavior             |
| `sandbox-up`, `sandbox-ensure`, `sandbox-down`, `sandbox-status` | Provide the managed sandbox lifecycle described above                                                                      |
| `check`, `check-ts`, `check-py`, `format-check`                  | Run scoped verification and report each check's result consistently                                                        |
| `test`, `test-ts`, `test-py`, `test-e2e`, `eval-agent`           | Select verification scope, run it in the appropriate environment, and retain results and evidence                          |
| `format`, `format-ts`, `format-py`                               | Make file scope explicit and distinguish formatting changes from read-only checks                                          |
| `seed`, `seed-inbox`                                             | Generate fixtures for an explicitly identified, authorized development or sandbox database                                 |
| `build`, `start`                                                 | Build and run the local production-like stack as a distinct environment                                                    |
| `voice-setup`, `voice-up`, `voice-down`, `voice-status`          | Make voice prerequisites, lifecycle, health, and live-provider behavior part of the same interface                         |
| `clean`                                                          | Explain and remove selected generated artifacts or owned resources without confusing cleanup with data deletion            |
| `help`                                                           | Provide discoverable, task-specific help that reflects actual options and resolved environment context                     |

### Expected behavior

- Show the selected project, checkout, environment, and resolved addresses when relevant; do not make developers memorize slot arithmetic or fixed ports.
- Explain whether an operation installs dependencies, generates files, starts services, changes data, or uses paid providers.
- Starting a complete development environment includes its required services; starting a selected service has clear prerequisite behavior.
- The CLI resolves resource allocation and running-service state itself. Port selection, process discovery, dependency startup, and safe recovery are never delegated back to the coding agent as routine setup instructions.
- Routine inspection and shutdown should remain usable when application dependencies or generated clients are broken. They should not unexpectedly trigger installation or generation.
- Report component results, exit status, executed test counts, and skipped or empty suites consistently. An empty suite is never reported as passing tests.
- Allow focused checks and tests during iteration, with deliberate broader verification when needed.
- Distinguish formatting selected files from formatting the entire repository.
- Keep environment preparation and data-changing actions explicit, with fail-fast validation of required configuration.
- Developers, agents, and CI should use the same underlying operation definitions rather than maintaining independent copies of the workflow.
- Local inspection, shutdown, and recovery should not require the central Sparrow service to be available where they can safely operate from local context.

Established tools such as Bun, uv, Prisma, Ruff, Pyright, Playwright, and Compose can continue to perform their specialist work. The CLI owns their coordinated workflow and results; replacing the Makefile does not mean rewriting those tools.

A small Makefile may remain for temporary compatibility or a few simple entry points. It should not remain a second source of operational logic. The final command names, project configuration format, migration order, and compatibility period are implementation decisions for later.

## Team benefits

| Benefit                    | Expected effect                                                                  |
| -------------------------- | -------------------------------------------------------------------------------- |
| Easier onboarding          | Learn one workflow instead of a collection of scripts and host conventions       |
| Better parallel work       | Clear environment ownership, resource allocation, and cleanup                    |
| Faster reviews             | Open an issue's preview and inspect evidence without deploying shared staging    |
| Better handoffs            | Humans, Claude Code, and Codex continue from the same recorded context           |
| Faster diagnosis           | Locate the failed operation, stage, service, and relevant evidence               |
| More credible verification | See exactly what was checked, against which revision and environment             |
| Clearer releases           | Understand actual platform and voice versions and partial rollout outcomes       |
| Lower operational waste    | Reduce forgotten environments, repeated setup, and unnecessary provider spending |
| Stronger accountability    | Preserve the identities behind requests, approvals, execution, and recovery      |
| Lower agent overhead       | Spend fewer tool calls and tokens rediscovering operational context              |

## Product boundaries

- Preserve V1's task, review, finding, and identity workflows as the foundation.
- Super Sparrow continues to serve one team, divided by projects; WP Agent's customer organizations do not become Sparrow tenants.
- SQLite remains the source of truth for Sparrow's records, with accepted writes and their events committed together. WP Agent retains its own application data and rules.
- Operational capabilities require explicit permissions beyond permission to edit an issue.
- Plans, comments, findings, model output, and tool arguments remain untrusted data. They never become executable instructions merely because Sparrow stores them.
- Sparrow coordinates execution; execution boundaries enforce isolation and resource limits.
- Keep recovery possible when Sparrow is unavailable, especially for production and administrator access.
- Preserve separation between development, previews, staging, and production, even when infrastructure is shared.
- Sparrow holds the team's dev, staging, and production settings for each project and nothing more: it is not a general secrets manager. Do not turn it into a second WP Agent backend or an unrestricted remote shell.
- Choose features around the team's actual needs before attempting a general-purpose infrastructure platform.

## Decisions for later

This document intentionally leaves implementation choices and delivery order open:

- The enforceable isolation model and supported operating systems.
- Where environments and operations execute, and how remote execution is authorized.
- The boundary between Sparrow's CLI and WP Agent's application-specific server tool.
- The relationship to CircleCI, existing deployment workflows, and secret storage.
- Permission scopes, approval rules, and emergency recovery procedures.
- Preview transport, gateway placement, hostname routing, and access policies.
- Retention, artifact storage, environment expiration, and resource budgets.
- Reliability, resource targets, and success measures for the expanded product.

The expansion succeeds when a teammate can open an issue and understand what was requested, where it ran, what was verified, who approved it, and what reached the server, while spending less effort operating the tooling itself.
