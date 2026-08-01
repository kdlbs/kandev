#!/usr/bin/env python3
"""Behavior tests for the constrained PR-head file reader."""

import base64
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import textwrap
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
HELPER = REPO_ROOT / ".github" / "scripts" / "claude-read-pr-file.py"


class ClaudeReadPrFileTest(unittest.TestCase):
    def run_helper(self, path: str) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temp_dir:
            fake_gh = Path(temp_dir) / "gh"
            head_sha = "a" * 40
            content = base64.b64encode(b"const reviewed = true;\n").decode("ascii")
            binary_content = base64.b64encode(b"\xff").decode("ascii")
            fake_gh.write_text(
                textwrap.dedent(
                    f"""\
                    #!/usr/bin/env python3
                    import json
                    import sys

                    endpoint = sys.argv[-1]
                    if endpoint == "repos/kdlbs/kandev/pulls/2109":
                        print(json.dumps({{"head": {{"sha": "{head_sha}"}}}}))
                    elif endpoint == (
                        "repos/kdlbs/kandev/contents/apps/web/demo%20file.ts"
                        "?ref={head_sha}"
                    ):
                        print(json.dumps({{
                            "type": "file",
                            "path": "apps/web/demo file.ts",
                            "size": 23,
                            "encoding": "base64",
                            "content": "{content}",
                        }}))
                    elif endpoint.endswith("/contents/directory?ref={head_sha}"):
                        print(json.dumps({{"type": "dir", "path": "directory"}}))
                    elif endpoint.endswith("/contents/large.txt?ref={head_sha}"):
                        print(json.dumps({{
                            "type": "file",
                            "path": "large.txt",
                            "size": 262145,
                            "encoding": "base64",
                            "content": "",
                        }}))
                    elif endpoint.endswith("/contents/binary.dat?ref={head_sha}"):
                        print(json.dumps({{
                            "type": "file",
                            "path": "binary.dat",
                            "size": 1,
                            "encoding": "base64",
                            "content": "{binary_content}",
                        }}))
                    else:
                        print(f"unexpected endpoint: {{endpoint}}", file=sys.stderr)
                        raise SystemExit(2)
                    """
                ),
                encoding="utf-8",
            )
            fake_gh.chmod(0o755)
            env = os.environ.copy()
            env.update(
                {
                    "CLAUDE_REVIEW_PR_NUMBER": "2109",
                    "GITHUB_REPOSITORY": "kdlbs/kandev",
                    "PATH": f"{temp_dir}:{env['PATH']}",
                }
            )
            return subprocess.run(
                [sys.executable, str(HELPER), path],
                check=False,
                capture_output=True,
                env=env,
                text=True,
            )

    def test_reads_utf8_file_from_the_bound_pr_head(self) -> None:
        result = self.run_helper("apps/web/demo file.ts")

        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual("const reviewed = true;\n", result.stdout)

    def test_rejects_paths_outside_the_repository(self) -> None:
        result = self.run_helper("../runner-secret")

        self.assertNotEqual(0, result.returncode)
        self.assertIn("relative repository path", result.stderr)

    def test_rejects_non_file_content(self) -> None:
        result = self.run_helper("directory")

        self.assertNotEqual(0, result.returncode)
        self.assertIn("regular file", result.stderr)

    def test_rejects_oversized_content(self) -> None:
        result = self.run_helper("large.txt")

        self.assertNotEqual(0, result.returncode)
        self.assertIn("256 KiB", result.stderr)

    def test_rejects_non_utf8_content(self) -> None:
        result = self.run_helper("binary.dat")

        self.assertNotEqual(0, result.returncode)
        self.assertIn("UTF-8", result.stderr)

    def test_rejects_non_normalized_paths(self) -> None:
        result = self.run_helper("apps//web/file.ts")

        self.assertNotEqual(0, result.returncode)
        self.assertIn("relative repository path", result.stderr)


if __name__ == "__main__":
    unittest.main()
