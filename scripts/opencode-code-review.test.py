#!/usr/bin/env python3
import json
import os
import re
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "opencode-code-review"


class OpenCodeReviewScriptTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.workdir = Path(self.tmp.name)
        self.calls_path = self.workdir / "gh-calls.jsonl"
        self.output_path = self.workdir / "output.txt"
        self.files_path = self.workdir / "files.txt"
        self.status_path = self.workdir / "status.txt"
        self.stdout_path = self.workdir / "stdout.txt"
        self.stderr_path = self.workdir / "stderr.txt"
        self.summary_path = self.workdir / "summary.md"
        self.files_path.write_text("src/app.ts\n", encoding="utf-8")
        self.status_path.write_text("0\n", encoding="utf-8")
        self.stdout_path.write_text("stdout line\n", encoding="utf-8")
        self.stderr_path.write_text("stderr line\n", encoding="utf-8")
        self.summary_path.write_text("", encoding="utf-8")
        self.fake_gh = self.write_fake_gh()

    def write_fake_gh(self) -> Path:
        fake_gh = self.workdir / "gh"
        fake_gh.write_text(
            textwrap.dedent(
                """\
                #!/usr/bin/env python3
                import json
                import os
                import sys
                from pathlib import Path

                calls_path = Path(os.environ["GH_CALLS"])
                args = sys.argv[1:]
                calls_path.write_text(
                    calls_path.read_text() + json.dumps(args) + "\\n"
                    if calls_path.exists()
                    else json.dumps(args) + "\\n"
                )

                if "comments?per_page=100" in " ".join(args):
                    print("[]")
                    raise SystemExit(0)

                joined = " ".join(args)
                if "/pulls/" in joined and os.environ.get("INLINE_FAIL") == "1":
                    print("line is not part of the diff", file=sys.stderr)
                    raise SystemExit(1)
                if "/issues/" in joined and os.environ.get("ISSUE_COMMENT_FAIL") == "1":
                    print("issue comments unavailable", file=sys.stderr)
                    raise SystemExit(1)

                print("{}")
                """
            ),
            encoding="utf-8",
        )
        fake_gh.chmod(0o755)
        return fake_gh

    def run_script(
        self,
        *,
        output: str,
        inline_fail: bool = False,
        issue_comment_fail: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        self.output_path.write_text(output, encoding="utf-8")
        env = {
            **os.environ,
            "GH_BIN": str(self.fake_gh),
            "GH_CALLS": str(self.calls_path),
            "GITHUB_REPOSITORY": "kdlbs/kandev",
            "PR_NUMBER": "42",
            "HEAD_SHA": "0123456789abcdef",
            "OPENCODE_MODEL": "opencode-go/minimax-m3",
        }
        if inline_fail:
            env["INLINE_FAIL"] = "1"
        if issue_comment_fail:
            env["ISSUE_COMMENT_FAIL"] = "1"
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "post-findings",
                "--output",
                str(self.output_path),
                "--files",
                str(self.files_path),
                "--opencode-status",
                str(self.status_path),
                "--stdout",
                str(self.stdout_path),
                "--stderr",
                str(self.stderr_path),
                "--summary",
                str(self.summary_path),
            ],
            text=True,
            capture_output=True,
            env=env,
            check=False,
        )

    def read_calls(self) -> list[list[str]]:
        if not self.calls_path.exists():
            return []
        return [json.loads(line) for line in self.calls_path.read_text().splitlines()]

    def bodies(self) -> list[str]:
        bodies = []
        for call in self.read_calls():
            for index, arg in enumerate(call):
                if arg == "-f" and index + 1 < len(call) and call[index + 1].startswith("body="):
                    bodies.append(call[index + 1][len("body=") :])
        return bodies

    def test_missing_findings_block_fails_and_posts_diagnostic(self) -> None:
        result = self.run_script(output="OpenCode refused to read external_directory (/tmp/*)\n")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("did not produce parseable findings", result.stderr)
        self.assertTrue(
            any("<!-- opencode-review:diagnostic -->" in body for body in self.bodies()),
            "expected a stable diagnostic PR comment",
        )
        self.assertIn("No parseable findings block", self.summary_path.read_text())

    def test_invalid_findings_json_fails_and_posts_diagnostic(self) -> None:
        result = self.run_script(output="<opencode_findings>{}</opencode_findings>\n")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("did not produce parseable findings", result.stderr)
        self.assertTrue(any("<!-- opencode-review:diagnostic -->" in body for body in self.bodies()))
        self.assertIn("not an array", self.summary_path.read_text())

    def test_valid_empty_array_posts_no_findings_comment(self) -> None:
        result = self.run_script(output="<opencode_findings>[]</opencode_findings>\n")

        self.assertEqual(result.returncode, 0, result.stderr)
        bodies = self.bodies()
        self.assertTrue(any("<!-- opencode-review:no-findings -->" in body for body in bodies))
        self.assertFalse(any("<!-- opencode-review:diagnostic -->" in body for body in bodies))

    def test_inline_failures_are_posted_as_one_fallback_comment(self) -> None:
        result = self.run_script(
            output=textwrap.dedent(
                """\
                <opencode_findings>
                [{"path":"src/app.ts","line":99,"title":"Bad line","body":"This line moved."}]
                </opencode_findings>
                """
            ),
            inline_fail=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        bodies = self.bodies()
        self.assertTrue(any("<!-- opencode-review:fallback-findings -->" in body for body in bodies))
        self.assertTrue(any("src/app.ts:99" in body for body in bodies))

    def test_inline_findings_beyond_limit_are_preserved_in_fallback_comment(self) -> None:
        findings = [
            {"path": "src/app.ts", "line": index + 1, "title": f"Finding {index + 1}", "body": "body"}
            for index in range(30)
        ]

        result = self.run_script(
            output=f"<opencode_findings>{json.dumps(findings)}</opencode_findings>\n",
            inline_fail=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        bodies = self.bodies()
        self.assertTrue(any("<!-- opencode-review:fallback-findings -->" in body for body in bodies))
        self.assertTrue(any("src/app.ts:1" in body for body in bodies))
        self.assertTrue(any("src/app.ts:30" in body for body in bodies))
        self.assertTrue(any("## GitHub rejected inline placement" in body for body in bodies))
        self.assertTrue(any("## Additional findings beyond inline comment limit" in body for body in bodies))
        summary = self.summary_path.read_text()
        self.assertIn("Inline comments rejected: `20`", summary)
        self.assertIn("Findings beyond inline limit: `10`", summary)
        self.assertIn("Fallback findings included in comment: `30`", summary)
        self.assertIn("Fallback findings omitted from comment: `0`", summary)

    def test_fallback_comment_reports_findings_omitted_by_body_limit(self) -> None:
        findings = [
            {"path": "src/app.ts", "line": index + 1, "title": f"Finding {index + 1}", "body": "x" * 4000}
            for index in range(30)
        ]

        result = self.run_script(
            output=f"<opencode_findings>{json.dumps(findings)}</opencode_findings>\n",
            inline_fail=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        bodies = self.bodies()
        self.assertTrue(any("Additional fallback findings omitted" in body for body in bodies))
        summary = self.summary_path.read_text()
        self.assertRegex(summary, r"Fallback findings included in comment: `1[0-9]`")
        omitted_match = re.search(r"Fallback findings omitted from comment: `([1-9][0-9]*)`", summary)
        self.assertIsNotNone(omitted_match, summary)

    def test_fallback_comment_failure_fails_the_step(self) -> None:
        result = self.run_script(
            output=textwrap.dedent(
                """\
                <opencode_findings>
                [{"path":"src/app.ts","line":99,"title":"Bad line","body":"This line moved."}]
                </opencode_findings>
                """
            ),
            inline_fail=True,
            issue_comment_fail=True,
        )

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Could not post OpenCode comment", result.stderr)


if __name__ == "__main__":
    unittest.main()
