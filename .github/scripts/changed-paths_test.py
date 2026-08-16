#!/usr/bin/env python3
"""Tests for the in-job `paths:` filter used by the merge-queue gating jobs.

Two halves. The first exercises the matcher directly. The second reads the
`PATTERNS` blocks out of the workflows that actually use it and asserts they
still select what their trigger-level `paths:` filters used to select -- a
regex that silently stops matching would hand every merge queue entry a full
E2E run, or worse, skip one that mattered.
"""

from pathlib import Path
import os
import re
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))

from importlib import import_module

changed_paths = import_module("changed-paths")


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = REPO_ROOT / ".github" / "workflows"
SCRIPT = Path(__file__).resolve().parent / "changed-paths.py"

# Every workflow that moved its filter into a `changes` job, with one path that
# must still select it and one that must not.
GATED_WORKFLOWS = {
    "backend-tests.yml": (
        ["apps/backend/internal/orchestrator/orchestrator.go"],
        ["apps/web/components/App.tsx", "apps/backend/AGENTS.md"],
    ),
    "frontend-tests.yml": (
        ["apps/web/components/App.tsx"],
        ["apps/backend/internal/orchestrator/orchestrator.go", "apps/web/README.md"],
    ),
    "e2e-tests.yml": (
        ["apps/web/e2e/specs/kanban.spec.ts"],
        ["docs/public/install.md", "README.md"],
    ),
}


def patterns_block(workflow: str) -> str:
    """Return the `PATTERNS:` heredoc-style YAML block of the `changes` job."""
    text = (WORKFLOWS / workflow).read_text(encoding="utf-8")
    match = re.search(r"(?m)^          PATTERNS: \|\n((?:^            .*\n|^\n)+)", text)
    if match is None:
        raise AssertionError(
            f"{workflow} has no `PATTERNS: |` block in its `changes` job. The "
            "gating job cannot filter anything without one, so every job it "
            "guards would run on every event -- or none would."
        )
    return match.group(1)


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


class WorkflowPatternsTest(unittest.TestCase):
    """The lists in the workflows must still mean what their filters meant."""

    def test_each_gating_workflow_selects_and_rejects_the_expected_paths(self) -> None:
        for workflow, (selected, rejected) in GATED_WORKFLOWS.items():
            with self.subTest(workflow=workflow):
                path_filter = changed_paths.Filter(patterns_block(workflow).splitlines())

                for path in selected:
                    self.assertTrue(
                        path_filter.includes(path),
                        f"{workflow} must still run for {path}.",
                    )
                for path in rejected:
                    self.assertFalse(
                        path_filter.includes(path),
                        f"{workflow} must not be selected by {path} alone.",
                    )

    def test_every_gating_workflow_filters_on_its_own_definition(self) -> None:
        """A change to the workflow file must run the jobs it defines.

        This is the trigger-level `paths:` entry each of these workflows
        carried for itself. Losing it means a PR that rewrites the E2E workflow
        never runs it.
        """
        for workflow in GATED_WORKFLOWS:
            with self.subTest(workflow=workflow):
                path_filter = changed_paths.Filter(patterns_block(workflow).splitlines())
                self.assertTrue(
                    path_filter.includes(f".github/workflows/{workflow}"),
                    f"{workflow} must select itself.",
                )

    def test_gating_workflows_carry_no_trigger_level_path_filter(self) -> None:
        """The whole point: these workflows must always report a conclusion.

        A `paths:` key back on `pull_request` or `merge_group` reintroduces the
        pending-forever check that blocks a merge queue.
        """
        for workflow in GATED_WORKFLOWS:
            with self.subTest(workflow=workflow):
                text = (WORKFLOWS / workflow).read_text(encoding="utf-8")
                triggers = text.partition("\nconcurrency:")[0]
                self.assertNotIn(
                    "\n    paths:",
                    triggers,
                    f"{workflow} must not filter its triggers by path. The "
                    "filter belongs in the `changes` job, where a skip still "
                    "reports a conclusion.",
                )
                self.assertIn(
                    "\n  merge_group:\n",
                    triggers,
                    f"{workflow} must keep its merge_group trigger; without it "
                    "the merge queue waits forever for a check that never runs.",
                )


if __name__ == "__main__":
    unittest.main()
