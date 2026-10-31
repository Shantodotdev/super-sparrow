#!/usr/bin/env python3
"""Load production settings from Doppler and send only deployment inputs over verified SSH."""

import argparse
import json
import os
import re
import shlex
import subprocess
import tempfile
from pathlib import Path

APP_SETTINGS = (
    "PUBLIC_URL",
    "LOG_LEVEL",
    "TRUSTED_PROXIES",
    "RESEND_API_KEY",
    "EMAIL_FROM",
    "SIGN_IN_CODE_KEY",
    "GITHUB_WEBHOOK_SECRET",
    "BACKUP_AGE_RECIPIENT",
    "BACKUP_DAILY_TIME",
    "BACKUP_TIME_ZONE",
    "BACKUP_KEEP_DAYS",
    "R2_ACCOUNT_ID",
    "R2_ACCESS_KEY_ID",
    "R2_SECRET_ACCESS_KEY",
    "R2_BUCKET",
)
CONNECTION_SETTINGS = (
    "DEPLOY_HOST",
    "DEPLOY_USER",
    "DEPLOY_DIR",
    "DEPLOY_SSH_KEY",
    "DEPLOY_KNOWN_HOSTS",
)


def production_settings(settings):
    """Select runtime settings without forwarding deployment credentials or recovery material."""
    for name in APP_SETTINGS + CONNECTION_SETTINGS:
        value = settings.get(name)
        if not isinstance(value, str) or not value.strip():
            raise ValueError(f"Doppler setting {name} is required")
    return {name: settings[name] for name in APP_SETTINGS}


def main():
    """Validate arguments, fetch Doppler settings, and execute the host-side rollout."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", required=True)
    parser.add_argument("--config", required=True)
    parser.add_argument(
        "--mode", choices=("check", "configure", "release"), required=True
    )
    parser.add_argument("--image")
    parser.add_argument("--commit")
    args = parser.parse_args()
    if args.mode == "release":
        if not args.commit or not re.fullmatch(r"[0-9a-f]{40}", args.commit):
            parser.error("release requires --commit with a full commit SHA")
        if not args.image or not re.fullmatch(
            r"ghcr\.io/shantodotdev/super-sparrow@sha256:[0-9a-f]{64}", args.image
        ):
            parser.error(
                "release requires an immutable Super Sparrow GHCR image digest"
            )
        for name in ("REGISTRY_USER", "REGISTRY_TOKEN"):
            if not os.environ.get(name):
                parser.error(f"{name} is required for a release")

    # Capture secrets in memory, never in command arguments or deployment output.
    result = subprocess.run(
        [
            "doppler",
            "secrets",
            "download",
            "--no-file",
            "--format",
            "json",
            "--project",
            args.project,
            "--config",
            args.config,
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode:
        raise RuntimeError("Could not load Doppler configuration")
    settings = json.loads(result.stdout)
    application = production_settings(settings)
    for name in ("DEPLOY_HOST", "DEPLOY_USER"):
        if not re.fullmatch(r"[A-Za-z0-9_.-]+", settings[name]) or settings[
            name
        ].startswith("-"):
            raise ValueError(f"Invalid {name}")
    payload = {
        "mode": args.mode,
        "directory": settings["DEPLOY_DIR"],
        "settings": application,
        "image": args.image,
        "commit": args.commit,
        "registry_user": os.environ.get("REGISTRY_USER")
        if args.mode == "release"
        else None,
        "registry_token": os.environ.get("REGISTRY_TOKEN")
        if args.mode == "release"
        else None,
        "compose": Path(__file__).with_name("compose.production.yaml").read_text(),
    }
    remote = Path(__file__).with_name("rollout.py").read_text()

    # The private key and known-host file exist only for this SSH connection.
    with tempfile.TemporaryDirectory(prefix="sparrow-ssh-") as temporary:
        folder = Path(temporary)
        key = folder / "key"
        hosts = folder / "known_hosts"
        for path, value in (
            (key, settings["DEPLOY_SSH_KEY"]),
            (hosts, settings["DEPLOY_KNOWN_HOSTS"]),
        ):
            path.write_text(value.rstrip() + "\n")
            path.chmod(0o600)
        command = [
            "ssh",
            "-F",
            "/dev/null",
            "-o",
            "BatchMode=yes",
            "-o",
            "IdentitiesOnly=yes",
            "-o",
            "StrictHostKeyChecking=yes",
            "-o",
            f"UserKnownHostsFile={hosts}",
            "-o",
            "ConnectTimeout=15",
            "-o",
            "ServerAliveInterval=15",
            "-i",
            str(key),
            f"{settings['DEPLOY_USER']}@{settings['DEPLOY_HOST']}",
            "python3 -c " + shlex.quote(remote),
        ]
        subprocess.run(command, input=json.dumps(payload), text=True, check=True)


if __name__ == "__main__":
    main()
