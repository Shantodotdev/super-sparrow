# Production performance issues

Recorded on October 31, 2026. The fixes below were implemented locally on November 1, 2026; production measurements still need repeating after deployment. Performance requirements are in [SRS.md](SRS.md#5-non-functional-requirements).

## Reported behavior

- Creating an issue appears to take 3–4 seconds.
- Avatars load slowly, and production feels slow overall.

## Evidence

Read-only inspection of production logs, resource usage, HTTP responses, and the local source found:

| Observation                                        | Result                                                                     |
| -------------------------------------------------- | -------------------------------------------------------------------------- |
| Last two logged issue creation requests            | HTTP 201; server processing took 4 ms and 5 ms                             |
| Other sampled API requests                         | Commonly 0–6 ms of server processing                                       |
| VPS load average                                   | 0.48 over one minute                                                       |
| Available VPS memory                               | Approximately 5.8 GB                                                       |
| Super Sparrow container                            | Approximately 11 MB RAM and negligible CPU at the time sampled             |
| Public health request from the inspecting computer | Approximately 0.68 seconds total, including DNS, connection, and TLS setup |
| Sample avatar, `nerdy-owl-astronomer.png`          | 2,637,088 bytes; 1254 × 1254 pixels                                        |
| Download of that avatar from production            | Approximately 3.40 seconds total; 0.64 seconds to first byte               |
| Avatar directory                                   | Approximately 77 MB across the built-in catalog                            |
| Avatar response cache policy                       | `Cache-Control: no-cache`, requiring revalidation before reuse             |

These are point-in-time samples, not a sustained load test or a browser trace. They show no evidence of VPS resource exhaustion during inspection. Network distance and connection speed contribute to observed latency, but the exact browser interaction reported by the user has not been captured.

## Issue creation waits for broad refetches

In `web/src/queries/issues.ts`, `useCreateIssue` returns `client.invalidateQueries()` from its success callback. The mutation therefore stays pending until the invalidated active queries finish refetching. `web/src/components/new-issue-modal.tsx` awaits that mutation before closing the dialog.

In `web/src/components/app-shell.tsx`, live events also invalidate all queries. A successful write can consequently cause overlapping refreshes from both the mutation and the event stream. Production logs show repeated reads of the current user, people, projects, issues, labels, and milestones after an issue is created.

The loaders in `web/src/queries/resources.ts` add dependencies: several first fetch projects, then fetch resources for each project. `getIssues` reads issues across all projects and follows pagination. Repeated project reads and dependent network round trips can make the interaction feel much slower than the database write itself.

### Proposed work

- Update the relevant TanStack Query caches from the committed issue returned by the server so the dialog can close after the successful write.
- Refresh only the affected resources in the background, rather than awaiting a global refresh.
- Make live-event invalidation selective and avoid duplicate refreshes for the same change.
- Reuse the project cache where appropriate and avoid fetching unrelated projects' data for a single-project view.
- Preserve server-confirmed writes, error handling, actor attribution, and live updates for other teammates.

## Avatars are oversized and require revalidation

`web/src/components/avatar.tsx` displays the PNG avatars at roughly 36 × 36 pixels, with other components using smaller sizes. The source images are much larger and commonly exceed 2 MB each. `web/src/components/profile-modal.tsx` renders the catalog as image choices, so opening it can request many large files.

`server/internal/httpserver/frontend.go` gives long-lived immutable caching only to files under `/assets/`. Avatars live under `/avatars/` and receive `no-cache`. Browsers may store these responses, but must revalidate them before reuse; this does not mean every reuse downloads the full image again.

### Proposed work

- Generate appropriately sized, compressed avatar variants, including a format such as WebP where suitable.
- Size variants for both normal and high-density displays, including the larger profile preview.
- Use content hashes or explicit versions in URLs before applying long-lived immutable caching.
- Update the catalog URLs, avatar component, and asset consistency tests together.
- Consider lazy loading catalog choices and define image dimensions to keep layout stable.

## Implemented fixes and local verification

- Issue creation publishes the committed server response to loaded issue lists and the detail cache. The modal no longer awaits a global refresh. Background reads cannot overwrite the immediate cache update with an earlier in-flight response.
- Live events refresh affected resource families and project caches in batches. The creation event skips an issue already received from the successful POST; activity still refreshes. Connection establishment and reconnection reconcile caches to cover missed changes.
- The project issue list requests only its project's issues, labels, and milestones. Aggregate loaders share the project and issue caches instead of repeating those dependencies.
- The 35 avatars now have 48, 96, and 192 pixel WebP variants totaling 556,432 bytes across all 105 files. Normal avatars and the larger preview use responsive sources; catalog choices use lazy loading and explicit dimensions.
- `/avatars/v1/` receives one-year immutable caching. Treat that directory as immutable: publish changed artwork under a new version and update both the catalog and component URLs. Original PNG URLs remain available for older open clients and as source images.
- Asset tests check every catalog choice's three WebP variants and their byte budgets. The HTTP tests cover versioned and legacy cache headers; the avatar API retains its `{id, name, url}` response shape.

The local Chromium test uses a temporary SQLite database. With 200 ms simulated latency and approximately 1 Mbps download bandwidth, the issue dialog closes and the new issue appears while follow-up API GETs are deliberately held indefinitely. A simulated failed POST leaves the draft available. The same browser run checks live issue edits, finding decisions, actor attribution, persisted profiles, and project isolation.

For `nerdy-owl-astronomer-96.webp`, the browser measured 4,722 image bytes, `image/webp`, and `Cache-Control: public, max-age=31536000, immutable`. Its warm fetch transferred zero network bytes. The original recorded PNG was 2,637,088 bytes. All 105 variant dimensions were checked, and representative 192 pixel images were inspected visually.

Validation commands:

- `make check-go`: passed.
- `GOFLAGS=-json make test-go`: 170 top-level tests and 93 subtests passed; the optional browser test was skipped in this run.
- `make test-web`: 28 tests passed, including six performance regression tests.
- `make check-web`: TypeScript and route generation passed; Prettier reported pre-existing formatting in five unchanged files: `code-input.tsx`, `discussion.tsx`, `lib/format.ts`, the issue detail route, and `signin.tsx`.
- `make build-web` and the separate `TestBrowserSmoke` run with `SPARROW_E2E_CHROME=/usr/bin/google-chrome`: passed, with 84 browser assertions. The smoke script was updated to match the existing sign-in, description editor, and comment controls.

The installed Bun was 1.4.0 rather than the pinned 1.4.4. The browser also logged existing editor inline-style CSP violations; no runtime exceptions were reported. These measurements do not establish production p95 response times or VPS resource usage under load.

## Remaining production verification

- Capture a browser network trace of issue creation to distinguish POST time, follow-up reads, and dialog completion.
- Verify the dialog completes after the successful POST without waiting for unrelated reads, and that failures keep the form available.
- Check that a newly created issue appears immediately and that other signed-in teammates still receive the update.
- Compare cold and warm avatar loads, transferred bytes, request counts, image quality, and cache headers.
- Repeat measurements under a representative slower connection and confirm that VPS CPU, memory, and database timings remain healthy.
- Repeat the relevant checks with the pinned Bun toolchain before release.
