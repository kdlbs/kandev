#!/usr/bin/env python3
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


class PRWalkthroughRenderTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.worktree = Path(self.tmp.name)
        self.skill = self.worktree / ".agents" / "skills" / "pr-walkthrough"
        (self.skill / "scripts").mkdir(parents=True)
        shutil.copy2(
            SKILL / "scripts" / "pr-walkthrough-render", self.skill / "scripts" / "pr-walkthrough-render"
        )
        shutil.copytree(SKILL / "references", self.skill / "references")
        self.script = self.skill / "scripts" / "pr-walkthrough-render"
        self.data = json.loads(
            (self.skill / "references" / "example.json").read_text(encoding="utf-8")
        )
        self.env = {
            **os.environ,
            "PR_NUMBER": "42",
            "PR_TITLE": "Trusted pull request title",
            "PR_URL": "https://github.com/kdlbs/kandev/pull/42",
            "PR_REPO": "kdlbs/kandev",
            "PR_BASE": "main",
            "PR_HEAD": "feature/walkthrough",
        }

    def run_render(
        self,
        data: object | None,
        *,
        stdin: str | None = None,
    ) -> subprocess.CompletedProcess[str]:
        draft = self.worktree / ".pr-walkthrough" / "draft.json"
        draft.parent.mkdir(parents=True, exist_ok=True)
        if data is None:
            draft.unlink(missing_ok=True)
        else:
            draft.write_text(json.dumps(data), encoding="utf-8")
        return subprocess.run(
            [sys.executable, str(self.script)],
            cwd=self.worktree,
            env=self.env,
            input=stdin,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_reads_only_the_fixed_draft_and_ignores_stdin(self) -> None:
        result = self.run_render(self.data, stdin=json.dumps({"pr": {}}))

        self.assertEqual(result.returncode, 0, result.stderr)
        rendered_data = json.loads(
            (
                self.worktree
                / "docs"
                / "pr-walkthrough"
                / "pr-42.json"
            ).read_text(encoding="utf-8")
        )
        self.assertEqual(rendered_data["pr"]["title"], "Trusted pull request title")

    def test_missing_draft_fails_without_partial_outputs(self) -> None:
        result = self.run_render(None, stdin=json.dumps(self.data))

        self.assertNotEqual(result.returncode, 0)
        output = self.worktree / "docs" / "pr-walkthrough"
        self.assertFalse((output / "pr-42.json").exists())
        self.assertFalse((output / "pr-42.html").exists())
        self.assertIn("draft.json", result.stderr)

    def test_binds_trusted_pr_identity_and_removes_model_link_overrides(self) -> None:
        self.data["pr"].update(
            {
                "number": 999,
                "title": "Untrusted title",
                "url": "javascript:alert(1)",
                "files_url": "https://attacker.invalid/files",
                "repo": "attacker/repo; curl attacker.invalid",
                "base": "wrong-base",
                "head": "wrong-head",
            }
        )
        self.data["changes"][0]["file_url"] = "javascript:alert(2)"

        result = self.run_render(self.data)

        self.assertEqual(result.returncode, 0, result.stderr)
        json_path = self.worktree / "docs" / "pr-walkthrough" / "pr-42.json"
        html_path = self.worktree / "docs" / "pr-walkthrough" / "pr-42.html"
        rendered_data = json.loads(json_path.read_text(encoding="utf-8"))
        self.assertEqual(
            rendered_data["pr"],
            {
                **self.data["pr"],
                "number": 42,
                "title": "Trusted pull request title",
                "url": "https://github.com/kdlbs/kandev/pull/42",
                "repo": "kdlbs/kandev",
                "base": "main",
                "head": "feature/walkthrough",
            }
            | {"files_url": "https://github.com/kdlbs/kandev/pull/42/files"},
        )
        self.assertNotIn("file_url", rendered_data["changes"][0])
        html = html_path.read_text(encoding="utf-8")
        self.assertIn("gh pr review 42 --repo kdlbs/kandev --approve", html)
        self.assertNotIn("attacker.invalid", html)
        self.assertNotIn("javascript:", html)

    def test_renderer_failure_leaves_no_partial_outputs(self) -> None:
        result = self.run_render({"pr": {}})

        self.assertNotEqual(result.returncode, 0)
        output = self.worktree / "docs" / "pr-walkthrough"
        self.assertFalse((output / "pr-42.json").exists())
        self.assertFalse((output / "pr-42.html").exists())
        self.assertIn("why.problem is required", result.stderr)


if __name__ == "__main__":
    unittest.main()
