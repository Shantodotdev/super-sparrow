# Secrets and CLI plan

Status: **draft, not final.** This plan changes as the idea is discussed. Nothing is built until it is final.

It covers two connected pieces of work: a secrets feature that replaces Doppler, and a full Sparrow CLI that local agents use through their person's sign-in instead of tokens.

## Goal

Replace Doppler for our team with a secrets feature inside Super Sparrow, so the team keeps one sign-in, one set of permissions, and no extra bill. It covers only what we use, not everything Doppler does.

## What Doppler does for us today

Found in the WP Agent and Super Sparrow repositories:

- **Configs:** `wp-agent` dev and stg, `wp-agent-voice` stg, `super-sparrow` prd, `super-sparrow-recovery` prd. About 60 keys.
- **Local setup:** `make env SLOT=N` runs `infrastructure/scripts/sync-local-env.mjs`, which downloads the dev config as JSON and merges it into `.env`. Values below the `# --- Local overrides` line win.
- **CircleCI:** each context holds one read-only `DOPPLER_TOKEN`. `infrastructure/scripts/doppler-env.sh` loads the named keys a job needs, and `apply-config.sh` runs the deploy script with `doppler run`.
- **GitHub Actions:** Super Sparrow's deploy (`.github/workflows/ci.yml`) and Apply production settings (`configure-production.yml`) read `super-sparrow/prd`.
- **References:** dev uses `${stg.OPENROUTER_API_KEY}`, and the voice config refers to `wp-agent` values, so a value is never copied.

## Sparrow CLI

### Goal

A full `sparrow` CLI that does everything the MCP tools do. Agents on a person's own machine (Claude Code, Codex, others) use the CLI through that person's sign-in, so local work needs no agent tokens. Tokens are only for cloud sessions and CI.

### Sign-in

- `sparrow login` signs a person in with an emailed code, like the web, and stores the session in the operating system's keychain. `sparrow logout` ends it. Like a web session, it lasts 30 days and renews while in use.
- Each CLI sign-in shows in Settings with the person's other signed-in devices, and can be signed out from there.
- Local agents run `sparrow` commands with that session. They need no token, no MCP setup, and no secret in their configuration.

### Install

- **Signed releases.** GitHub Actions builds the CLI for Linux, macOS, and Windows on each version tag and signs the release (its version and each file's checksum) with an Ed25519 key. The key is a secret of a GitHub `release` environment, used only for version tags, and a teammate approves each release run. The Sparrow server never holds it.
- **First install.** A teammate downloads the CLI from the private repository's GitHub Releases with `gh` and checks its checksum. The first download never trusts the Sparrow server.
- **Updates.** The Sparrow server hosts the signed releases, so the CLI always matches the server. `sparrow update` downloads the server's current CLI and replaces itself only when the signature matches a public key built into the CLI. A break-in on the server cannot produce a CLI that passes this check.
- **No downgrades.** The CLI refuses a signed release older than itself, so an old version with a known bug cannot be served again.
- **Key rotation.** The CLI carries a second public key, so a leaked signing key can be replaced without reinstalling everywhere.
- **HTTPS only.** The CLI refuses plain `http` addresses, except `localhost` in development.
- Apple and Windows code signing are not planned: they cost a yearly fee and the CLI is an internal tool.

### Commands

- One command for every MCP tool, grouped by subject: `project`, `issue`, `plan`, `subissue`, `label`, `review`, `finding`, `comment`, `fix-summary`, and `secrets`.
- Each command takes the same inputs as its MCP tool, with the same checks and the same error codes.
- Output is readable text by default and JSON with `--json`, for agents and scripts. Exit statuses are stable.
- `sparrow tool list` and `sparrow tool call` stay, for any tool without its own command.
- Today's CLI (`server/internal/cli`) covers part of this and needs `SPARROW_URL` and `SPARROW_TOKEN`; it grows into the full CLI.

### Which agent did it

- Every write records who made it: the signed-in person, or one of that person's agents.
- Agent names are a fixed list: `claude-code` and `codex`. There is no free-text or "other" name; a new agent is added to the list in code when the team needs it.
- The CLI detects Claude Code and Codex from the environment they run commands in. An agent can also name itself with `--agent` or the `SPARROW_AGENT` environment variable, and its model with `SPARROW_MODEL` as today.
- If the CLI cannot tell which agent is running and none is named, the write is refused. A person running commands themselves passes `--human`, and the write is recorded as them.
- Human-only actions (deciding fix, wontfix, or later, managing people, creating tokens) are allowed in the CLI with `--human`. The team accepts that an agent could pass `--human`; the CLI never passes it for an agent it detected.
- The server cannot verify what the CLI says about who is running it. The person's sign-in is what is verified.

### Cloud sessions and CI

- Cloud sessions use an agent token with the same CLI, or the MCP endpoint, as today.
- The MCP endpoint keeps working for local agents too, with a token, for anyone who prefers it. Local setup docs point at the CLI first.
- CI uses service tokens (below). Tokens are not needed for local work.

## Configs

Secrets are off for a project until an owner turns them on. A project with secrets on has exactly three configs, created automatically. Nobody creates, renames, or deletes configs.

| Config     | Who it belongs to         | Who can use it                                               |
| ---------- | ------------------------- | ------------------------------------------------------------ |
| Dev        | Each person has their own | Only that person and their own agents                        |
| Staging    | The team                  | Owners, and members allowed to manage staging and production |
| Production | The team                  | Owners, and members allowed to manage staging and production |

- Voice settings live in staging and production with the rest. The separate `wp-agent-voice` project goes away.
- Keys are `UPPER_SNAKE_CASE`. Values can be any text, including long one-line values such as a base64 SSH key.

### Personal dev

- Each project has shared **team dev defaults**, such as the OpenRouter key, so nobody copies keys by hand.
- Every person's dev is the team defaults plus their own overrides. An override changes only that person's dev, and a later change to a default reaches everyone who has not overridden that key.
- Owners and members allowed to manage staging and production edit the team defaults. Everyone else can only override keys for themselves.
- Team defaults hold their own values, ideally separate dev keys. Dev never refers to staging, so dev access never exposes a staging value.
- Changing your dev never changes anyone else's.
- Nobody can see anyone else's overrides, owners included.
- Disabling a teammate deletes their overrides right away.
- A person's local agents read that person's dev through their CLI sign-in, so an agent can run `make env` in a worktree. Every agent token of a person can also read it, for cloud sessions. Nobody's agents can read anyone else's dev.

## Permissions

- **Owners** can read and edit staging and production, and manage everyone's permissions.
- **Members** have one setting, set by an owner: **can manage staging and production**.
  - On: they can read and edit both configs and create service tokens for them.
  - Off: they see the key names of both configs, never the values, and cannot edit them.
- **Agents** never read or edit staging or production, whatever their owner's permission. They can read only their owner's dev.
- **Service tokens** read one config (staging or production) and nothing else. They cannot write.
- All of this is checked on the server, never only in the interface.

## Required keys

- Each config has a list of required keys, edited by owners and members allowed to manage staging and production. Team dev defaults have their own list.
- The Secrets page and the MCP key-name tool show which required keys have no value, so a missing setting is caught before a deploy.
- The download endpoint still returns what is there; the deploy scripts keep their own checks.

## Service tokens for CI

- Created by anyone allowed to manage staging and production. Each reads one project's whole staging or production config; the CI scripts already pick the keys each job needs.
- Last one year and can be renewed. Sparrow warns owners 14 days before one expires.
- Shown once, start with `sst_` so secret scanners recognize them, stored only as a hash, expire, and can be revoked.
- CircleCI contexts and GitHub environments hold one token each, replacing `DOPPLER_TOKEN`.

## Reading settings

- **HTTP download:** one endpoint returns a config as JSON, `.env` lines, or shell `export` lines. CI uses `curl`, so it installs nothing new.
- **CLI sign-in:** `make env` uses the person's `sparrow login` session to read their dev. A CLI session reads only its person's dev, never staging or production, because agents on the same machine use it. Staging and production values are revealed only in the web and desktop app.
- **CLI:** `sparrow secrets download`, `sparrow secrets set`, `sparrow secrets import`, and `sparrow run --config dev -- <command>`.
- **MCP:** agents can list key names in staging and production (never values), so they can say which setting is missing. They read their owner's dev through the CLI or the download endpoint.

## Web interface

- A Secrets page in each project that has secrets on, with three tabs: My dev, Staging, Production. Members without the staging and production permission see those tabs with key names only.
- My dev shows each key's value, marked as a team default or your override, with a reset to the default.
- Values are masked. Reveal and copy are one click each.
- Add, edit, and delete keys. Paste a `.env` to add many at once.
- Before saving, show which keys are added, changed, or removed. Production changes also need a confirm step in that dialog; no second person is needed.
- History for each key, with restore of an earlier value.
- Anyone who can use staging or production can download the whole config as a `.env`. Each download goes in the access log.

## Storage and security

- Values are encrypted in SQLite with a key that comes from a server setting, kept out of the database and its backups. The server refuses to start without it.
- Each value's encryption is bound to its project, config, owner, and key name, so a value copied to another row cannot be decrypted.
- Every version is kept forever, encrypted, so a value can be restored.
- Backups stay encrypted with age as today. Restoring secrets also needs the encryption key, which the team keeps in a password manager.

## History and audit

- Every change writes an event with the actor and the key name, never the value.
- Staging and production changes appear in the project's activity feed, names only, for people who can see that config.
- Changes to someone's personal dev are visible only to that person.
- Reveals and downloads (by people, agents, and service tokens) go in an access log, kept forever. Owners see the staging and production log; each person sees their own dev log.

## Super Sparrow's own settings

Super Sparrow stays on the WP Agent staging server, also once it holds WP Agent production secrets. The team accepts that a break-in on that server would reach production secrets.

Super Sparrow cannot load its own settings from itself: if it were down, it could not load the settings needed to fix it. Its production settings move to GitHub's `production` environment secrets, and the recovery keys (backup age key, secrets encryption key) move to a password manager.

## Not building

- Custom configs, config branching, per-key permissions, per-key service tokens.
- References between values. Voice and platform settings now share a config, and dev holds its own copies, so nothing needs them.
- Automatic rotation, dynamic secrets, syncing to other services.

## Later (noted only, not planned)

- **Auto-reload:** an admin panel action that reloads the containers on the VPS after settings change.

## Moving off Doppler

1. Import each Doppler config into Super Sparrow: WP Agent stg and voice stg into Staging, and WP Agent dev into the team dev defaults with its own values in place of the staging references.
2. Store the secrets encryption key and the backup key in the team's password manager, and test a restore once.
3. Switch WP Agent's local setup (`make env`) to read personal dev.
4. Switch CircleCI: `doppler-env.sh` and `apply-config.sh` read from the download endpoint.
5. Move Super Sparrow's deploy to GitHub environment secrets.
6. After the first successful staging deploy from Sparrow, revoke the Doppler tokens, keep an encrypted export offline for a month, and cancel Doppler.

## Build steps

The CLI comes first, so the secrets commands and `make env` reuse its sign-in.

1. Finalize this plan, then add it to `docs/SRS.md` and `docs/PLAN.md`, and update the out-of-scope line in the SRS and the "general secrets manager" line in `docs/V2_FEATURES.md`.
2. CLI: `sparrow login`, CLI sessions, agent detection and naming, a command for every MCP tool, and serving the CLI binary from the server.
3. Secrets server: migration, encryption, store, permission checks, API, events, tests.
4. Secrets web: the Secrets page and the member permission setting.
5. Service tokens, the download endpoint, the MCP key-name tool, and the `sparrow secrets` commands.
6. Doppler import and the move described above.

Existing agent tokens on people's machines keep working; each person decides whether to revoke them.

## Open questions

None yet.

## Decisions so far

- Three fixed configs per project: dev, staging, production. No custom configs.
- Voice settings live in staging and production.
- Dev is personal: changing it affects only you, and nobody else can see it.
- Agents can read their owner's dev.
- One member permission: can manage staging and production (read and edit both).
- Auto-reload of VPS containers comes later and is not planned yet.
- Secrets are off per project until an owner turns them on.
- Personal dev is team dev defaults plus each person's private overrides.
- Members without the staging and production permission see key names only.
- Super Sparrow stays on the WP Agent staging server, also for production secrets.
- Owners and members allowed to manage staging and production edit team dev defaults.
- Dev holds its own values and never refers to staging. No references at all.
- Super Sparrow's own production settings go in GitHub's `production` environment; recovery keys go in a password manager.
- Service tokens read a whole staging or production config.
- People get their dev on their machine through `sparrow login`, a CLI sign-in with an emailed code.
- Every agent token reads its owner's dev automatically.
- Production changes need a confirm step; no second approval.
- Disabling a teammate deletes their dev overrides.
- A CLI sign-in reads only its person's dev.
- Service tokens last one year and can be renewed.
- Old versions and the access log are kept forever.
- Each config has a required-keys list, and Sparrow shows missing ones.
- A full `sparrow` CLI does everything the MCP tools do. Local agents use it through their person's `sparrow login` session, with no tokens.
- Tokens are only for cloud sessions and CI.
- Every write records which agent made it (Claude Code, Codex, or another).
- The CLI detects Claude Code and Codex; a write with no detected or named agent is refused unless the person passes `--human`.
- Human-only actions are allowed in the CLI with `--human`.
- Agent names are only `claude-code` and `codex`, with no free-text names.
- The MCP endpoint keeps working for both local and cloud agents.
- Staging and production changes appear in the activity feed, names only.
- Anyone permitted can download a whole staging or production config as a `.env`.
- A CLI session lasts 30 days and renews while in use.
- The server serves the CLI binary; `sparrow update` keeps it current.
- CLI releases are signed in GitHub Actions with an approved `release` environment; the CLI checks the signature and refuses downgrades.
- The first CLI install comes from the private repository's GitHub Releases.
- Doppler is cancelled after the first successful staging deploy from Sparrow; no side-by-side week.
- Recovery keys live in the team's password manager.
- The CLI is built before secrets.
- Existing local agent tokens are left as they are.
