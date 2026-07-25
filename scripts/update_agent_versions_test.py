#!/usr/bin/env python3
"""Tests for the allowlisted core-agent version updater."""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("update_agent_versions.py")


def load_updater():
    if not SCRIPT.exists():
        raise AssertionError(f"updater module must exist at {SCRIPT}")
    spec = importlib.util.spec_from_file_location("update_agent_versions", SCRIPT)
    if spec is None or spec.loader is None:
        raise AssertionError(f"cannot load updater module from {SCRIPT}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class UpdateAgentVersionsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.updater = load_updater()

    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.root = Path(self.temp_dir.name)

    def write(self, path: str, content: str) -> None:
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    def fixture_pin(self):
        return self.updater.Pin(
            agent="Example",
            package="@example/agent",
            source_path="agent.go",
            version_constant="exampleVersion",
            targets=(
                self.updater.Target("agent.go", 1),
                self.updater.Target("agent_test.go", 2),
                self.updater.Target("VERSIONS.md", 1),
            ),
        )

    def write_valid_fixture(self) -> None:
        self.write("agent.go", 'const exampleVersion = "1.2.3"\n')
        self.write(
            "agent_test.go",
            'want := "@example/agent@1.2.3"\ninstall := "@example/agent@1.2.3"\n',
        )
        self.write("VERSIONS.md", "| Example | `1.2.3` |\n")

    def test_updates_only_allowlisted_occurrences_and_reports_change(self) -> None:
        self.write_valid_fixture()
        self.write("history.md", "Historical @example/agent@1.2.3 stays unchanged.\n")

        updates = self.updater.update_repository(
            self.root,
            {"@example/agent": "1.3.0"},
            pins=(self.fixture_pin(),),
        )

        self.assertEqual(
            [update.as_dict() for update in updates],
            [
                {
                    "agent": "Example",
                    "package": "@example/agent",
                    "current": "1.2.3",
                    "latest": "1.3.0",
                }
            ],
        )
        self.assertIn('"1.3.0"', (self.root / "agent.go").read_text())
        self.assertEqual(
            (self.root / "agent_test.go").read_text().count("1.3.0"),
            2,
        )
        self.assertIn("1.2.3", (self.root / "history.md").read_text())
        self.assertIn("| Example | `@example/agent` | `1.2.3` | `1.3.0` |", self.updater.markdown_report(updates))

    def test_current_versions_are_a_noop(self) -> None:
        self.write_valid_fixture()
        before = {path: path.read_text() for path in self.root.iterdir()}

        updates = self.updater.update_repository(
            self.root,
            {"@example/agent": "1.2.3"},
            pins=(self.fixture_pin(),),
        )

        self.assertEqual(updates, [])
        self.assertEqual(before, {path: path.read_text() for path in self.root.iterdir()})

    def test_prerelease_latest_fails_before_writing(self) -> None:
        self.write_valid_fixture()
        before = (self.root / "agent.go").read_text()

        with self.assertRaisesRegex(ValueError, "stable semantic version"):
            self.updater.update_repository(
                self.root,
                {"@example/agent": "1.3.0-beta.1"},
                pins=(self.fixture_pin(),),
            )

        self.assertEqual((self.root / "agent.go").read_text(), before)

    def test_occurrence_drift_fails_before_any_write(self) -> None:
        self.write_valid_fixture()
        self.write("agent_test.go", 'want := "@example/agent@1.2.3"\n')
        before = {
            path.relative_to(self.root): path.read_text()
            for path in self.root.rglob("*")
            if path.is_file()
        }

        with self.assertRaisesRegex(ValueError, "expected 2 occurrences"):
            self.updater.update_repository(
                self.root,
                {"@example/agent": "1.3.0"},
                pins=(self.fixture_pin(),),
            )

        after = {
            path.relative_to(self.root): path.read_text()
            for path in self.root.rglob("*")
            if path.is_file()
        }
        self.assertEqual(after, before)

    def test_occurrence_drift_fails_even_when_version_is_current(self) -> None:
        self.write_valid_fixture()
        self.write("agent_test.go", 'want := "@example/agent@1.2.3"\n')

        with self.assertRaisesRegex(ValueError, "expected 2 occurrences"):
            self.updater.update_repository(
                self.root,
                {"@example/agent": "1.2.3"},
                pins=(self.fixture_pin(),),
            )

    def test_shared_versions_update_by_package_specific_template(self) -> None:
        first_pin = self.updater.Pin(
            agent="First",
            package="@example/first",
            source_path="first.go",
            version_constant="firstVersion",
            targets=(
                self.updater.Target("first.go", 1, 'const firstVersion = "{version}"'),
                self.updater.Target(
                    "VERSIONS.md", 1, "| First | `@example/first` | `{version}` |"
                ),
            ),
        )
        second_pin = self.updater.Pin(
            agent="Second",
            package="@example/second",
            source_path="second.go",
            version_constant="secondVersion",
            targets=(
                self.updater.Target("second.go", 1, 'const secondVersion = "{version}"'),
                self.updater.Target(
                    "VERSIONS.md", 1, "| Second | `@example/second` | `{version}` |"
                ),
            ),
        )
        self.write("first.go", 'const firstVersion = "1.2.3"\n')
        self.write("second.go", 'const secondVersion = "1.2.3"\n')
        self.write(
            "VERSIONS.md",
            "| First | `@example/first` | `1.2.3` |\n"
            "| Second | `@example/second` | `1.2.3` |\n",
        )

        updates = self.updater.update_repository(
            self.root,
            {"@example/first": "1.3.0", "@example/second": "1.4.0"},
            pins=(first_pin, second_pin),
        )

        self.assertEqual(len(updates), 2)
        self.assertEqual(
            (self.root / "VERSIONS.md").read_text(),
            "| First | `@example/first` | `1.3.0` |\n"
            "| Second | `@example/second` | `1.4.0` |\n",
        )

    def test_registry_lookup_parses_json_without_executing_package(self) -> None:
        completed = subprocess.CompletedProcess(
            args=[],
            returncode=0,
            stdout=json.dumps("2.4.6") + "\n",
            stderr="",
        )
        with mock.patch.object(self.updater.subprocess, "run", return_value=completed) as run:
            version = self.updater.fetch_latest_version("@example/agent")

        self.assertEqual(version, "2.4.6")
        run.assert_called_once_with(
            ["npm", "view", "@example/agent", "dist-tags.latest", "--json"],
            check=True,
            capture_output=True,
            text=True,
            timeout=30,
        )

    def test_main_returns_error_when_metadata_lookup_times_out(self) -> None:
        stderr = io.StringIO()
        with (
            mock.patch.object(
                self.updater.subprocess,
                "run",
                side_effect=subprocess.TimeoutExpired(["npm", "view"], 30),
            ),
            contextlib.redirect_stderr(stderr),
        ):
            status = self.updater.main()

        self.assertEqual(status, 2)
        self.assertIn("timed out", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
