# Production performance issues

Recorded on October 31, 2026. These issues are deferred; this document records observations and proposed work, not completed fixes. Performance requirements are in [SRS.md](SRS.md#5-non-functional-requirements).

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

## Verification when fixes are implemented

- Capture a browser network trace of issue creation to distinguish POST time, follow-up reads, and dialog completion.
- Verify the dialog completes after the successful POST without waiting for unrelated reads, and that failures keep the form available.
- Check that a newly created issue appears immediately and that other signed-in teammates still receive the update.
- Compare cold and warm avatar loads, transferred bytes, request counts, image quality, and cache headers.
- Repeat measurements under a representative slower connection and confirm that VPS CPU, memory, and database timings remain healthy.
- Run the checks and tests required by AGENTS.md for the files changed during implementation. No application tests were run for this documentation-only record.
