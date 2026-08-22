#!/usr/bin/env python3
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "pr-walkthrough-render"
REFERENCES = ROOT / ".agents" / "skills" / "pr-walkthrough" / "references"


class PRWalkthroughRenderTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.worktree = Path(self.tmp.name)
        trusted = self.worktree / ".opencode-walkthrough"
        trusted.mkdir()
        shutil.copy2(REFERENCES / "build.py", trusted / "build.py")
        shutil.copy2(REFERENCES / "shell.html", trusted / "shell.html")
        self.data = json.loads((REFERENCES / "example.json").read_text(encoding="utf-8"))
        self.env = {
            **os.environ,
            "PR_NUMBER": "42",
            "PR_TITLE": "Trusted pull request title",
            "PR_URL": "https://github.com/kdlbs/kandev/pull/42",
            "PR_REPO": "kdlbs/kandev",
            "PR_BASE": "main",
            "PR_HEAD": "feature/walkthrough",
        }

    def run_render(self, data: object) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT)],
            cwd=self.worktree,
            env=self.env,
            input=json.dumps(data),
            text=True,
            capture_output=True,
            check=False,
        )

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
