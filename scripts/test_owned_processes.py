import os
import json
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

from owned_processes import OwnedProcessRegistry, ProcessOwnershipError


class OwnedProcessRegistryTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="nomad-e2e-processes-")
        self.root = Path(self.temp.name)
        self.root.chmod(0o700)
        self.registry = OwnedProcessRegistry(self.root)
        self.registry._process_identity = self._fixture_identity
        self.registry._group_members = lambda pgid: []

    def _fixture_identity(self, pid):
        child = self.registry._children.get(pid)
        if child is None or child.poll() is not None:
            return None
        return "fixture-start", pid

    def tearDown(self):
        # Test-owned children are always stopped before removing their fixture.
        try:
            try:
                self.registry.stop_all(timeout=2)
            except ProcessOwnershipError:
                # The symlink rejection case intentionally leaves a bad path.
                path = self.root / "runner-processes.json"
                if path.is_symlink():
                    path.unlink()
        finally:
            self.temp.cleanup()

    def test_registry_tracks_only_pid_start_identity_and_label(self):
        record = self.registry.start(
            "sleep-fixture",
            ["/bin/sleep", "30"],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin"},
        )

        self.assertEqual(set(record), {"label", "pid", "started_at", "pgid"})
        self.assertEqual(record["label"], "sleep-fixture")
        self.assertEqual(record["pgid"], record["pid"])
        stored = self.registry.read_records()
        self.assertEqual(stored, [record])

        stopped = self.registry.stop("sleep-fixture", timeout=2)
        self.assertEqual(stopped, "stopped")
        self.assertEqual(self.registry.read_records(), [])

    def test_pid_reuse_or_identity_mismatch_is_never_signaled(self):
        record = self.registry.start(
            "sleep-fixture",
            ["/bin/sleep", "30"],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin"},
        )
        original = self.registry._process_identity
        self.registry._process_identity = lambda pid: ("different-start", record["pgid"])
        try:
            with self.assertRaises(ProcessOwnershipError):
                self.registry.stop("sleep-fixture", timeout=0.1)
        finally:
            self.registry._process_identity = original

        self.assertEqual(self.registry.read_records(), [record])
        self.assertEqual(self.registry.stop("sleep-fixture", timeout=2), "stopped")

    def test_crash_refuses_start_time_format_mismatch_before_sigkill(self):
        record = self.registry.start(
            "crash-fixture",
            ["/bin/sleep", "30"],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin"},
        )
        child = self.registry._children[record["pid"]]
        original = self.registry._process_identity
        # A whitespace/format discrepancy must fail closed exactly like PID reuse.
        self.registry._process_identity = lambda pid: (" " + record["started_at"], record["pgid"])
        try:
            with self.assertRaises(ProcessOwnershipError):
                self.registry.crash("crash-fixture", timeout=0.1)
        finally:
            self.registry._process_identity = original

        self.assertIsNone(child.poll(), "identity mismatch must not signal the process")
        self.assertEqual(self.registry.read_records(), [record])
        self.assertEqual(self.registry.crash("crash-fixture", timeout=2), "crashed")
        self.assertEqual(self.registry.read_records(), [])

    def test_crash_retains_record_when_process_inspection_is_denied(self):
        record = self.registry.start(
            "inspection-fixture",
            ["/bin/sleep", "30"],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin"},
        )
        child = self.registry._children[record["pid"]]
        self.registry._process_identity = OwnedProcessRegistry._process_identity
        denied = subprocess.CompletedProcess(["ps"], 1, "", "permission denied")
        with patch("owned_processes.subprocess.run", side_effect=[denied, denied]):
            with self.assertRaises(ProcessOwnershipError):
                self.registry.crash("inspection-fixture", timeout=0.1)

        self.assertIsNone(child.poll(), "uninspectable PID must never be signaled")
        # Restore the test's owned-child identity probe for safe tearDown.
        self.registry._process_identity = self._fixture_identity
        self.assertEqual(self.registry.read_records(), [record])
        self.assertEqual(self.registry.stop("inspection-fixture", timeout=2), "stopped")

    def test_registry_rejects_unsafe_pid_and_process_group_values(self):
        path = self.root / "runner-processes.json"
        for field in ("pid", "pgid"):
            path.write_text(json.dumps({
                "schema_version": 1,
                "processes": [{"label": "bad", "pid": 123, "started_at": "fixture", "pgid": 123}],
            }), encoding="utf-8")
            payload = json.loads(path.read_text())
            payload["processes"][0][field] = 1
            path.write_text(json.dumps(payload), encoding="utf-8")
            with self.assertRaises(ProcessOwnershipError):
                self.registry.read_records()
        path.unlink()

    def test_missing_pid_is_recorded_as_absent_without_signaling_reused_identity(self):
        record = self.registry.start(
            "short-fixture",
            ["/bin/sleep", "0.05"],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin"},
        )
        time.sleep(0.2)

        self.assertEqual(self.registry.stop("short-fixture", timeout=0.1), "already_absent")
        self.assertEqual(self.registry.read_records(), [])

    def test_duplicate_label_is_rejected_before_second_process_starts(self):
        first = self.registry.start(
            "same-label",
            ["/bin/sleep", "30"],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin"},
        )
        with self.assertRaises(ProcessOwnershipError):
            self.registry.start(
                "same-label",
                ["/bin/sleep", "30"],
                cwd=self.root,
                env={"PATH": "/usr/bin:/bin"},
            )
        self.assertEqual(self.registry.read_records(), [first])

    def test_registry_file_symlink_is_rejected(self):
        path = self.root / "runner-processes.json"
        target = self.root / "elsewhere.json"
        target.write_text("{}", encoding="utf-8")
        path.symlink_to(target)

        with self.assertRaises(ProcessOwnershipError):
            self.registry.read_records()


if __name__ == "__main__":
    unittest.main()
