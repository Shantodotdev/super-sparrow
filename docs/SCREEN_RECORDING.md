# Screen recording

Design for recording the screen onto an issue, Loom style. This is a proposal: no code exists for it yet. Once it is built, [SRS.md](SRS.md), [API.md](API.md) and [DEPLOYMENT.md](DEPLOYMENT.md) become the source of truth and this file should shrink to a pointer.

## Goals

- Anyone can record their screen from an issue and attach it.
- The recording uploads while it is being made, so it is available the moment it stops.
- Members play it in the page without downloading it.
- Nothing is stored on the Go server. Files live in R2.
- Capture feels native in the desktop app: no external tool, no prompt on every use.

## Decisions

- Files go to R2 directly from the client with presigned multipart uploads. The Go server signs and records metadata; it never handles the video bytes.
- Playback is progressive: the server returns a short-lived presigned GET URL and the `<video>` element reads the R2 object with Range requests.
- The desktop app records natively in Rust. The browser recorder (`getDisplayMedia` and `MediaRecorder`) is the fallback for the web page.
- No ffmpeg or transcoding on the server, and no HLS, Mux, or Cloudflare Stream.
- Reliability and native feel decide between options. Implementation effort is not a factor.

## Upload flow

1. The client asks the server to start a recording on an issue. The server checks the session, creates an R2 multipart upload for `attachments/<id>` with a Content-Type from a fixed allowlist, and stores a pending attachment row for the actor.
2. The recorder fills fixed-size parts. R2 requires every part except the last to be the same size and at least 5 MiB; 8 MiB parts at 2.5 Mbps fill roughly every 27 seconds, and 10,000 parts allow about 80 GB, well above the 5 GB video cap.
3. For each full part the client asks the server for a presigned `UploadPart` URL and PUTs the part to R2. The server signs one key, one part number, and a bounded Content-Length, so a client can never write elsewhere or send more than allowed.
4. On stop the client uploads the last part and sends the part list to finish.
5. The server calls `ListParts`, checks the total size against the cap, completes the upload, checks the object with `HeadObject` and a ranged read of its first bytes, then marks the attachment ready and writes its event in one transaction.
6. Resume after a dropped connection or restart uses `ListParts` to find the next part. Abandoned uploads are aborted by an R2 lifecycle rule for incomplete multipart uploads, and a server job removes pending rows that never finished. Deleting an attachment deletes its R2 object.

## Playback

The server authorizes the session and returns a presigned GET URL, or redirects to it. The bucket stays private. The video element streams and seeks with Range requests straight from R2.

## Seekability

Chrome's `MediaRecorder` WebM has no duration or seek index, so scrubbing is poor, and the server cannot remux it. Mitigations:

- The desktop recorder writes fragmented MP4 with a correct duration and seek index, which fixes this at the source.
- In the browser, prefer MP4 where supported (Chrome 130 and later, Safari) and store the duration measured by the client.
- Verify browser fragmented-MP4 seeking in a spike before relying on it. If it falls short, remux in the browser before finishing, or use a post-processing step outside the Go server.

## Desktop app

Native capture and recording in Rust, in `desktop/src-tauri`.

- Linux: the xdg-desktop-portal ScreenCast request with a saved restore token. The picker appears once; later captures read a silent PipeWire stream. On KDE, screenshots can also use `org.kde.KWin.ScreenShot2`, which needs `X-KDE-DBUS-Restricted-Interfaces=org.kde.KWin.ScreenShot2` in `desktop/src-tauri/linux/super-sparrow.desktop` and an installed build.
- macOS: ScreenCaptureKit, with the one-time Screen Recording permission.
- Windows: Windows Graphics Capture.
- Video is H.264, hardware encoded where available, with system and microphone audio, written as fragmented MP4 to a local file first. A background task uploads parts from that file, so a network drop or crash loses nothing.
- The app window is excluded from the capture instead of being hidden.

### Screenshots today

`capture_screen` in `desktop/src-tauri/src/capture.rs` uses `xcap`. On Wayland, `xcap` tries GNOME's private screenshot interface and then falls back to the portal Screenshot request. On KDE that brings up the system screenshot tool and a permission prompt, which interrupts the flow. Moving screenshots to the ScreenCast stream, with the KDE interface as a fast path, removes the prompt after the first grant. This is from reading `xcap`'s source, not from a test run on every desktop.

Linux always shows the compositor's sharing indicator while capturing. That cannot be removed.

## Security

- The actor comes from the session or agent token, never from the request body. Only the actor who started a recording can sign parts or finish it.
- Agents cannot record. Only people start recordings.
- Clients never hold R2 credentials. Presigned URLs are short-lived and scoped to one key and one part.
- Sign and finish requests are validated at the boundary: allowed content type, part number range, declared sizes, and total size cap enforced from `ListParts`.
- Because the server does not see the bytes, content type is checked from the first bytes of the stored object at finish, and the attachment is rejected and its object deleted if it fails.
- Backups currently encrypt attachments with age before upload. Media stored directly in R2 is not covered by that. Decide between bucket-side encryption and another scheme before shipping.

## R2 setup

- Bucket CORS allows `PUT` and `GET` from the app origin and exposes the `ETag` header, because the client needs part ETags.
- A lifecycle rule aborts incomplete multipart uploads after a few days.
- The Go `internal/r2` package gains a presigner for `CreateMultipartUpload`, `UploadPart`, `ListParts`, `CompleteMultipartUpload`, `AbortMultipartUpload`, `HeadObject`, and `GetObject`.

## Open questions

- How media in R2 is encrypted, as above.
- How the SRS wording on content type recognized from the bytes and the SHA-256 of an attachment changes when the server does not read the file. Either the client computes the hash and the server verifies the first bytes, or the hash is dropped for videos.
- Whether browser fragmented-MP4 seeking is good enough or needs a remux step.
- The part size: 5 MiB loses less on a crash, 8 MiB makes fewer requests.

## Research

How other projects do it:

- Cap uploads chunks to S3-compatible storage while recording, then sends a complete call with the chunk list.
- CaptureFlow streams a multipart upload to R2 while recording and finalizes on stop.
- SendRec, a Go and React project, records in the browser, prefers MP4 with a WebM fallback, and uploads with presigned S3 URLs.
- Chromium keeps `MediaRecorder` WebM unseekable (issue 40482588); the common fix is a lossless remux that writes the duration and puts the cues first.
