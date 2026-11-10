# HTTP API

The Go server's JSON API, for the web interface, the CLI, and scripts. The handlers are in `server/internal/httpserver`, and their JSON shapes are in `views.go`.

## Conventions

- **Authentication.** People send the session cookie that sign-in sets. Agents and scripts send `Authorization: Bearer ssp_…` (the scheme in any case). A request that changes data and carries an `Origin` header must come from `PUBLIC_URL`.
- **Access.** Each endpoint below is open to everyone signed in, people and agents, unless marked _person_ (no agents), _owner_, or _public_.
- **Errors.** Every error is `{"error": "…"}`. A validation error also names the request field at fault: `{"error": "is required", "field": "title"}` with status 400. Other statuses are 401 (not signed in), 403 (not allowed), 404, 405 with `Allow` (a known path with another method), 409 (a conflict, or a move the record cannot make, such as a finding from `open` to `verified`), and 429 with `Retry-After`.
- **Bodies.** Unknown fields are refused. In a `PATCH`, a field that is absent stays as it is, and `""` or `null` clears an optional field. A required field refuses both.
- **Keys.** Projects are addressed by key (`WP`), issues by key (`WP-142`), rounds by number, and findings by label within their round (`R3`, `r3`, or `3`). Keys are matched without regard to case.
- **Times** are RFC 3339 in UTC. Unset IDs and text are `""`, and unset times are `null`.

## Sign-in, sessions, people, and tokens

| Method and path                 | Access | What it does                                                                                                                |
| ------------------------------- | ------ | --------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/health`               | public | `{"status": "ok"}` when the database answers                                                                                |
| `POST /api/auth/code`           | public | `{email}`; emails a code to an active teammate, with the same answer either way                                             |
| `POST /api/auth/verify`         | public | `{email, code}`; sets the session cookie and returns `{actor}`. Ten checks per IP an hour                                   |
| `POST /api/auth/sign-out`       | person | Ends this browser's session                                                                                                 |
| `GET /api/me`                   |        | `{actor}`, the caller                                                                                                       |
| `PATCH /api/me`                 | person | `{display_name?, avatar_id?}`; changes only the signed-in person. Empty or null avatar restores initials. Returns `{actor}` |
| `GET /api/avatars`              | person | `{avatars: [{id, name, url}]}`; built-in avatar choices accepted by profile updates                                         |
| `GET /api/sessions`             | person | The caller's signed-in devices, with `current` marking this one                                                             |
| `DELETE /api/sessions`          | person | Signs out everywhere                                                                                                        |
| `DELETE /api/sessions/{id}`     | person | Signs out one device                                                                                                        |
| `GET /api/people`               |        | Every actor: people, agents, and the system. Agents get no email addresses                                                  |
| `POST /api/people`              | owner  | `{display_name, email, role}`; adds and emails a teammate. Returns `{actor, invitation_sent}`                               |
| `POST /api/people/{id}/disable` | owner  | Disables a person and their agents, and ends their sessions and tokens                                                      |
| `GET /api/tokens`               | person | The caller's agent tokens; `?all=true` lists everyone's, for owners                                                         |
| `POST /api/tokens`              | person | `{agent_name, label}`; returns `{token, secret}`. The secret is shown only here                                             |
| `POST /api/tokens/{id}/renew`   | person | Another 90 days, for the token's own person                                                                                 |
| `DELETE /api/tokens/{id}`       | person | Revokes a token; owners can revoke anyone's                                                                                 |

## Projects, milestones, and labels

| Method and path                         | Access | What it does                                                             |
| --------------------------------------- | ------ | ------------------------------------------------------------------------ |
| `GET /api/projects`                     |        | `{projects}` in key order                                                |
| `POST /api/projects`                    | person | `{key, name}`                                                            |
| `GET /api/projects/{key}`               |        | `{project}`                                                              |
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

## Attachments

Files are attached to an issue and shown below its description. They are stored in a dedicated, private Cloudflare R2 bucket (`ATTACHMENTS_R2_BUCKET`), not in the database, and the client uploads straight to R2. The bucket is not backed up, so an object that is lost leaves an attachment row with no file.

| Method and path                       | Access  | What it does                                                                                                                                                                                                              |
| ------------------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST /api/issues/{key}/attachments`  |         | JSON `{filename, content_type, byte_size}`. Creates a pending attachment and returns 201 `{attachment, upload: {url, method: "PUT", headers}}`. Any type up to 100 MB; a video up to 5 GB (413 when larger)               |
| `POST /api/attachments/{id}/complete` | creator | Call after the file is uploaded. The server reads the first bytes of the object to recognize its type and checks its size, then returns 200 `{attachment}`. An object that breaks the rules is deleted along with its row |
| `GET /api/issues/{key}/attachments`   |         | `{attachments}`, oldest first. Pending attachments are not listed                                                                                                                                                         |
| `GET /api/attachments/{id}`           |         | Session or agent token. Answers 302 to a short-lived signed R2 URL. Images, video, audio, and PDF show in the page; every other type, including HTML and SVG, is forced to download                                       |
| `DELETE /api/attachments/{id}`        | person  | Deletes the attachment and its object. The creator can also delete an attachment that is still pending                                                                                                                    |

The client sends the file with the `method` and `headers` of `upload` (the `Content-Type` it declared) to `upload.url`. The upload goes to the bucket's origin, so the bucket needs a CORS rule for the app's origin; see [DEPLOYMENT.md](DEPLOYMENT.md).

Each attachment returns `id`, `project_key`, `issue_key`, `filename`, `content_type`, `byte_size`, `url`, `created_by_actor_id`, and `created_at`. Completed uploads write `attachment.created` and deletions write `attachment.deleted`, on the project.

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

## Agents

`POST /mcp` is the MCP endpoint for agent tokens. See [DEPLOYMENT.md](DEPLOYMENT.md#connecting-agents). `POST /api/webhooks/github` receives GitHub webhooks when the webhook is set up. It answers deliveries without a well-formed signature, and events other than `ping` and `pull_request`, before reading their body, and takes bodies up to 1 MiB.

## Personal keyboard shortcuts

`GET /api/me/shortcuts` returns the signed-in person’s `{ revision, overrides }`. An account with no edits returns revision `0` and an empty overrides object. Agents cannot read or edit personal shortcuts.

`PATCH /api/me/shortcuts` accepts `revision`, `action`, and `binding`. It changes only the authenticated session owner. `binding` is a canonical chord, such as `Meta+Shift+Q`; navigation actions also accept two chords separated by a space, such as `G I`. An empty string disables an action, and `null` restores its built-in binding. The response has the same shape as GET.

Actions and built-in bindings are defined in `server/internal/keyboard/definitions.json`, shared by the server and frontend. `Mod` accepts Ctrl or Meta for in-app accelerators; a global binding requires explicit Ctrl, Alt, or Meta. Modifiers are ordered Ctrl, Alt, Meta (or Mod), Shift. Escape and Tab are reserved. Invalid actions, chords, conflicts, and ambiguous prefixes return 400. Stale revisions return 409. Each successful edit records `actor.shortcuts_updated` in the same transaction.
