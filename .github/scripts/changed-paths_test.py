#!/usr/bin/env python3
"""Tests for the in-job `paths:` filter used by the merge-queue gating jobs.

Covers the matcher and the command-line contract the workflow steps rely on: a
`run=true` / `run=false` step output, and a hard failure rather than a silent
"nothing changed" when the pattern list is missing.
"""

from pathlib import Path
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))

from importlib import import_module

changed_paths = import_module("changed-paths")


SCRIPT = Path(__file__).resolve().parent / "changed-paths.py"


class MatcherTest(unittest.TestCase):
    def test_double_star_crosses_slashes_and_single_star_does_not(self) -> None:
        crossing = changed_paths.Filter(["apps/web/**"])
        self.assertTrue(crossing.includes("apps/web/components/deep/App.tsx"))

        flat = changed_paths.Filter(["apps/web/*"])
        self.assertTrue(flat.includes("apps/web/package.json"))
        self.assertFalse(flat.includes("apps/web/components/App.tsx"))

    def test_question_mark_matches_one_non_slash_character(self) -> None:
        path_filter = changed_paths.Filter(["apps/?eb/main.ts"])
        self.assertTrue(path_filter.includes("apps/web/main.ts"))
        self.assertFalse(path_filter.includes("apps//eb/main.ts"))

    def test_patterns_are_anchored_at_both_ends(self) -> None:
        path_filter = changed_paths.Filter(["Makefile"])
        self.assertTrue(path_filter.includes("Makefile"))
        self.assertFalse(path_filter.includes("apps/backend/Makefile"))
        self.assertFalse(path_filter.includes("Makefile.bak"))

    def test_last_matching_pattern_decides(self) -> None:
        """`!**/*.md` after an include is an exclusion, not an extra include."""
        path_filter = changed_paths.Filter(["apps/web/**", "!**/*.md"])

        self.assertFalse(path_filter.includes("apps/web/README.md"))
        self.assertTrue(path_filter.includes("apps/web/components/App.tsx"))
        # Order matters: reversing the list makes the negation dead.
        reversed_filter = changed_paths.Filter(["!**/*.md", "apps/web/**"])
        self.assertTrue(reversed_filter.includes("apps/web/README.md"))

    def test_a_single_relevant_file_selects_the_workflow(self) -> None:
        path_filter = changed_paths.Filter(["apps/web/**", "!**/*.md"])

        self.assertTrue(
            path_filter.matches_any(["apps/web/README.md", "apps/web/lib/api.ts"]),
            "A docs change bundled with a code change must still run the job.",
        )
        self.assertFalse(path_filter.matches_any(["apps/web/README.md"]))
        self.assertFalse(
            path_filter.matches_any([]),
            "An empty diff selects nothing; the caller decides whether an "
            "unresolvable base means 'run everything'.",
        )

    def test_blank_lines_and_comments_are_ignored(self) -> None:
        path_filter = changed_paths.Filter(["", "# a note", "apps/web/**", "  "])
        self.assertTrue(path_filter.includes("apps/web/main.tsx"))

    def test_unsupported_metacharacters_are_rejected(self) -> None:
        for pattern in ("apps/web/*.[jt]s", "apps/web/a+b", "apps/{web,cli}/**"):
            with self.subTest(pattern=pattern):
                with self.assertRaises(ValueError):
                    changed_paths.Filter([pattern])

    def test_an_empty_pattern_list_is_rejected(self) -> None:
        with self.assertRaises(ValueError):
            changed_paths.Filter(["", "# only comments"])


class CommandLineTest(unittest.TestCase):
    def run_script(self, patterns: str, changed: str) -> tuple[int, str, str]:
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "github_output"
            output.touch()
            env = {
                **os.environ,
                "PATTERNS": patterns,
                "GITHUB_OUTPUT": str(output),
            }
            result = subprocess.run(
                [sys.executable, str(SCRIPT)],
                input=changed,
                env=env,
                capture_output=True,
                text=True,
            )
            return result.returncode, result.stdout, output.read_text(encoding="utf-8")

    def test_writes_the_step_output(self) -> None:
        code, stdout, step_output = self.run_script(
            "apps/web/**\n!**/*.md\n", "apps/web/lib/api.ts\n"
        )
        self.assertEqual(code, 0)
        self.assertIn("run=true", stdout)
        self.assertEqual(step_output, "run=true\n")

    def test_reports_false_without_a_relevant_change(self) -> None:
        code, stdout, step_output = self.run_script(
            "apps/web/**\n!**/*.md\n", "docs/public/install.md\napps/web/README.md\n"
        )
        self.assertEqual(code, 0)
        self.assertIn("run=false", stdout)
        self.assertEqual(step_output, "run=false\n")

    def test_a_missing_pattern_variable_fails_loudly(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            env = {k: v for k, v in os.environ.items() if k != "PATTERNS"}
            env["GITHUB_OUTPUT"] = str(Path(tmp) / "out")
            result = subprocess.run(
                [sys.executable, str(SCRIPT)],
                input="apps/web/lib/api.ts\n",
                env=env,
                capture_output=True,
                text=True,
            )
        self.assertNotEqual(
            result.returncode,
            0,
            "An unset PATTERNS must fail the step. Defaulting to 'run nothing' "
            "would pass a merge queue entry that was never tested.",
        )


if __name__ == "__main__":
    unittest.main()
