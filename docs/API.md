# HTTP API

The Go server's JSON API, for the web interface, the CLI, and scripts. The handlers are in `server/internal/httpserver`, and their JSON shapes are in `views.go`.

## Conventions

- **Authentication.** People in a browser send the session cookie that sign-in sets. The `sparrow` CLI sends `Authorization: Bearer ssc_…`, its own session from [`POST /api/auth/cli/verify`](#sign-in-sessions-people-and-tokens). Agents in cloud sessions and scripts send `Authorization: Bearer ssp_…` (the scheme in any case). CI sends `sst_…`, a service token, to `GET /api/secrets/download` only. A request that changes data and carries an `Origin` header must come from `PUBLIC_URL`. A CLI session is a person's own, so it opens the same endpoints as the browser, except that owner-only endpoints and the secrets of staging and production need a browser (see [Secrets](#secrets)). Owner-only endpoints refuse a CLI session because adding people or granting the staging and production permission would let it reach what only a browser may read.
- **Access.** Each endpoint below is open to everyone signed in, people and agents, unless marked _person_ (no agents), _owner_, or _public_.
- **Errors.** Every error is `{"error": "…"}`. A validation error also names the request field at fault: `{"error": "is required", "field": "title"}` with status 400. Other statuses are 401 (not signed in), 403 (not allowed), 404, 405 with `Allow` (a known path with another method), 409 (a conflict, or a move the record cannot make, such as a finding from `open` to `verified`), and 429 with `Retry-After`.
- **Bodies.** Unknown fields are refused. In a `PATCH`, a field that is absent stays as it is, and `""` or `null` clears an optional field. A required field refuses both.
- **Keys.** Projects are addressed by key (`WP`), issues by key (`WP-142`), rounds by number, and findings by label within their round (`R3`, `r3`, or `3`). Keys are matched without regard to case.
- **Agent header.** A CLI session may send `Sparrow-Agent: claude-code` or `Sparrow-Agent: codex`. The request then acts as that person's agent of that name, created on first use, and every write records the agent instead of the person. Any other value is refused with 400 and `field` set to `Sparrow-Agent`. The header means nothing with a cookie or an `ssp_` token. The server cannot check that the CLI really runs inside that agent; the verified part is the person's sign-in. The CLI sets the header itself when it detects Claude Code or Codex. A CLI session without the header acts as the person. `Sparrow-Model` names the model an agent runs, as before.
- **Times** are RFC 3339 in UTC. Unset IDs and text are `""`, and unset times are `null`.

## Sign-in, sessions, people, and tokens

| Method and path                       | Access | What it does                                                                                                                                                      |
| ------------------------------------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/health`                     | public | `{"status": "ok"}` when the database answers                                                                                                                      |
| `POST /api/auth/code`                 | public | `{email}`; emails a code to an active teammate, with the same answer either way                                                                                   |
| `POST /api/auth/verify`               | public | `{email, code}`; sets the session cookie and returns `{actor}`. Ten checks per IP an hour                                                                         |
| `POST /api/auth/cli/verify`           | public | `{email, code, device_label}`; signs the CLI in. Returns `{token, session, actor}`, where `token` is `ssc_…` and is shown only here. Same rate limits as `verify` |
| `POST /api/auth/sign-out`             | person | Ends this browser's session, or the CLI session that sent the request                                                                                             |
| `GET /api/me`                         |        | `{actor}`, the caller                                                                                                                                             |
| `PATCH /api/me`                       | person | `{display_name?, avatar_id?}`; changes only the signed-in person. Empty or null avatar restores initials. Returns `{actor}`                                       |
| `GET /api/avatars`                    | person | `{avatars: [{id, name, url}]}`; built-in avatar choices accepted by profile updates                                                                               |
| `GET /api/sessions`                   | person | The caller's signed-in devices, with `current` marking this one. Each has `client`, `browser` or `cli`                                                            |
| `DELETE /api/sessions`                | person | Signs out everywhere                                                                                                                                              |
| `DELETE /api/sessions/{id}`           | person | Signs out one device                                                                                                                                              |
| `GET /api/people`                     |        | Every actor: people, agents, and the system. Agents get no email addresses. People also carry `can_manage_environments`                                           |
| `POST /api/people`                    | owner  | `{display_name, email, role}`; adds and emails a teammate. Returns `{actor, invitation_sent}`                                                                     |
| `POST /api/people/{id}/disable`       | owner  | Disables a person and their agents, and ends their sessions and tokens. Their personal dev secrets are deleted                                                    |
| `PATCH /api/people/{id}/environments` | owner  | `{can_manage_environments}`, required; returns `{person}`. Only people have it; owners can manage whatever it says                                                |
| `GET /api/tokens`                     | person | The caller's agent tokens; `?all=true` lists everyone's, for owners                                                                                               |
| `POST /api/tokens`                    | person | `{agent_name, label}`, where `agent_name` is `claude-code` or `codex`; returns `{token, secret}`. The secret is shown only here                                   |
| `POST /api/tokens/{id}/renew`         | person | Another 90 days, for the token's own person                                                                                                                       |
| `DELETE /api/tokens/{id}`             | person | Revokes a token; owners can revoke anyone's                                                                                                                       |

## Projects, milestones, and labels

| Method and path                         | Access | What it does                                                             |
| --------------------------------------- | ------ | ------------------------------------------------------------------------ |
| `GET /api/projects`                     |        | `{projects}` in key order                                                |
| `POST /api/projects`                    | person | `{key, name}`                                                            |
| `GET /api/projects/{key}`               |        | `{project}`. A project carries `secrets_enabled`                         |
| `PATCH /api/projects/{key}`             | person | `{name}`; the key never changes                                          |
| `DELETE /api/projects/{key}`            | owner  | Deletes the project and everything in it; its history stays              |
| `GET /api/projects/{key}/milestones`    |        | `{milestones}` in position order                                         |
| `POST /api/projects/{key}/milestones`   | person | `{name}`; returns `{milestone}` with calculated status                   |
| `PATCH /api/milestones/{id}`            | person | `{name?, position?}`; returns `{milestone}`                              |
| `GET /api/projects/{key}/labels`        |        | `{labels}` in name order                                                 |
| `POST /api/projects/{key}/labels`       |        | `{name, color?}`; color is a six-digit hexadecimal value                 |
| `PATCH /api/projects/{key}/labels/{id}` |        | Supplied `name` and/or `color`; returns `{label}`. Emits `label.updated` |

Milestone status is read-only and calculated from its assigned full issues on every read: `planned` when empty or all issues are Backlog/Todo; `done` when every issue is Done/Canceled; otherwise `active` when any issue has started or finished. Reopening or reassigning issues recalculates status automatically. Subissues do not determine milestone status. Milestone create/update requests reject `status`.

Project responses include `slug`, generated once from the project name and kept on rename. The SPA uses `/{slug}/issues` and `/{slug}/milestones`; HTTP API project endpoints continue to use the project key.

## Issues and plans

| Method and path                      | What it does                                                                                                                                                                                                                                                                             |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/projects/{key}/issues`     | `{issues, next_before}`, newest first. Filters: `?status=todo,in_progress`, `?milestone=<id>`, `?assignee=<id>`. Pages: `?limit=` (1 to 200, 50 by default) and `?before=<next_before>`, which is `null` on the last page                                                                |
| `POST /api/projects/{key}/issues`    | `{title, status, priority?, plan?, parent_issue_id?, milestone_id?, assignee_actor_id?, branch_name?, pull_request_url?, label_ids?}`                                                                                                                                                    |
| `GET /api/issues/{key}`              | `{issue}` with its `plan` and `checklist`                                                                                                                                                                                                                                                |
| `GET /api/issues/for-branch?branch=` | The issue that records the branch, or whose key the branch name contains                                                                                                                                                                                                                 |
| `PATCH /api/issues/{key}`            | Any of the creation fields plus `status`; `label_ids` replaces the set. A new `plan` needs the `plan_revision` it was edited from, and is refused with 409 when the plan has changed since. All of it applies in one transaction                                                         |
| `PATCH /api/issues/{key}/checklist`  | `{items: [{index, checked, text}]}`; ticks plan checklist items by index without rewriting the rest of the plan. `text` is the item's text as the caller read it; when the plan was edited and that index now holds another item, nothing changes and the 409 says where the item is now |

Priority is `urgent`, `high`, `medium`, `low`, or an empty string for no priority. `PATCH` accepts `priority: null` to clear it.

Issue statuses are `backlog`, `todo`, `in_progress`, `in_review`, `done`, and `canceled`. Lists leave out `plan` and `checklist`, and every issue carries `checklist_progress: {done, total}` and `plan_revision`, which goes up with every change to the plan, ticked checklist items included.

## Subissues

Subissues are lightweight children, separate from full issues and their existing `parent_issue_id` links. Their fields are title, Markdown description, ID, status, labels (`label_ids`, from the parent issue's project), and creator (`created_by_actor_id`). They have no reviews, findings, comments, assignees, priorities, milestones, branches, pull requests, or nested children. Review and discussion endpoints accept full issues only.

| Method and path                          | What it does                                                                                                                                                                                  |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/issues/{key}/subissues`        | `{subissues}` in ascending number order, including descriptions; an empty list is `[]`                                                                                                        |
| `POST /api/issues/{key}/subissues`       | `{title, description?, status, label_ids?}`; returns `{subissue}` with 201                                                                                                                    |
| `PATCH /api/issues/{key}/subissues/{id}` | Supplied `title`, `description`, `status`, and/or `label_ids` (replaces the set); returns `{subissue}`. Description edits require `description_revision`; a stale edit returns 409 atomically |

Each subissue returns `id` (UUID), `key` (such as `WP-142.1`), `issue_id`, `title`, `description`, `description_revision`, and `status`. Numbers increase within the parent without consuming project issue numbers. Title is required (up to 200 characters), description supports up to 100,000 characters, and status is required on creation and uses the issue statuses. `description: null` clears the description when paired with its revision; null titles and statuses are rejected. Omitted update fields are preserved. The subissue ID must belong to the issue in the path; otherwise the server returns 404.

People and agents use the same authenticated endpoints. Actor fields are rejected; the server derives identity from the session or token. Each committed creation or edit emits `issue.subissue_created` or `issue.subissue_updated` on the parent issue, with the subissue ID and number in its payload. These events appear in the parent history and event stream. The web interface shows subissues below the description and instead of review rounds in the sidebar, with creation available from either list. Clicking an existing subissue opens a right-side drawer: titles save on blur, statuses save immediately, and Markdown descriptions autosave with revision checks and recoverable drafts. MCP exposes `list_subissues`, `create_subissue`, `update_subissue`, and `set_labels`; the CLI can call them through `tool call`. Existing CLI issue commands operate on full issues. See [MCP.md](MCP.md) for tool arguments, pagination, and safe retries.

## Reviews and findings

| Method and path                                            | What it does                                                                                                                                  |
| ---------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/issues/{key}/reviews`                            | `{rounds}` in order                                                                                                                           |
| `POST /api/issues/{key}/reviews`                           | `{plan_coverage?}`; starts the next numbered review round, with at most one in progress                                                       |
| `GET /api/issues/{key}/reviews/{round}`                    | `{round, findings}`, most severe first                                                                                                        |
| `POST /api/issues/{key}/reviews/{round}/finish`            | `{summary}`                                                                                                                                   |
| `POST /api/issues/{key}/reviews/{round}/findings`          | `{severity, title, problem, fix, evidence?, file_path?, line?}`; severity is `blocking`, `should_fix`, or `nit`                               |
| `GET /api/issues/{key}/findings`                           | Every round's findings. `?status=open,fix` narrows them; duplicates are left out unless `?include_duplicates=true`                            |
| `GET /api/projects/{key}/findings`                         | `{findings, issues}`: every issue's findings, newest issue first, filtered like the issue's list, with a summary of each issue they belong to |
| `GET /api/issues/{key}/reviews/{round}/findings/{finding}` | `{finding}`                                                                                                                                   |
| `POST …/findings/{finding}/status`                         | `{status, reason?}`; returns `{finding, follow_up_issue}`                                                                                     |
| `PUT …/findings/{finding}/duplicate`                       | `{duplicate_of_finding_id}`, a finding on the same issue, or `""` to clear                                                                    |
| `POST /api/issues/{key}/fix-summary`                       | `{summary, findings: [{round, finding}]}`; marks at least one finding fixed and comments, all at once                                         |

Every finding carries `issue_key` and `comment_count`, and every issue carries `unresolved_findings`, which counts its findings that are `open`, `fix`, or `fixed`, duplicates left out, and `comment_count`, which counts the comments on the issue itself.

Reviews, findings, and fix summaries have no commit fields. An empty object is enough to start a review round. Retired commit fields are rejected as unknown fields; agents do not need to create a Git commit to report findings or fixes. The CLI uses `review start KEY [--coverage TEXT]` and `fix-summary KEY --summary TEXT --findings 1/R1`.

A finding moves `open` → `fix`, `wontfix` (needs a `reason`), or `later`, and only a person makes those decisions or withdraws one back to `open`. Then `fix` → `fixed` → `verified`, and `fixed` → `fix` when a fix did not hold; agents can make these moves. Deciding `later` creates a follow-up issue in the backlog and returns it as `follow_up_issue`.

## Comments and history

| Method and path                             | What it does                                                                                                                           |
| ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`, `POST /api/issues/{key}/comments`    | `{comments}` oldest first; post `{body, parent_comment_id?}` to add one                                                                |
| `GET`, `POST …/findings/{finding}/comments` | The same for a finding                                                                                                                 |
| `PATCH /api/comments/{id}`                  | `{body}`; only by its author                                                                                                           |
| `GET /api/issues/{key}/events`              | `{events, next_after}`: the issue's history, including its rounds, findings, and comments. Pages with `?after` and `?limit`            |
| `GET /api/events`                           | Every event, oldest first. Filters: `?project=WP`, `?target_type=` with `?target_id=`, `?actor=<id>`. Pages with `?after` and `?limit` |
| `GET /api/events/stream`                    | Server-Sent Events of new events (see below)                                                                                           |

An event has `sequence`, `kind` (such as `finding.status_changed`), `target_type`, `target_id`, `project_id`, `actor_id`, `reported_model`, `payload`, and `created_at`. Agents see only project events, never those about people, sessions, or tokens.

### Live updates

`GET /api/events/stream?project=WP` sends each new event as:

```
id: 1234
event: change
data: {"sequence":1234,"kind":"comment.added",…}
```

The browser's `EventSource` reconnects by itself and sends `Last-Event-ID`, and the stream then sends everything it missed. A new connection without `Last-Event-ID` or `?after` receives only events made after it connected. A comment line every 25 seconds keeps proxies from closing the connection.

## Secrets

A project keeps settings in four configs: `dev`, `dev-defaults`, `staging`, and `production`. Secrets are off for a project until an owner turns them on, and every endpoint below except enabling and the overview answers 409 until then. Names are `UPPER_SNAKE_CASE` (`^[A-Z_][A-Z0-9_]{0,127}$`), and a value is any UTF-8 text up to 64 KiB. Values are encrypted on the server; no endpoint, event, or error message other than the ones that return a value shows one.

- `dev` is the caller's own dev: the team's `dev-defaults` with the caller's overrides on top. An agent's `dev` is its owner's.
- Someone _can manage_ when they are an owner, or a member whose `can_manage_environments` an owner turned on. Reading or changing the values of `staging` and `production`, writing `dev-defaults`, editing required keys, and managing service tokens need that **and a browser session**. A CLI session, an agent, and an agent token are refused, so nothing on a developer's machine reaches those values. Everyone signed in sees the key names of every config.
- Everyone signed in can read `dev` and `dev-defaults`. A person changes their own `dev` overrides from a browser or the CLI; an agent never changes secrets.

| Method and path                                   | Access | What it does                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------------------------------------------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST /api/projects/{key}/secrets/enable`         | owner  | Turns secrets on for the project and returns `{project}`. Turning them on again changes nothing                                                                                                                                                                                                                                                                                                                                                                                                 |
| `GET /api/projects/{key}/secrets`                 |        | `{enabled, can_manage, is_owner, configs}`. `configs` is `null` while secrets are off, otherwise each config's `{secrets, required, missing}`                                                                                                                                                                                                                                                                                                                                                   |
| `GET …/secrets/{config}/values/{name}`            |        | `{name, value, version, source?}`; one value, recorded in the access log as a reveal. In `dev`, `source` is `override` or `default`                                                                                                                                                                                                                                                                                                                                                             |
| `PATCH …/secrets/{config}`                        | person | `{set: {NAME: value}, delete: [NAME]}`; changes up to 500 keys at once, all or none, and returns the config's `{secrets, required, missing}`. In `dev` it changes only the caller's overrides; deleting one returns the key to its default, and deleting a key that already uses the default changes nothing. Names that change how programs start, such as `PATH`, `LD_*`, `DYLD_*`, `NODE_OPTIONS`, `GIT_*`, and `SPARROW_*`, are refused, since `sparrow run` puts values in the environment |
| `GET …/secrets/{config}/history/{name}`           |        | `{versions}`, newest first, each `{version, deleted, actor_id, created_at}`, never a value. In `dev` it is the caller's own overrides                                                                                                                                                                                                                                                                                                                                                           |
| `POST …/secrets/{config}/history/{name}/restore`  | person | `{version}`, an earlier version; makes it current as a new version and returns the config's listing. Restoring a version that deleted the key deletes it again                                                                                                                                                                                                                                                                                                                                  |
| `PUT …/secrets/{config}/required`                 | person | `{names}`; replaces the required keys of `dev`, `staging`, or `production` and returns the config's listing. `dev-defaults` shares the list of `dev`                                                                                                                                                                                                                                                                                                                                            |
| `GET …/secrets/{config}/download?format=`         |        | The whole config, recorded in the access log as a download. `format` is required                                                                                                                                                                                                                                                                                                                                                                                                                |
| `GET …/secrets/access-log?config=&limit=&cursor=` | person | `{entries, next_cursor}`, newest first, each `{id, config, action, names, actor_id, service_token_id, created_at}`. `action` is `reveal` or `download`; exactly one of the two IDs is set, the other is `null`. Owners read the log of a team config; everyone else reads only the log of their own `dev`                                                                                                                                                                                       |
| `GET /api/projects/{key}/service-tokens`          | person | `{tokens}`, newest first, including revoked and expired ones. Each is `{id, config, label, display_hint, created_by, last_used_at, expires_at, revoked_at, created_at}`                                                                                                                                                                                                                                                                                                                         |
| `POST /api/projects/{key}/service-tokens`         | person | `{config, label}`, where `config` is `staging` or `production`; returns `{token, secret}` with 201. The secret starts with `sst_` and is shown only here                                                                                                                                                                                                                                                                                                                                        |
| `POST /api/service-tokens/{id}/renew`             | person | Expires a year from now; returns `{token}`. A revoked token returns 409                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `DELETE /api/service-tokens/{id}`                 | person | Revokes a token and answers 204. Revoking it again changes nothing                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `GET /api/secrets/download?format=`               | public | For CI. `Authorization: Bearer sst_…` names the project and config; the response is that whole config. A wrong, revoked, or expired token is 401, and requests are rate-limited per client address. A token stops working when the person who made it is disabled or loses the staging and production permission. Sets the token's `last_used_at` and writes the access log                                                                                                                     |

Each config in a listing has `secrets`, `required`, and `missing`. `secrets` is `[{name, version, updated_at, updated_by}]`, and in `dev` each entry also has `source`, `default` or `override`. `required` is the config's required key names, and `missing` is the required names with no value.

`{config}` is one of the four names above. `dev` in `values`, `download`, and `history` always means the caller's, so nobody can read another person's overrides.

**Downloads.** `format=json` returns one object of names to values. `format=env` returns `text/plain` lines like `NAME='value'`, and is refused with 400 when a value has a single quote or a line break. `format=shell` returns `export NAME='value'` lines, with each single quote written as `'\''`. Responses are sent with `Cache-Control: no-store`.

**Service tokens** are for CI. Each reads one project's `staging` or `production` config, nothing else, and cannot write. They last one year, can be renewed, and are stored only as a hash. The Secrets page marks a token that expires within 14 days.

**History and events.** Every change keeps the earlier value as a version, forever. Changes to `dev-defaults`, `staging`, and `production`, required keys, service tokens, turning secrets on, and a person's staging and production permission write events with names and never values. Changes to a person's own `dev` write events that only that person sees in the event list and stream, so nobody else learns which overrides they have.

## Agents

`POST /mcp` is the MCP endpoint. It takes an agent token or a signed-in CLI session: with a CLI session and no `Sparrow-Agent` header the tools run as the person, and with the header as that person's agent. A browser cookie is refused with 403. See [MCP.md](MCP.md) and [DEPLOYMENT.md](DEPLOYMENT.md#connecting-agents). `POST /api/webhooks/github` receives GitHub webhooks when the webhook is set up. It answers deliveries without a well-formed signature, and events other than `ping` and `pull_request`, before reading their body, and takes bodies up to 1 MiB.

## CLI downloads

`GET /cli/{file}` is public and serves the latest signed CLI release from the server's `CLI_DIR`: `manifest.json`, `manifest.json.sig`, and `sparrow-{linux,darwin,windows}-{amd64,arm64}`, with `.exe` on Windows. Any other name, an unset `CLI_DIR`, or a missing file is a 404. The CLI checks the manifest's signature itself, so the server only hands the files out. See [DEPLOYMENT.md](DEPLOYMENT.md#cli-releases).
