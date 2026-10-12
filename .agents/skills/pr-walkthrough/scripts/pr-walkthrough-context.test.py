#!/usr/bin/env python3
"""Tests for bounded, immutable PR walkthrough context preparation."""

import json
from pathlib import Path
import runpy
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


SKILL_DIR = Path(__file__).resolve().parents[1]
ROOT = SKILL_DIR.parents[2]
SCRIPT = SKILL_DIR / "scripts" / "pr-walkthrough-context"
MAX_FILE_BYTES = 512 * 1024
MAX_TOTAL_BYTES = 8 * 1024 * 1024


class PRWalkthroughContextTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name) / "repo"
        self.repo.mkdir()
        self.git("init")
        self.git("config", "user.name", "Walkthrough Test")
        self.git("config", "user.email", "walkthrough@example.invalid")

        (self.repo / "safe.txt").write_text("base bytes\n", encoding="utf-8")
        (self.repo / "removed.txt").write_text("remove me\n", encoding="utf-8")
        self.git("add", ".")
        self.git("commit", "-m", "base")
        self.base = self.git("rev-parse", "HEAD").stdout.strip()

        (self.repo / "safe.txt").write_text("head bytes\n", encoding="utf-8")
        (self.repo / "removed.txt").unlink()
        (self.repo / "binary.dat").write_bytes(b"\xff\xfe")
        (self.repo / "large.dat").write_bytes(b"x" * (MAX_FILE_BYTES + 1))
        (self.repo / "head-link").symlink_to("safe.txt")
        (self.repo / ":unsafe").write_text("must not materialize\n", encoding="utf-8")
        for index in range(17):
            (self.repo / f"zz-budget-{index:02d}.txt").write_bytes(
                b"x" * MAX_FILE_BYTES
            )
        self.git("add", "--all")
        self.git("commit", "-m", "head")
        self.head = self.git("rev-parse", "HEAD").stdout.strip()

    def git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args],
            cwd=self.repo,
            text=True,
            capture_output=True,
            check=True,
        )

    def run_context(
        self,
        output: Path,
        *,
        base: str | None = None,
        head: str | None = None,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--repo",
                str(self.repo),
                "--base-sha",
                base or self.base,
                "--head-sha",
                head or self.head,
                "--output-dir",
                str(output),
            ],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
        )

    def manifest(self, output: Path) -> dict[str, object]:
        return json.loads((output / "manifest.json").read_text(encoding="utf-8"))

    def test_materializes_exact_head_files_and_records_omissions(self) -> None:
        output = Path(self.temp.name) / "context"
        result = self.run_context(output)

        self.assertEqual(result.returncode, 0, result.stderr)
        manifest = self.manifest(output)
        self.assertEqual(manifest["base_sha"], self.base)
        self.assertEqual(manifest["head_sha"], self.head)
        self.assertEqual(manifest["merge_base"], self.base)
        entries = {entry["path"]: entry for entry in manifest["files"]}

        self.assertEqual(
            (output / "head" / "files" / "safe.txt").read_text(encoding="utf-8"),
            "head bytes\n",
        )
        self.assertNotIn("worktree bytes", (output / "head" / "files" / "safe.txt").read_text())
        self.assertFalse((output / "head" / "files" / "removed.txt").exists())
        self.assertEqual(entries["removed.txt"]["reason"], "deleted")
        self.assertEqual(entries["binary.dat"]["reason"], "binary")
        self.assertEqual(entries["large.dat"]["reason"], "oversized")
        self.assertEqual(entries["head-link"]["reason"], "non_regular")
        self.assertEqual(entries[":unsafe"]["reason"], "unsafe_path")
        self.assertTrue(
            any(entry.get("reason") == "budget_excluded" for entry in entries.values())
        )
        self.assertTrue(all(entry["materialized"] for entry in entries.values() if entry["path"] == "safe.txt"))

    def test_does_not_read_or_follow_worktree_files(self) -> None:
        output = Path(self.temp.name) / "context"
        (self.repo / "safe.txt").write_text("worktree bytes\n", encoding="utf-8")

        result = self.run_context(output)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            (output / "head" / "files" / "safe.txt").read_text(encoding="utf-8"),
            "head bytes\n",
        )
        self.assertFalse((output / "head" / "files" / "head-link").exists())

    def test_manifest_is_deterministic(self) -> None:
        first = Path(self.temp.name) / "first"
        second = Path(self.temp.name) / "second"

        first_result = self.run_context(first)
        second_result = self.run_context(second)

        self.assertEqual(first_result.returncode, 0, first_result.stderr)
        self.assertEqual(second_result.returncode, 0, second_result.stderr)
        self.assertEqual(
            (first / "manifest.json").read_bytes(),
            (second / "manifest.json").read_bytes(),
        )
        self.assertEqual(
            sorted(str(path.relative_to(first)) for path in (first / "head" / "files").rglob("*")),
            sorted(str(path.relative_to(second)) for path in (second / "head" / "files").rglob("*")),
        )

    def test_large_change_uses_bounded_git_processes(self) -> None:
        for index in range(200):
            (self.repo / f"batch-{index:03d}.txt").write_text(f"head file {index}\n")
        self.git("add", ".")
        self.git("commit", "-m", "large change")
        head = self.git("rev-parse", "HEAD").stdout.strip()
        prepare = runpy.run_path(str(SCRIPT))["prepare"]
        original = prepare.__globals__["subprocess"].Popen
        with patch.object(prepare.__globals__["subprocess"], "Popen", wraps=original) as launches:
            output = Path(self.temp.name) / "batch-context"
            prepare(str(self.repo), self.base, head, output)
        self.assertLessEqual(launches.call_count, 20)
        for index in range(200):
            self.assertEqual(
                (output / "head" / "files" / f"batch-{index:03d}.txt").read_text(),
                f"head file {index}\n",
            )

    def test_batch_framing_preserves_literal_paths_empty_files_and_unterminated_text(self) -> None:
        samples = {"a-empty.txt": b"", "b-[*]-ü\n.txt": b"no final newline", "c-after.txt": b"following record\n"}
        for name, content in samples.items():
            (self.repo / name).write_bytes(content)
        self.git("add", ".")
        self.git("commit", "-m", "batch framing")
        head = self.git("rev-parse", "HEAD").stdout.strip()
        output = Path(self.temp.name) / "framing-context"
        result = self.run_context(output, head=head)
        self.assertEqual(result.returncode, 0, result.stderr)
        for name, content in samples.items():
            self.assertEqual((output / "head" / "files" / name).read_bytes(), content)

    def test_partial_clone_fetches_changed_blobs_in_one_batch_without_moving_refs(self) -> None:
        for index in range(50):
            (self.repo / f"partial-{index:03d}.txt").write_text(f"promised file {index}\n")
        self.git("add", ".")
        self.git("commit", "-m", "promised blobs")
        head = self.git("rev-parse", "HEAD").stdout.strip()
        self.git("config", "uploadpack.allowFilter", "true")
        self.git("config", "uploadpack.allowAnySHA1InWant", "true")
        clone = Path(self.temp.name) / "partial"
        subprocess.run(["git", "clone", "--filter=blob:none", "--no-checkout", self.repo.as_uri(), str(clone)], check=True, capture_output=True)
        counter = Path(self.temp.name) / "upload-pack-count"
        wrapper = Path(self.temp.name) / "count-upload-pack"
        wrapper.write_text(
            "#!/usr/bin/env python3\nimport os, sys\n"
            + f"with open({str(counter)!r}, 'a') as log: log.write('fetch\\n')\n"
            + "os.execvp('git', ['git', 'upload-pack', *sys.argv[1:]])\n"
        )
        wrapper.chmod(0o755)
        subprocess.run(["git", "-C", str(clone), "config", "remote.origin.uploadpack", str(wrapper)], check=True)
        before = subprocess.check_output(["git", "-C", str(clone), "for-each-ref"])
        missing = subprocess.check_output(["git", "-C", str(clone), "rev-list", "--objects", "--missing=print", "--no-object-names", head + "^{tree}"])
        self.assertGreaterEqual(missing.count(b"?"), 50)
        prepare = runpy.run_path(str(SCRIPT))["prepare"]
        output = Path(self.temp.name) / "partial-context"
        prepare(str(clone), self.base, head, output)
        self.assertLessEqual(len(counter.read_text().splitlines()), 2)
        self.assertEqual(subprocess.check_output(["git", "-C", str(clone), "for-each-ref"]), before)
        self.assertEqual(subprocess.check_output(["git", "-C", str(clone), "rev-parse", "HEAD"]).decode().strip(), head)
        self.assertFalse((clone / ".git" / "FETCH_HEAD").exists())
        for index in range(50):
            self.assertEqual((output / "head" / "files" / f"partial-{index:03d}.txt").read_text(), f"promised file {index}\n")

    def test_rejects_invalid_shas_and_missing_merge_base(self) -> None:
        output = Path(self.temp.name) / "context"

        invalid = self.run_context(output, base="not-a-sha")
        self.assertNotEqual(invalid.returncode, 0)
        self.assertIn("SHA", invalid.stderr)

        tree = self.git("rev-parse", f"{self.head}^{{tree}}").stdout.strip()
        other_result = subprocess.run(
            ["git", "commit-tree", tree],
            cwd=self.repo,
            input="unrelated root\n",
            text=True,
            capture_output=True,
            check=True,
        )
        other = other_result.stdout.strip()

        no_base = self.run_context(output, head=other)
        self.assertNotEqual(no_base.returncode, 0)
        self.assertIn("merge base", no_base.stderr.lower())


if __name__ == "__main__":
    unittest.main()
