import json
import os
import tempfile
import unittest
from pathlib import Path

from scripts.fixture_ssh_identity_preflight import (
    PreflightError,
    identity_path_for_home,
    parse_process_env_value,
    private_regular_file,
    verify_launch_manifest,
)


class FixtureSSHIdentityPreflightTests(unittest.TestCase):
    def test_child_home_selects_the_default_ssh_identity_path(self):
        self.assertEqual(
            identity_path_for_home("/fixture/home"),
            Path("/fixture/home/.ssh/id_ed25519"),
        )

    def test_missing_or_relative_home_fails_closed(self):
        for home in ("", "relative/home"):
            with self.subTest(home=home), self.assertRaises(PreflightError):
                identity_path_for_home(home)

    def test_process_environment_value_must_be_unique(self):
        self.assertEqual(
            parse_process_env_value("kandev HOME=/fixture/home PATH=/bin", "HOME"),
            "/fixture/home",
        )
        with self.assertRaises(PreflightError):
            parse_process_env_value("kandev HOME=/one HOME=/two", "HOME")

    def test_identity_must_be_private_regular_and_inside_fixture(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            key = root / "home" / ".ssh" / "id_ed25519"
            key.parent.mkdir(parents=True)
            key.write_text("fixture-only")
            os.chmod(key, 0o600)
            private_regular_file(key, root=root)
            with self.assertRaises(PreflightError):
                private_regular_file(root / "missing", root=root)

    def test_supervisor_manifest_must_preserve_expected_home_and_backend(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            home = root / "home"
            home.mkdir()
            manifest_path = home / "launch.json"
            executable = root / "bin" / "kandev"
            executable.parent.mkdir()
            executable.write_text("fixture binary placeholder")
            payload = {
                "mode": "start",
                "backend_executable": str(executable),
                "argv": ["__backend"],
                "home_dir": str(home),
                "port": 43841,
                "env": {
                    "HOME": str(home),
                    "KANDEV_HOME_DIR": str(home),
                    "KANDEV_DATABASE_PATH": str(home / "data.db"),
                },
            }
            manifest_path.write_text(json.dumps(payload))
            os.chmod(manifest_path, 0o600)
            verify_launch_manifest(
                manifest_path,
                executable=executable,
                expected_home=str(home),
                expected_database=str(home / "data.db"),
                port=43841,
            )
            # The live restart harness naturally supplies pathlib objects.
            # This used to fail *after* the API child had started because the
            # manifest contains strings while expected_home was a Path.
            verify_launch_manifest(
                manifest_path,
                executable=executable,
                expected_home=home,
                expected_database=home / "data.db",
                port=43841,
            )
            payload["env"]["HOME"] = "/wrong/home"
            manifest_path.write_text(json.dumps(payload))
            with self.assertRaises(PreflightError):
                verify_launch_manifest(
                    manifest_path,
                    executable=executable,
                    expected_home=str(home),
                    expected_database=str(home / "data.db"),
                    port=43841,
                )


if __name__ == "__main__":
    unittest.main()
