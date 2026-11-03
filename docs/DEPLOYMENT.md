# Deployment

Super Sparrow runs as one service behind the existing reverse proxy, which terminates HTTPS. The server serves the API, the MCP endpoint, the event stream, the built frontend, and the signed CLI releases on one address.

## Settings

Set `APP_ENV=production` on a server. Production requires every setting in `.env.example`, and the server refuses to start and names each one that is missing or malformed. Start from `.env.example`. On a server:

- `PUBLIC_URL` is the HTTPS address people open, such as `https://sparrow.example.com`. Over HTTPS the session cookie is `Secure` and `__Host-` prefixed.
- `SIGN_IN_CODE_KEY` is a random secret of at least 32 characters, such as the output of `openssl rand -hex 32`. Sign-in codes are hashed with it, so keep it out of the database's backups.
- `TRUSTED_PROXIES` lists the reverse proxy's address, so per-IP rate limits see the client's address from `X-Forwarded-For`.
- `SECRETS_ENCRYPTION_KEY` encrypts every stored secret value. It is required in every environment, development too, and must be 32 random bytes in base64, such as the output of `openssl rand -base64 32`. Save it in the team's password manager when you create it, and never change it: values sealed with another key cannot be decrypted, so a changed or lost key loses the secrets. It stays out of the database and its backups, so a backup restores secrets only together with this key. The server refuses to start without it.
- `WEB_DIR` is the built frontend's folder: `web/dist/client` after `make build-web`, or `/srv/web` in the Docker image.
- `CLI_DIR` is the folder of the signed CLI release the server hands out: the files [CLI releases](#cli-releases) lists. It is `/srv/cli` in the Docker image. It must exist, but may be empty before the first signed release; the server then answers 404 for the CLI files.
- The backup settings below.
- `GITHUB_WEBHOOK_SECRET` is the webhook's secret.

## Run it with Docker Compose

1. Copy `.env.example` to `deploy/sparrow.env` and fill it in. Compose sets `APP_ENV=production`, the listen address, the database path, the frontend folder, and the CLI folder, which belong to the image, so leave those five out.
2. `cd deploy && VERSION=v0.2.0 docker compose up -d --build`
3. `docker compose exec sparrow sparrow create-owner --name "Your Name" --email you@example.com` adds the first owner.

The database lives in the `sparrow-data` volume. The container listens on `127.0.0.1:8090` only, for the reverse proxy.

## Production on wpstaging

Super Sparrow runs at `https://sparrow.premiumcommerce.site` on WP Agent's staging VPS.
This is Super Sparrow production, with `APP_ENV=production`, its own SQLite volume, and encrypted
R2 backups. WP Agent's Caddy provides HTTPS; Super Sparrow deployments do not restart it.

### Doppler and GitHub settings

Doppler still holds Super Sparrow's own production secrets and deployment settings, until they move to GitHub's `production` environment secrets. Super Sparrow cannot load its own settings from itself, because it would be down exactly when they are needed to fix it:

- **`super-sparrow` / `prd`** holds the application settings from `.env.example`, `SECRETS_ENCRYPTION_KEY` among them, and
  `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_DIR`, `DEPLOY_SSH_KEY`, and `DEPLOY_KNOWN_HOSTS`.
  The image's environment, listen address, and paths stay in `deploy/compose.production.yaml`.
- **`super-sparrow-recovery` / `prd`** holds `BACKUP_AGE_IDENTITY`, the backup private key, and
  references to the production R2 settings. No CI service token is created for this project. Keep a copy of `SECRETS_ENCRYPTION_KEY` in the team's password manager as well.
- `RESEND_API_KEY` is the sending-only key imported from Super Sparrow's local `.env`.
  Manage and rotate it in this project's production config.
- GitHub's **`production`** environment holds only `DOPPLER_TOKEN`, a read-only service token
  scoped to `super-sparrow/prd`. Only the `main` branch may use this environment. GHCR uses
  GitHub's short-lived job token rather than a stored registry password.

The deployment client sends only the named application settings to the VPS, not the SSH key,
Doppler token, or backup private key. SSH uses the previously verified host key and a dedicated
key pair. Its temporary local key file and the host's temporary registry login are deleted
when those operations end.

### Deployments and settings changes

`.github/workflows/ci.yml` checks Go, the frontend, formatting, the desktop app, and deployment
logic. A successful push to `main` then builds a Linux amd64 image with `deploy/Dockerfile`,
publishes it to GHCR with a commit tag, and deploys its immutable digest over SSH. Pull requests
run checks without accessing production secrets. The Docker build verifies that the SPA shell's
assets exist, and excludes secret files and generated local build output.

Production runs are serialized and are not canceled by a newer push. The host also holds a
deployment lock. Before changing files, it validates the proposed Compose configuration and
pulls the image. It saves a consistent SQLite snapshot, then replaces only the Super Sparrow
container. HTTPS health and frontend asset checks must succeed before the release is recorded.
The host keeps the three most recent pre-deployment snapshots with owner-only access.

To change settings, edit `super-sparrow/prd` in Doppler and run **Apply production settings** in
GitHub Actions on `main`. This updates settings without building or changing the running image.
The same validation and smoke checks apply. A failed deployment restores the previous settings
and image only when no migrations were applied; it preserves the current database, including
accepted writes. If the schema changed, it stops the failed release and reports the snapshot
path for manual recovery. It never automatically overwrites the live database.

### Existing host and recovery

The production deployment directory is `/opt/super-sparrow`, using `compose.production.yaml`.
The Compose project is `super-sparrow`, the external data volume is `sparrow-data`, and the
external network is `sparrow-proxy` (`172.30.90.0/29`). Caddy uses `172.30.90.2`; Sparrow uses
`172.30.90.3`. Provision the network and volume before deploying. The rollout rejects changes
to the running project's name, data volume, or proxy network; migrate them explicitly first.

The existing host was migrated on 2026-11-02. Its image and application settings were retained,
the copied database passed file and SQLite integrity checks, and encrypted R2 sync resumed.
The former data volume remains an offline recovery copy. Private migration records and original
configuration are in `/var/backups/super-sparrow/rename-20261102T151646Z` on the host; the database
in `sparrow-data` is now the source of truth. Do not start the old volume after the new instance
has accepted writes. The repository changes must be committed and merged before the next WP
Agent deployment, which would otherwise overwrite the shared proxy configuration again.

#### One-time migration of the existing resource names

This migration briefly interrupts both applications' HTTPS service. Run it as root on the host,
with both deployment pipelines paused. Keep the old volume and deployment files until the new
service, sign-in, event stream, and encrypted backups have been verified. Never run two Sparrow
containers against the database, and never copy a SQLite database while Sparrow is running.

1. Acquire both deployment locks for the entire maintenance session: open descriptor 8 on
   `/opt/wp-agent/staging/.deploy.lock` and descriptor 9 on
   `/opt/super-sparrow/staging/.deploy.lock`, then acquire `flock -n 8` and `flock -n 9`.
   Stop if either lock is held. Record the running Sparrow image digest and the Caddy container
   ID, and save both applications' current Compose files, Caddy files, and Caddy checksum.
2. Confirm that the existing Sparrow container mounts
   `super-sparrow-staging_sparrow-data` at `/data` and is the only writer. Confirm the only
   endpoints on `sparrow-staging-proxy` are Sparrow and WP Agent's Caddy. Stop if other services
   use that network. Verify a recent encrypted backup is available before continuing.
3. Stop Sparrow using its existing Compose file in `/opt/super-sparrow/staging`, without removing
   volumes. Inspect the old volume's `Mountpoint` with `docker volume inspect`, then create
   `sparrow-data` with `docker volume create sparrow-data`. Require the destination volume to
   be empty, and copy the stopped volume's complete contents with `cp -a`, using Docker's
   inspected mountpoints rather than assuming `/var/lib/docker`. Preserve numeric ownership
   and modes. Check the copied `sparrow.db` using SQLite `PRAGMA integrity_check` and compare
   all copied files against the stopped source before starting any new container.
4. Remove the stopped Sparrow container, leaving its volume intact. Stop and remove only
   Caddy from WP Agent's stack. Remove `sparrow-staging-proxy` after confirming it has no
   endpoints, then run `docker network create --subnet 172.30.90.0/29 sparrow-proxy`.
   Docker cannot rename a network or create an overlapping subnet while the old network exists.
5. Copy the private deployment settings, release state, and backups from
   `/opt/super-sparrow/staging` into `/opt/super-sparrow`, preserving permissions. Remove the
   copied `compose.staging.yaml` compatibility symlink if present; install the new
   `deploy/compose.production.yaml` there. Keep the recorded `SPARROW_IMAGE` unchanged for this
   migration. Install WP Agent PR #96's updated Compose, Caddy, and deployment scripts on the
   host, including the `sparrow-proxy` network attachment.
6. Start Sparrow from `/opt/super-sparrow` with
   `docker compose -f compose.production.yaml up -d --no-deps --pull never sparrow`.
   Recreate Caddy using WP Agent's usual base and staging Compose files and its existing `.env`.
   Validate the active Caddy configuration and verify `/api/health`, sign-in, frontend assets,
   the event stream, and all WP Agent HTTPS routes. Record the new Caddy configuration as
   last-good using WP Agent's `record` deployment command only after those checks succeed.
7. Set `DEPLOY_DIR=/opt/super-sparrow` in Doppler's `super-sparrow/prd` config. Move the local
   recovery key from `~/.local/state/super-sparrow/staging/backup-identity.txt` to
   `~/.local/state/super-sparrow/backup-identity.txt`, preserving `0700` directory and `0600`
   file permissions. Archive the old deployment directory securely outside the active path.
   Release the locks, resume both pipelines, and run the production deployment check.

If the migration fails before the new Sparrow starts, restore the saved Compose files, recreate
its former proxy network after removing the new network's endpoints, and start the old project
against the untouched old volume. Once the new Sparrow has accepted writes, its new volume is
the source of truth: never restart the stale old volume. Stop the new service before copying
its complete data back for any manual reversal, verify the copied database, and reconnect the
old proxy before restarting. Keep both pipelines paused until recovery is verified.

`sparrow.env` on the host is generated from Doppler and readable only by root (`0600`).
Previous settings are retained with the same permissions for rollback. These are plaintext
runtime files, and Docker administrators can inspect the container environment. Do not edit
these files manually or commit secrets. The backup private key stays off the VPS.

Local recovery copies remain at `~/.local/state/super-sparrow/backup-identity.txt` and
`~/Documents/Super Sparrow Recovery/backup-private-key.txt`, with `0600` file permissions and
`0700` directories. The separate Doppler recovery project provides an off-device copy.
[Restoring](#restoring) documents recovery from encrypted R2 copies. The pre-deployment SQLite
snapshots are local recovery aids; they are not the encrypted off-site backups.

WP Agent records the shared proxy sites with its Caddy rollback snapshot. Automatic rollback
retains the Sparrow import even when the previous snapshot predates that import. Deliberately
redeploying WP Agent commits older than this protection requires preserving the production route
and network attachment manually.

## Run it with systemd

1. Download `sparrow-<version>-linux-amd64` and `sparrow-<version>-web.tar.gz` from the GitHub Release, or build both with `make build`. Install the binary at `/usr/local/bin/sparrow`, and unpack the frontend into `/srv/sparrow/web` (from a build, copy `web/dist/client` there).
2. Put the settings in `/etc/sparrow/sparrow.env`, readable by root only, with `APP_ENV=production`, `DATABASE_PATH=/var/lib/sparrow/sparrow.db`, `WEB_DIR=/srv/sparrow/web`, and `CLI_DIR=/srv/sparrow/cli`. Download the latest release's `sparrow-*` binaries, `manifest.json`, and `manifest.json.sig` into the CLI folder, or leave it empty until there is a signed release.
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

Two jobs run inside the server, always in production and in development when the backup settings are set. Each takes a consistent copy with `VACUUM INTO`, compresses it, encrypts it with [age](https://age-encryption.org) to the public key in `BACKUP_AGE_RECIPIENT`, and uploads it to R2. The server holds only the public key, so someone who takes the server or its settings still cannot read a backup.

- The **sync** runs when the server starts and then every hour, replacing `sync/sparrow.db.gz.age`. R2 replaces the old copy only once the new upload has finished.
- The **daily backup** runs at `BACKUP_DAILY_TIME` in `BACKUP_TIME_ZONE` (03:00 in `Asia/Dhaka` in production) as `daily/sparrow-YYYY-MM-DD.db.gz.age`, dated in that time zone, then deletes dated copies older than `BACKUP_KEEP_DAYS`. A daily backup that fails, or that was missed while the server was down, runs again after each hourly sync until one succeeds that day.

Each run is recorded in the database, and failures are logged.

To set them up:

1. Run `sparrow backup-keygen`. It prints a key pair: save the three lines it prints in the team's password manager, because a backup cannot be restored without the private key. Never put them on the server.
2. Create an R2 bucket, and an R2 API token with Object Read & Write on that bucket only.
3. Set `BACKUP_AGE_RECIPIENT` to the public key, starting with `age1`, that `backup-keygen` printed, `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`, `BACKUP_DAILY_TIME` and `BACKUP_TIME_ZONE` (`03:00` and `Asia/Dhaka`), and `BACKUP_KEEP_DAYS` (`30` in production).

## Restoring

`sparrow restore` needs only the four `R2_` settings and the private key, so it runs on a fresh machine before the server is set up there.

1. Export the four R2 settings in the shell, and copy the private key from the password manager.
2. `sparrow restore --list` shows the copies.
3. `sparrow restore --copy sync --identity - --database /var/lib/sparrow/sparrow.db` restores the hourly copy, reading the private key from standard input, so paste it and press Ctrl-D. `--identity` also takes a file, such as one saved by `backup-keygen`; delete that file afterwards. Use `--copy daily/2026-03-01` for a dated copy. The command decrypts the copy, runs `PRAGMA integrity_check`, and only then writes the database file. It refuses to overwrite an existing database, so move a damaged one aside first. The copies are standard age files, so `age -d -i key.txt copy.db.gz.age | gunzip > sparrow.db` also works.
4. Start the server. It applies any newer migrations at startup.

Secret values are in the database encrypted with `SECRETS_ENCRYPTION_KEY`, and the key is not in the backup, so restoring a database to a fresh server also needs the same key in its settings. With a different key the server starts, but no secret can be read.

A sync loses at most the last hour of changes. Restore a daily copy to undo a mistake that the sync has already copied, such as a bad delete.

## GitHub webhook

The webhook marks an issue done when its pull request merges. It finds the issue by the pull request's URL, or else by the branch name, as in `feat/wp-142-meta-comments`.

1. Set `GITHUB_WEBHOOK_SECRET` to a random string of at least 20 characters, such as the output of `openssl rand -hex 32`.
2. In the repository's settings, add a webhook with the payload URL `https://sparrow.example.com/api/webhooks/github`, content type `application/json`, the same secret, and the "Pull requests" event.

The server refuses any delivery whose `X-Hub-Signature-256` does not match. The change is recorded as the system actor.

## Importing from Linear

On the server, with the server's settings and a Linear personal API key in `LINEAR_API_KEY`:

`sparrow import-linear --project "Version 01"`

It imports the project, its milestones, its issues with their Linear numbers, labels, parents, and assignees matched by email, and every comment, with the original author and time written at its top. A comment in the review round format below also becomes a finished review round with its findings. The import is written as the system actor, and running it again imports only what is new. Pass `--team KEY` when the project belongs to several Linear teams. The command prints a warning for everything it skipped.

The review round format the importer reads is below. Labels can be bold or plain, findings can be headings or bold lines, and fields can come in any order. Review rounds do not require source-control metadata.

```md
## Review round 2

Plan coverage: Steps 1 to 3.

### R1 [Blocking] Text can arrive before its file

**Problem:** The worker sends whichever row it claims first.
**Fix:** Send both from one job.
**Evidence:** `apps/api/src/staff-reply.ts:189`
**Status:** fixed

### Summary

One blocking finding.
```

## CLI releases

Each version tag builds the `sparrow` CLI for Linux, macOS, and Windows (amd64 and arm64) and signs it, so teammates and agents can install and update it without trusting the server that hands it out. The `release` workflow's `cli` jobs build the six binaries. Its `sign` job then writes `manifest.json`, which lists the version and each file's size and SHA-256 checksum, signs it with an Ed25519 key into `manifest.json.sig`, writes `SHA256SUMS`, and uploads all of them to the GitHub Release. The `sign` job runs in the GitHub `release` environment, which holds the signing key and waits for a reviewer's approval on every run. The Sparrow server never holds the key.

To set it up once:

1. In the repository's settings, create an environment named `release`. Add required reviewers, and restrict deployment to `v*` tags, so only a tagged release can use the key.
2. Run `sparrow release keygen` twice, once for the current key and once for a spare. Each run prints the private key on standard output and the public key and its key ID on standard error. Save both private keys in the team's password manager; they are not stored anywhere else.
3. Add the current private key to the `release` environment as the secret `CLI_SIGNING_KEY`. Keep the spare's private key out of GitHub.
4. Put both public keys in `PublicKeys` in `server/internal/release/release.go`, the current one first, and merge that before you push the first tag. Every CLI trusts the keys built into it, so the spare lets you replace a leaked key: sign the next release with the spare and every installed CLI still accepts it.

To release, push a `v*` tag as in [GETTING_STARTED.md](GETTING_STARTED.md#releases) and approve the `sign` job. The next deploy from `main` downloads the latest signed CLI into `deploy/cli`, which the Dockerfile copies to `/srv/cli`, so the server offers it at `/cli/{file}`. When there is no release yet, or the latest one has no signature, the image ships without a CLI.

### Installing and updating

The first install comes from the private repository's GitHub Releases, so it never trusts the Sparrow server. With the GitHub CLI signed in to an account that can read the repository, for Linux on amd64:

1. `gh release download --repo shantodotdev/super-sparrow --pattern sparrow-linux-amd64 --pattern SHA256SUMS`
2. `grep ' sparrow-linux-amd64$' SHA256SUMS | sha256sum --check` must print `OK`. On macOS use `shasum -a 256 --check` instead.
3. `chmod +x sparrow-linux-amd64`, then move it onto your `PATH` as `sparrow`. Other platforms use their own file name: `sparrow-darwin-arm64`, `sparrow-windows-amd64.exe`, and so on.
4. `sparrow login --url https://sparrow.example.com` signs you in.

After that, `sparrow update` downloads the server's current release and replaces the running program. It needs the server's address, from the stored login or `SPARROW_URL`, but no sign-in. It accepts a release only when:

- the address is `https`, or `http` for `localhost` only;
- the manifest's signature matches a public key built into the CLI;
- the version is newer than its own, so a signed older release with a known bug cannot be served again (a `dev` build may update to any release);
- the downloaded file has the size and checksum the signed manifest names.

On Windows the old program is kept as `sparrow.exe.old` next to the new one.

## Settings for CI

A project with secrets turned on keeps its `staging` and `production` settings in Super Sparrow's Secrets page, encrypted, with a history of every change. CI reads one config with a service token:

1. Someone allowed to manage staging and production creates the token in the project's Secrets page, choosing the config and a label such as `circleci staging`. The `sst_…` secret is shown once. Store it as a CI secret; Super Sparrow keeps only its hash.
2. The job downloads the config with `curl`, which installs nothing new. The token names the project and config, so the request carries nothing else but the format:

```bash
curl -fsS -H "Authorization: Bearer $SPARROW_SERVICE_TOKEN" \
  "https://sparrow.example.com/api/secrets/download?format=env" > .env
```

Use `format=json` or `format=shell` instead, as [API.md](API.md#secrets) describes. A value with a single quote or a line break cannot be written as an `env` line, so the server refuses `format=env` for it; use `json` or `shell`. A wrong, expired, or revoked token gets 401.

A service token reads one config and cannot write, and every download goes in the access log. It lasts a year; renew it from the Secrets page, which marks a token that expires within 14 days. Revoke a token there when a job no longer needs it. People and their agents never read staging or production values with the CLI: those need the permission and a browser.

## Connecting agents

On a person's own machine, agents use the CLI through that person's sign-in and need no token. Install the CLI as above and run `sparrow login`. It emails a 6-digit code, like the web, and keeps the session in the operating system's keychain. Where there is no keychain, such as a headless Linux machine, it writes `credentials.json` under the user's config folder with owner-only permissions and prints a warning. The session lasts 30 days and renews while in use. It appears in Settings next to the person's other devices, marked CLI, and can be signed out there or with `sparrow logout`.

Inside Claude Code and Codex the CLI sends the `Sparrow-Agent` header, so their writes are recorded as that person's `claude-code` or `codex` agent. It detects Claude Code from `CLAUDECODE=1`, and Codex from `CODEX_SANDBOX`, `CODEX_SANDBOX_NETWORK_DISABLED`, or `CODEX_THREAD_ID`. `--agent claude-code|codex` or `SPARROW_AGENT` names the agent when detection cannot. With none of them, the CLI acts as the person. The server cannot verify which agent runs the CLI; it verifies the person's sign-in. Those two names are the only agent names, for tokens too.

Agent tokens are for cloud sessions, where there is no person's sign-in to use. A cloud session sets `SPARROW_URL` and `SPARROW_TOKEN` for the CLI, or connects to the MCP endpoint with the token, as below. A local agent can still use a token and MCP if you prefer.

Each person creates a token per agent and machine in Settings. The Create agent token form provides a **Copy setup prompt** action after creation, tailored to Claude Code or Codex and the current server address. The prompt contains instructions and endpoint metadata, never the token secret. Tokens are shown once; the server retains only their hashes and cannot return the secret later.

The prompt asks the user's agent to configure permanent, user-scoped access on their machine:

- Read the official client documentation and confirm the installed client supports HTTP credential helpers.
- Accept the token through a private local input and store it in the user's persistent OS credential store, scoped to the endpoint and client.
- Create a local helper outside the repository that reads the stored credential and emits the Authorization header directly to the MCP client. Never capture its raw output in agent logs.
- Configure Claude Code at user scope using `headersHelper` in its user configuration (normally `~/.claude.json`), or Codex using `http_headers_helper` in its user configuration (normally `~/.codex/config.toml`). Preserve unrelated configuration and use absolute helper paths.
- Verify MCP discovery and `list_projects` without exposing the token or changing project data. Report any client restart required.

This setup persists across terminal closure, application restarts, and reboot; it does not require session environment exports or shell-profile changes. It requires local client execution, supported client versions, and an available OS credential store. If private input, helper support, or secure storage is unavailable, the agent must report the missing prerequisite rather than substitute plaintext storage or a temporary setup. The generated prompt does not itself install or verify the connection; the user's agent performs those steps.

Configuration persistence does not remove token expiry. A person renews the token in Super Sparrow before its 90-day expiry or creates a replacement and updates its credential-store entry. Agents must not create, renew, or revoke tokens themselves. Never put credentials in a repository or a setup prompt.

For client configuration details, see the official [Claude Code MCP guide](https://code.claude.com/docs/en/mcp) and [Codex MCP guide](https://developers.openai.com/codex/mcp).

See [MCP.md](MCP.md) for the tool workflow, pagination, retry keys, structured errors, and connection verification. `sparrow tool list` and `sparrow tool call list_projects --arguments '{}'` check the endpoint using `SPARROW_URL` and `SPARROW_TOKEN`; discovery in the actual agent client also verifies its configuration.

An agent may also send `Sparrow-Model: <model>` to record the model it runs on its changes. The server records the value as reported; it cannot verify it.

Scripts and agents without MCP use the CLI. It runs the same operations, with a command for every MCP tool, through the HTTP API with a `sparrow login` session or `SPARROW_URL` and `SPARROW_TOKEN`, and `sparrow help` lists its commands.
