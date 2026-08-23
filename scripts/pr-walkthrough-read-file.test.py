#!/usr/bin/env python3
"""Tests for immutable, regular-file reads from an untrusted PR commit."""

from pathlib import Path
import os
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "pr-walkthrough-read-file"


class PRWalkthroughReadFileTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name) / "repo"
        self.repo.mkdir()
        self.git("init")
        self.git("config", "user.name", "Walkthrough Test")
        self.git("config", "user.email", "walkthrough@example.invalid")
        (self.repo / "src").mkdir()
        (self.repo / "src" / "regular.txt").write_text("trusted commit bytes\n", encoding="utf-8")
        (self.repo / "executable.sh").write_text("#!/bin/sh\n", encoding="utf-8")
        os.chmod(self.repo / "executable.sh", 0o755)
        (self.repo / "binary.dat").write_bytes(b"\xff\xfe")
        (self.repo / "large.txt").write_text("x" * (512 * 1024 + 1), encoding="utf-8")
        os.symlink("/etc/passwd", self.repo / "outside-link")
        self.git("add", ".")
        self.git("commit", "-m", "test fixture")
        self.head = self.git("rev-parse", "HEAD").stdout.strip()

    def git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args],
            cwd=self.repo,
            text=True,
            capture_output=True,
            check=True,
        )

    def read(self, path: str, *, head: str | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--repo",
                str(self.repo),
                "--head-sha",
                head or self.head,
                "--path",
                path,
            ],
            text=True,
            capture_output=True,
            check=False,
        )

    def test_reads_regular_file_from_exact_commit_not_worktree(self) -> None:
        (self.repo / "src" / "regular.txt").write_text("changed worktree bytes\n", encoding="utf-8")

        result = self.read("src/regular.txt")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "trusted commit bytes\n")

    def test_reads_executable_regular_file(self) -> None:
        result = self.read("executable.sh")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "#!/bin/sh\n")

    def test_rejects_traversal_absolute_and_pathspec_paths(self) -> None:
        for path in ("../outside", "/etc/passwd", "src//regular.txt", ":(glob)**"):
            with self.subTest(path=path):
                result = self.read(path)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("invalid repository-relative path", result.stderr)

    def test_rejects_symlink(self) -> None:
        result = self.read("outside-link")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("regular file", result.stderr)

    def test_rejects_binary_file(self) -> None:
        result = self.read("binary.dat")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("UTF-8 text", result.stderr)

    def test_rejects_oversized_file(self) -> None:
        result = self.read("large.txt")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exceeds 524288 bytes", result.stderr)

    def test_rejects_missing_file_and_invalid_sha(self) -> None:
        missing = self.read("missing.txt")
        invalid_sha = self.read("src/regular.txt", head="main")

        self.assertNotEqual(missing.returncode, 0)
        self.assertIn("not a regular file", missing.stderr)
        self.assertNotEqual(invalid_sha.returncode, 0)
        self.assertIn("40 lowercase hexadecimal", invalid_sha.stderr)


if __name__ == "__main__":
    unittest.main()
