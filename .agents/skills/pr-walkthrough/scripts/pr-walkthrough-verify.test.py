#!/usr/bin/env python3
"""Tests for the managed PR walkthrough completion verifier."""

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[4]
SKILL = ROOT / ".agents" / "skills" / "pr-walkthrough"


class PRWalkthroughVerifyTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.worktree = Path(self.tmp.name)
        self.skill = self.worktree / ".agents" / "skills" / "pr-walkthrough"
        (self.skill / "scripts").mkdir(parents=True)
        shutil.copy2(
            SKILL / "scripts" / "pr-walkthrough-render",
            self.skill / "scripts" / "pr-walkthrough-render",
        )
        verifier_source = SKILL / "scripts" / "pr-walkthrough-verify"
        if verifier_source.is_file():
            shutil.copy2(verifier_source, self.skill / "scripts" / verifier_source.name)
        shutil.copytree(SKILL / "references", self.skill / "references")
        self.render_script = self.skill / "scripts" / "pr-walkthrough-render"
        self.verify_script = self.skill / "scripts" / "pr-walkthrough-verify"
        self.data = json.loads(
            (self.skill / "references" / "example.json").read_text(encoding="utf-8")
        )
        manifest_paths = {
            item["file"]
            for category in self.data["impact"].values()
            for item in category.get("items", [])
        }
        manifest_paths.update(change["file"] for change in self.data["changes"])
        manifest = {"files": [{"path": path} for path in sorted(manifest_paths)]}
        manifest_dir = self.worktree / ".pr-walkthrough" / "head-context"
        manifest_dir.mkdir(parents=True, exist_ok=True)
        (manifest_dir / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
        self.env = {
            **os.environ,
            "PR_NUMBER": "42",
            "PR_TITLE": "Trusted pull request title",
            "PR_URL": "https://github.com/kdlbs/kandev/pull/42",
            "PR_REPO": "kdlbs/kandev",
            "PR_BASE": "main",
            "PR_HEAD": "feature/walkthrough",
            "HEAD_SHA": "a" * 40,
        }
        self.draft_path = self.worktree / ".pr-walkthrough" / "draft.json"
        self.receipt_path = self.worktree / ".pr-walkthrough" / "render-complete.json"
        self.json_path = self.worktree / "docs" / "pr-walkthrough" / "pr-42.json"
        self.html_path = self.worktree / "docs" / "pr-walkthrough" / "pr-42.html"

    def render_valid_pair(self) -> None:
        self.draft_path.write_text(json.dumps(self.data), encoding="utf-8")
        result = subprocess.run(
            [sys.executable, str(self.render_script)],
            cwd=self.worktree,
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def run_verify(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(self.verify_script), *args],
            cwd=self.worktree,
            env=self.env,
            text=True,
            capture_output=True,
            check=False,
        )

    def read_receipt(self) -> dict[str, object]:
        return json.loads(self.receipt_path.read_text(encoding="utf-8"))

    def update_hash(self, key: str, path: Path) -> None:
        receipt = self.read_receipt()
        receipt[key] = hashlib.sha256(path.read_bytes()).hexdigest()
        self.receipt_path.write_text(json.dumps(receipt), encoding="utf-8")

    def test_verifies_generated_pair_without_changing_outputs(self) -> None:
        self.render_valid_pair()
        before = {
            path: path.read_bytes()
            for path in (self.draft_path, self.json_path, self.html_path, self.receipt_path)
        }

        result = self.run_verify()

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(before, {path: path.read_bytes() for path in before})

    def test_rejects_receipt_for_another_event_head(self) -> None:
        self.render_valid_pair()
        receipt = self.read_receipt()
        receipt["head_sha"] = "b" * 40
        self.receipt_path.write_text(json.dumps(receipt), encoding="utf-8")

        result = self.run_verify()

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("receipt identity does not match", result.stderr)

    def test_rejects_symlinked_output(self) -> None:
        self.render_valid_pair()
        target = self.worktree / ".pr-walkthrough" / "saved.html"
        self.html_path.replace(target)
        self.html_path.symlink_to(target)

        result = self.run_verify()

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("must be a regular file", result.stderr)

    def test_rejects_missing_or_mutated_output(self) -> None:
        self.render_valid_pair()
        self.html_path.unlink()
        missing = self.run_verify()
        self.assertNotEqual(missing.returncode, 0)
        self.assertIn("html output is missing", missing.stderr)

        self.render_valid_pair()
        with self.json_path.open("a", encoding="utf-8") as stream:
            stream.write(" ")
        mutated = self.run_verify()
        self.assertNotEqual(mutated.returncode, 0)
        self.assertIn("JSON output hash does not match", mutated.stderr)

    def test_rejects_invalid_schema_even_when_receipt_hash_matches(self) -> None:
        self.render_valid_pair()
        data = json.loads(self.json_path.read_text(encoding="utf-8"))
        data["changes"][0]["title"] = ""
        self.json_path.write_text(json.dumps(data), encoding="utf-8")
        self.update_hash("json_sha256", self.json_path)

        result = self.run_verify()

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("changes[0].title is required", result.stderr)

    def test_rejects_html_that_does_not_match_verified_json(self) -> None:
        self.render_valid_pair()
        with self.html_path.open("a", encoding="utf-8") as stream:
            stream.write("stale")
        self.update_hash("html_sha256", self.html_path)

        result = self.run_verify()

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("HTML output does not match the trusted renderer", result.stderr)

    def test_rejects_command_arguments(self) -> None:
        result = self.run_verify("unexpected")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not accept command arguments", result.stderr)


if __name__ == "__main__":
    unittest.main()
