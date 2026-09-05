#!/usr/bin/env python3
"""Behavior tests for the bounded GitHub Actions step-summary writer."""

from pathlib import Path
import subprocess
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / ".github" / "scripts" / "bounded-step-summary.py"


class BoundedStepSummaryTest(unittest.TestCase):
    def run_writer(self, summary: str, *, max_bytes: int = 983_040) -> bytes:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            source = tmp_path / "generated-summary.md"
            output = tmp_path / "github-step-summary.md"
            source.write_text(summary, encoding="utf-8")

            result = subprocess.run(
                [
                    "python3",
                    str(SCRIPT),
                    "--input",
                    str(source),
                    "--output",
                    str(output),
                    "--diagnostics-label",
                    "backend-test-results-1",
                    "--diagnostics-url",
                    "https://github.example.test/kdlbs/kandev/actions/runs/123#artifacts",
                    "--max-bytes",
                    str(max_bytes),
                ],
                capture_output=True,
                text=True,
                check=False,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            return output.read_bytes()

    def test_normal_summary_is_unchanged(self) -> None:
        summary = "## 📝 Test results\n\n🔴 `TestWorkspaceFailure` failed\n"

        self.assertEqual(self.run_writer(summary), summary.encode("utf-8"))

    def test_oversized_summary_is_bounded_explicit_and_utf8_safe(self) -> None:
        summary = (
            "## 📝 Test results\n\n"
            "🔴 `TestWorkspaceFailure` failed\n\n"
            "boundary-padding-12"
            + "diagnostic=失敗🔥\n" * 60_000
        )
        source_bytes = summary.encode("utf-8")
        self.assertGreater(len(source_bytes), 1_048_576)

        first = self.run_writer(summary)
        second = self.run_writer(summary)
        rendered = first.decode("utf-8")

        self.assertLess(len(first), 983_040)
        self.assertEqual(first, second)
        self.assertIn("TestWorkspaceFailure", rendered)
        self.assertIn("truncated", rendered.lower())
        self.assertIn("backend-test-results-1", rendered)
        self.assertIn(
            "https://github.example.test/kdlbs/kandev/actions/runs/123#artifacts",
            rendered,
        )


if __name__ == "__main__":
    unittest.main()
