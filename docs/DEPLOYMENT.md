# Deployment

Super Sparrow runs as one service behind the existing reverse proxy, which terminates HTTPS. The server serves the API, the MCP endpoint, the event stream, and the built frontend on one address.

## Settings

Every setting is required, and the server stops and names any that is missing. Start from `.env.example`. On a server:

- `SPARROW_PUBLIC_URL` is the HTTPS address people open, such as `https://sparrow.example.com`. Over HTTPS the session cookie is `Secure` and `__Host-` prefixed.
- `SPARROW_SIGN_IN_CODE_KEY` is a random secret of at least 32 characters, such as the output of `openssl rand -hex 32`. Sign-in codes are hashed with it, so keep it out of the database's backups.
- `SPARROW_TRUSTED_PROXIES` lists the reverse proxy's address, so per-IP rate limits see the client's address from `X-Forwarded-For`.
- `SPARROW_WEB_DIR` is the built frontend's folder: `web/dist/client` after `make build-web`, or `/srv/web` in the Docker image.
- `SPARROW_BACKUPS=enabled`, with the backup settings below.
- `SPARROW_GITHUB_WEBHOOK_SECRET` is the webhook's secret, or `none`.

## Run it with Docker Compose

1. Copy `.env.example` to `deploy/sparrow.env` and fill it in. Compose sets the listen address, database path, and frontend folder, which belong to the image, so leave those three out.
2. `cd deploy && SPARROW_VERSION=v0.2.0 docker compose up -d --build`
3. `docker compose exec sparrow sparrow create-owner --name "Your Name" --email you@example.com` adds the first owner.

The database lives in the `sparrow-data` volume. The container listens on `127.0.0.1:8090` only, for the reverse proxy.

## Run it with systemd

1. Download `sparrow-<version>-linux-amd64` and `sparrow-<version>-web.tar.gz` from the GitHub Release, or build both with `make build`. Install the binary at `/usr/local/bin/sparrow`, and unpack the frontend into `/srv/sparrow/web` (from a build, copy `web/dist/client` there).
2. Put the settings in `/etc/sparrow/sparrow.env`, readable by root only, with `SPARROW_DATABASE_PATH=/var/lib/sparrow/sparrow.db` and `SPARROW_WEB_DIR=/srv/sparrow/web`.
3. Copy `deploy/sparrow.service` to `/etc/systemd/system/`, then run `systemctl enable --now sparrow`.
4. Run `create-owner` once with the same settings: `sudo systemd-run --pty -p DynamicUser=yes -p StateDirectory=sparrow -p EnvironmentFile=/etc/sparrow/sparrow.env /usr/local/bin/sparrow create-owner --name "Your Name" --email you@example.com`.

## Reverse proxy

Forward everything to the server, and turn off response buffering for `/api/events/stream`, which stays open. The server already sends `X-Accel-Buffering: no` for nginx. For Caddy:

```
sparrow.example.com {
	reverse_proxy 127.0.0.1:8090 {
		flush_interval -1
	}
}
```

## Backups

Two jobs run inside the server once `SPARROW_BACKUPS=enabled`. Each takes a consistent copy with `VACUUM INTO`, compresses it, encrypts it with [age](https://age-encryption.org), and uploads it to R2:

- The **sync** runs when the server starts and then every hour, replacing `sync/sparrow.db.gz.age`. R2 replaces the old copy only once the new upload has finished.
- The **daily backup** runs at `SPARROW_BACKUP_DAILY_TIME` (UTC) as `daily/sparrow-YYYY-MM-DD.db.gz.age`, then deletes dated copies older than `SPARROW_BACKUP_KEEP_DAYS`.

Each run is recorded, failures are logged, and `GET /api/backups` shows the last run and last success of each job.

To set them up:

1. Make a key pair away from the server: `age-keygen -o sparrow-backup.key`. It prints the public key, which starts with `age1`. Store the private key file in the team's password manager. The server never holds it, so a stolen server cannot read its backups.
2. Create an R2 bucket, and an R2 API token with Object Read & Write on that bucket only.
3. Set `SPARROW_BACKUP_AGE_RECIPIENT` to the public key, `SPARROW_R2_ACCOUNT_ID`, `SPARROW_R2_ACCESS_KEY_ID`, `SPARROW_R2_SECRET_ACCESS_KEY`, `SPARROW_R2_BUCKET`, `SPARROW_BACKUP_DAILY_TIME` (such as `03:30`), and `SPARROW_BACKUP_KEEP_DAYS` (`30` on staging).

## Restoring

`sparrow restore` needs only the four `SPARROW_R2_` settings and the private key file, so it runs on a fresh machine before the server is set up there.

1. Export the four R2 settings in the shell.
2. `sparrow restore --list` shows the copies.
3. `sparrow restore --copy sync --identity sparrow-backup.key --database /var/lib/sparrow/sparrow.db` restores the hourly copy. Use `--copy daily/2026-03-01` for a dated copy. The command decrypts the copy, runs `PRAGMA integrity_check`, and only then writes the database file. It refuses to overwrite an existing database, so move a damaged one aside first.
4. Start the server. It applies any newer migrations at startup.

A sync loses at most the last hour of changes. Restore a daily copy to undo a mistake that the sync has already copied, such as a bad delete.

## GitHub webhook

The webhook marks an issue done when its pull request merges. It finds the issue by the pull request's URL, or else by the branch name, as in `feat/wp-142-meta-comments`.

1. Set `SPARROW_GITHUB_WEBHOOK_SECRET` to a random string of at least 20 characters, such as the output of `openssl rand -hex 32`.
2. In the repository's settings, add a webhook with the payload URL `https://sparrow.example.com/api/webhooks/github`, content type `application/json`, the same secret, and the "Pull requests" event.

The server refuses any delivery whose `X-Hub-Signature-256` does not match. The change is recorded as the system actor.

## Importing from Linear

On the server, with the server's settings and a Linear personal API key in `SPARROW_LINEAR_API_KEY`:

`sparrow import-linear --project "Version 01"`

It imports the project, its milestones, its issues with their Linear numbers, labels, parents, and assignees matched by email, and every comment, with the original author and time written at its top. A comment in the review round format below also becomes a finished review round with its findings. The import is written as the system actor, and running it again imports only what is new. Pass `--team KEY` when the project belongs to several Linear teams. The command prints a warning for everything it skipped.

The review round format the importer reads is below. Labels can be bold or plain, findings can be headings or bold lines, and fields can come in any order. A round without a commit is imported as a comment only.

```md
## Review round 2

Reviewed: a1b2c3d..e4f5a6b (includes uncommitted changes)
Plan coverage: Steps 1 to 3.

### R1 [Blocking] Text can arrive before its file

**Problem:** The worker sends whichever row it claims first.
**Fix:** Send both from one job.
**Evidence:** `apps/api/src/staff-reply.ts:189`
**Status:** fixed

### Summary

One blocking finding.
```

## Connecting agents

Each person creates a token per agent and machine in settings. A repository commits one MCP configuration that reads the token from `SPARROW_TOKEN`, so no token is committed.

Claude Code reads `.mcp.json` at the repository root:

```json
{
  "mcpServers": {
    "super-sparrow": {
      "type": "http",
      "url": "https://sparrow.example.com/mcp",
      "headers": { "Authorization": "Bearer ${SPARROW_TOKEN}" }
    }
  }
}
```

Codex reads `~/.codex/config.toml`:

```toml
[mcp_servers.super-sparrow]
url = "https://sparrow.example.com/mcp"
bearer_token_env_var = "SPARROW_TOKEN"
```

An agent may also send `Sparrow-Model: <model>` to record the model it runs on its changes. The server records the value as reported; it cannot verify it.

Scripts and agents without MCP use the CLI. It runs the same operations through the HTTP API with `SPARROW_URL` and `SPARROW_TOKEN`, and `sparrow help` lists its commands.
