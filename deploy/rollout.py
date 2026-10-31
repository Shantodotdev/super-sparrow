"""Apply a serialized production rollout on the existing host without changing its proxy."""

import contextlib
import fcntl
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path


def environment_text(settings):
    """Encode Compose's env-file syntax, rejecting ambiguous or multiline values."""
    lines = []
    for name, value in settings.items():
        if not re.fullmatch(r"[A-Z][A-Z0-9_]*", name):
            raise ValueError("Invalid environment setting name")
        if not isinstance(value, str) or any(
            character in value for character in "\r\n\x00'"
        ):
            raise ValueError(f"Invalid value for {name}")
        lines.append(f"{name}='{value}'\n")
    return "".join(lines)


def write_private(path, text):
    """Atomically replace a configuration file with owner-only permissions."""
    descriptor, temporary = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(descriptor, "w") as target:
            target.write(text)
            target.flush()
            os.fsync(target.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def command(arguments, **options):
    """Run an operational command without echoing secrets from configuration diagnostics."""
    result = subprocess.run(
        arguments, capture_output=True, text=True, check=False, **options
    )
    if result.returncode:
        raise RuntimeError(f"Command failed: {' '.join(arguments[:3])}")
    return result.stdout.strip()


def database_state(database):
    """Read the applied migration names for a safe image-only rollback decision."""
    with sqlite3.connect(f"file:{database}?mode=ro", uri=True) as connection:
        return connection.execute(
            "SELECT name FROM schema_migrations ORDER BY name"
        ).fetchall()


def smoke(public_url):
    """Require database health and every asset referenced by the frontend shell."""

    def read(path):
        with urllib.request.urlopen(public_url + path, timeout=10) as response:
            return response.read()

    if json.loads(read("/api/health")) != {"status": "ok"}:
        raise RuntimeError("Database health check failed")
    shell = read("/signin").decode()
    assets = set(re.findall(r'(?:src|href)="(/assets/[^"?#]+)"', shell))
    if not assets:
        raise RuntimeError("Frontend shell has no assets")
    for asset in assets:
        read(asset)


def main():
    """Validate staged files, snapshot SQLite, and activate or safely roll back the release."""
    request = json.load(sys.stdin)
    directory = Path(request["directory"])
    if not directory.is_absolute() or not directory.is_dir():
        raise ValueError("The existing deployment directory is required")
    os.umask(0o077)
    with open(directory / ".deploy.lock", "a") as lock, contextlib.chdir(directory):
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        apply_rollout(request, directory)


def apply_rollout(request, directory):
    """Apply validated runtime settings while the deployment lock is held."""
    mode = request["mode"]
    if mode not in ("check", "configure", "release"):
        raise ValueError("Invalid deployment mode")
    settings = request["settings"]
    text = environment_text(settings)
    public_url = settings["PUBLIC_URL"]
    if not public_url.startswith("https://") or public_url.endswith("/"):
        raise ValueError("PUBLIC_URL must be HTTPS without a trailing slash")

    # Reuse the running project's volume and external proxy network.
    compose = ["docker", "compose", "-f", "compose.production.yaml"]
    container = command(compose + ["ps", "-q", "sparrow"])
    if not container:
        raise RuntimeError("An existing running Super Sparrow deployment is required")
    details = json.loads(command(["docker", "inspect", container]))[0]
    volume = next(
        mount["Name"] for mount in details["Mounts"] if mount["Destination"] == "/data"
    )
    mountpoint = json.loads(command(["docker", "volume", "inspect", volume]))[0][
        "Mountpoint"
    ]
    database = Path(mountpoint) / "sparrow.db"
    migrations = database_state(database)
    previous_image = details["Config"]["Image"]
    image = request["image"] if mode == "release" else previous_image

    with tempfile.TemporaryDirectory(prefix=".candidate-", dir=directory) as temporary:
        candidate = Path(temporary)
        write_private(candidate / "sparrow.env", text)
        write_private(candidate / ".env", environment_text({"SPARROW_IMAGE": image}))
        write_private(candidate / "compose.production.yaml", request["compose"])
        command(
            [
                "docker",
                "compose",
                "--project-directory",
                str(candidate),
                "-f",
                str(candidate / "compose.production.yaml"),
                "config",
                "--quiet",
            ]
        )
    if mode == "check":
        smoke(public_url)
        print(
            "Doppler settings, verified SSH, Compose configuration, live database, and HTTPS assets checked."
        )
        return

    # Pull before replacing settings; registry credentials are deleted after the pull.
    if mode == "release":
        with tempfile.TemporaryDirectory(prefix="sparrow-registry-") as temporary:
            registry_environment = os.environ.copy()
            registry_environment["DOCKER_CONFIG"] = temporary
            command(
                [
                    "docker",
                    "login",
                    "ghcr.io",
                    "--username",
                    request["registry_user"],
                    "--password-stdin",
                ],
                input=request["registry_token"],
                env=registry_environment,
            )
            command(["docker", "pull", image], env=registry_environment)

    backup_directory = directory / "backups"
    backup_directory.mkdir(mode=0o700, exist_ok=True)
    snapshot = backup_directory / f"pre-deploy-{time.time_ns()}.db"
    with (
        sqlite3.connect(f"file:{database}?mode=ro", uri=True) as source,
        sqlite3.connect(snapshot) as target,
    ):
        source.backup(target)
    snapshot.chmod(0o600)
    previous = {
        name: Path(name).read_text()
        for name in ("sparrow.env", ".env", "compose.production.yaml")
    }

    try:
        write_private(Path("sparrow.env"), text)
        write_private(Path(".env"), environment_text({"SPARROW_IMAGE": image}))
        write_private(Path("compose.production.yaml"), request["compose"])
        command(compose + ["up", "-d", "--no-deps", "--pull", "never", "sparrow"])
        for attempt in range(18):
            try:
                smoke(public_url)
                break
            except (OSError, RuntimeError, ValueError):
                if attempt == 17:
                    raise RuntimeError("Production smoke checks failed") from None
                time.sleep(5)
    except (OSError, RuntimeError, ValueError):
        if database_state(database) != migrations:
            command(compose + ["stop", "sparrow"])
            raise RuntimeError(
                f"Failed release changed the schema; service stopped. Manual recovery needs {snapshot}"
            ) from None
        for name, contents in previous.items():
            write_private(Path(name), contents)
        command(compose + ["up", "-d", "--no-deps", "--pull", "never", "sparrow"])
        raise RuntimeError(
            "Deployment failed; previous image and settings restored without replacing the database"
        ) from None

    for name, contents in previous.items():
        write_private(Path(name + ".previous"), contents)
    if mode == "release":
        write_private(Path(".last-good-commit"), request["commit"] + "\n")
    for old in sorted(backup_directory.glob("pre-deploy-*.db"))[:-3]:
        old.unlink()
    print(
        "Production deployment healthy; encrypted backups remain enabled and the existing data volume is preserved."
    )


if __name__ == "__main__":
    main()
