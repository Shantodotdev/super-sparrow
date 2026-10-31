"""Exercise secret filtering and failure recovery without contacting the production server."""

import io
import json
import sqlite3
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import rollout

import deploy


class SecretBoundaryTests(unittest.TestCase):
    def test_recovery_and_connection_secrets_are_not_runtime_settings(self):
        settings = {
            name: "value" for name in deploy.APP_SETTINGS + deploy.CONNECTION_SETTINGS
        }
        settings["BACKUP_AGE_IDENTITY"] = "must-not-reach-the-server"
        runtime = deploy.production_settings(settings)
        self.assertEqual(set(runtime), set(deploy.APP_SETTINGS))
        self.assertNotIn("BACKUP_AGE_IDENTITY", runtime)
        self.assertNotIn("DEPLOY_SSH_KEY", runtime)

    def test_env_values_cannot_inject_another_setting(self):
        for value in (
            "secret\nAPP_ENV=development",
            "secret\rvalue",
            "secret'value",
            "secret\x00value",
        ):
            with self.subTest(value=value), self.assertRaises(ValueError):
                rollout.environment_text({"SIGN_IN_CODE_KEY": value})
        self.assertEqual(
            rollout.environment_text({"KEY": "$literal#value"}),
            "KEY='$literal#value'\n",
        )


class RollbackTests(unittest.TestCase):
    def exercise_failure(self, schema_changes):
        """Simulate writes during a failed release and inspect the rollback decision."""
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            database = folder / "sparrow.db"
            with sqlite3.connect(database) as connection:
                connection.executescript(
                    "CREATE TABLE schema_migrations(name TEXT); INSERT INTO schema_migrations VALUES ('0001'); CREATE TABLE accepted_writes(body TEXT);"
                )
            old = {
                "sparrow.env": "old-settings\n",
                ".env": "SPARROW_IMAGE=previous\n",
                "compose.production.yaml": "old-compose\n",
            }
            for name, contents in old.items():
                (folder / name).write_text(contents)
            request = {
                "directory": str(folder),
                "mode": "configure",
                "settings": {"PUBLIC_URL": "https://example.com"},
                "image": None,
                "commit": None,
                "compose": "new-compose\n",
            }
            operations = []

            def command(arguments, **options):
                operations.append(arguments)
                if arguments[:2] == ["docker", "inspect"]:
                    return json.dumps(
                        [
                            {
                                "Mounts": [
                                    {"Name": "live-data", "Destination": "/data"}
                                ],
                                "Config": {"Image": "previous"},
                            }
                        ]
                    )
                if arguments[:3] == ["docker", "volume", "inspect"]:
                    return json.dumps([{"Mountpoint": str(folder)}])
                if arguments[-3:] == ["ps", "-q", "sparrow"]:
                    return "running-container"
                return ""

            def fail_smoke(url):
                with sqlite3.connect(database) as connection:
                    connection.execute(
                        "INSERT INTO accepted_writes VALUES ('preserve this')"
                    )
                    if schema_changes:
                        connection.execute(
                            "INSERT INTO schema_migrations VALUES ('0002')"
                        )
                raise RuntimeError("unhealthy")

            original_directory = Path.cwd()
            try:
                with (
                    patch.object(
                        rollout.sys, "stdin", io.StringIO(json.dumps(request))
                    ),
                    patch.object(rollout, "command", command),
                    patch.object(rollout, "smoke", fail_smoke),
                    patch.object(rollout.time, "sleep"),
                    self.assertRaisesRegex(
                        RuntimeError, "schema" if schema_changes else "previous image"
                    ),
                ):
                    rollout.main()
            finally:
                import os

                os.chdir(original_directory)
            with sqlite3.connect(database) as connection:
                self.assertGreater(
                    connection.execute(
                        "SELECT COUNT(*) FROM accepted_writes"
                    ).fetchone()[0],
                    0,
                )
            self.assertEqual(
                any(operation[-2:] == ["stop", "sparrow"] for operation in operations),
                schema_changes,
            )
            if not schema_changes:
                for name, contents in old.items():
                    self.assertEqual((folder / name).read_text(), contents)
            self.assertEqual(len(list((folder / "backups").glob("*.db"))), 1)

    def test_image_rollback_preserves_newly_accepted_writes(self):
        self.exercise_failure(False)

    def test_changed_schema_stops_the_failed_release_for_manual_recovery(self):
        self.exercise_failure(True)


if __name__ == "__main__":
    unittest.main()
