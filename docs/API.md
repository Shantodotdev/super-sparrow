# HTTP API

The Go server's JSON API, for the web interface, the CLI, and scripts. The handlers are in `server/internal/httpserver`, and their JSON shapes are in `views.go`.

## Conventions

- **Authentication.** People send the session cookie that sign-in sets. Agents and scripts send `Authorization: Bearer ssp_…` (the scheme in any case). A request that changes data and carries an `Origin` header must come from `SPARROW_PUBLIC_URL`.
- **Access.** Each endpoint below is open to everyone signed in, people and agents, unless marked _person_ (no agents), _owner_, or _public_.
- **Errors.** Every error is `{"error": "…"}`. A validation error also names the request field at fault: `{"error": "is required", "field": "title"}` with status 400. Other statuses are 401 (not signed in), 403 (not allowed), 404, 405 with `Allow` (a known path with another method), 409 (a conflict, or a move the record cannot make, such as a finding from `open` to `verified`), and 429 with `Retry-After`.
- **Bodies.** Unknown fields are refused. In a `PATCH`, a field that is absent stays as it is, and `""` or `null` clears an optional field. A required field refuses both.
- **Keys.** Projects are addressed by key (`WP`), issues by key (`WP-142`), rounds by number, and findings by label within their round (`R3`, `r3`, or `3`). Keys are matched without regard to case.
- **Times** are RFC 3339 in UTC. Unset IDs and text are `""`, and unset times are `null`.

## Sign-in, sessions, people, and tokens

| Method and path                 | Access | What it does                                                                                  |
| ------------------------------- | ------ | --------------------------------------------------------------------------------------------- |
| `GET /api/health`               | public | `{"status": "ok"}` when the database answers                                                  |
| `POST /api/auth/code`           | public | `{email}`; emails a code to an active teammate, with the same answer either way               |
| `POST /api/auth/verify`         | public | `{email, code}`; sets the session cookie and returns `{actor}`. Ten checks per IP an hour     |
| `POST /api/auth/sign-out`       | person | Ends this browser's session                                                                   |
| `GET /api/me`                   |        | `{actor}`, the caller                                                                         |
| `GET /api/sessions`             | person | The caller's signed-in devices, with `current` marking this one                               |
| `DELETE /api/sessions`          | person | Signs out everywhere                                                                          |
| `DELETE /api/sessions/{id}`     | person | Signs out one device                                                                          |
| `GET /api/people`               |        | Every actor: people, agents, and the system. Agents get no email addresses                    |
| `POST /api/people`              | owner  | `{display_name, email, role}`; adds and emails a teammate. Returns `{actor, invitation_sent}` |
| `POST /api/people/{id}/disable` | owner  | Disables a person and their agents, and ends their sessions and tokens                        |
| `GET /api/tokens`               | person | The caller's agent tokens; `?all=true` lists everyone's, for owners                           |
| `POST /api/tokens`              | person | `{agent_name, label}`; returns `{token, secret}`. The secret is shown only here               |
| `POST /api/tokens/{id}/renew`   | person | Another 90 days, for the token's own person                                                   |
| `DELETE /api/tokens/{id}`       | person | Revokes a token; owners can revoke anyone's                                                   |

## Projects, milestones, and labels

| Method and path                       | Access | What it does                                                |
| ------------------------------------- | ------ | ----------------------------------------------------------- |
| `GET /api/projects`                   |        | `{projects}` in key order                                   |
| `POST /api/projects`                  | person | `{key, name}`                                               |
| `GET /api/projects/{key}`             |        | `{project}`                                                 |
| `PATCH /api/projects/{key}`           | person | `{name}`; the key never changes                             |
| `DELETE /api/projects/{key}`          | owner  | Deletes the project and everything in it; its history stays |
| `GET /api/projects/{key}/milestones`  |        | `{milestones}` in position order                            |
| `POST /api/projects/{key}/milestones` | person | `{name, status}`: `planned`, `active`, or `done`            |
| `PATCH /api/milestones/{id}`          | person | `{name?, status?, position?}`; returns `{milestone}`        |
| `GET /api/projects/{key}/labels`      |        | `{labels}` in name order                                    |
| `POST /api/projects/{key}/labels`     |        | `{name}`                                                    |

## Issues and plans

| Method and path                      | What it does                                                                                                                                                                                                              |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/projects/{key}/issues`     | `{issues, next_before}`, newest first. Filters: `?status=todo,in_progress`, `?milestone=<id>`, `?assignee=<id>`. Pages: `?limit=` (1 to 200, 50 by default) and `?before=<next_before>`, which is `null` on the last page |
| `POST /api/projects/{key}/issues`    | `{title, status, plan?, parent_issue_id?, milestone_id?, assignee_actor_id?, branch_name?, pull_request_url?, label_ids?}`                                                                                                |
| `GET /api/issues/{key}`              | `{issue}` with its `plan` and `checklist`                                                                                                                                                                                 |
| `GET /api/issues/for-branch?branch=` | The issue that records the branch, or whose key the branch name contains                                                                                                                                                  |
| `PATCH /api/issues/{key}`            | Any of the creation fields plus `status`; `label_ids` replaces the set. All of it applies in one transaction                                                                                                              |
| `PATCH /api/issues/{key}/checklist`  | `{items: [{index, checked}]}`; ticks plan checklist items by index without rewriting the rest of the plan                                                                                                                 |

Issue statuses are `backlog`, `todo`, `in_progress`, `in_review`, `done`, and `canceled`. Lists leave out `plan` and `checklist`, and every issue carries `checklist_progress: {done, total}`.

## Reviews and findings

| Method and path                                            | What it does                                                                                                                     |
| ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/issues/{key}/reviews`                            | `{rounds}` in order                                                                                                              |
| `POST /api/issues/{key}/reviews`                           | `{to_commit, from_commit?, includes_uncommitted?, plan_coverage?}`; without `from_commit`, starts where the previous round ended |
| `GET /api/issues/{key}/reviews/{round}`                    | `{round, findings}`, most severe first                                                                                           |
| `POST /api/issues/{key}/reviews/{round}/finish`            | `{summary}`                                                                                                                      |
| `POST /api/issues/{key}/reviews/{round}/findings`          | `{severity, title, problem, fix, evidence?, file_path?, line?, commit_sha?}`; severity is `blocking`, `should_fix`, or `nit`     |
| `GET /api/issues/{key}/findings`                           | Every round's findings. `?status=open,fix` narrows them; duplicates are left out unless `?include_duplicates=true`               |
| `GET /api/issues/{key}/reviews/{round}/findings/{finding}` | `{finding}`                                                                                                                      |
| `POST …/findings/{finding}/status`                         | `{status, reason?, fixed_in_commit?}`; returns `{finding, follow_up_issue}`                                                      |
| `PUT …/findings/{finding}/duplicate`                       | `{duplicate_of_finding_id}`, a finding on the same issue, or `""` to clear                                                       |
| `POST /api/issues/{key}/fix-summary`                       | `{summary, commit, findings: [{round, finding}]}`; marks at least one finding fixed and comments, all at once                    |

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
| `GET /api/backups`                          | _Person._ `{enabled, jobs: {sync, daily}}`, each with `last_run` and `last_success`                                                    |

An event has `sequence`, `kind` (such as `finding.status_changed`), `target_type`, `target_id`, `project_id`, `actor_id`, `reported_model`, `payload`, and `created_at`. Agents see only project events, never those about people, sessions, or tokens.

### Live updates

`GET /api/events/stream?project=WP` sends each new event as:

```
id: 1234
event: change
data: {"sequence":1234,"kind":"comment.added",…}
```

The browser's `EventSource` reconnects by itself and sends `Last-Event-ID`, and the stream then sends everything it missed. A new connection without `Last-Event-ID` or `?after` receives only events made after it connected. A comment line every 25 seconds keeps proxies from closing the connection.

## Agents

`POST /mcp` is the MCP endpoint for agent tokens. See [DEPLOYMENT.md](DEPLOYMENT.md#connecting-agents). `POST /api/webhooks/github` receives GitHub webhooks when the webhook is set up. It answers deliveries without a well-formed signature, and events other than `ping` and `pull_request`, before reading their body, and takes bodies up to 1 MiB.
