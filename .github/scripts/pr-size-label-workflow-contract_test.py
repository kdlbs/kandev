#!/usr/bin/env python3
"""Contract tests for the pull request size label workflow."""

from __future__ import annotations

import json
from pathlib import Path
import re
import subprocess
import sys
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "pr-size-label.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


def extract_function(source: str, name: str) -> str:
    """Return one complete JavaScript function from the trusted workflow."""

    marker = f"function {name}("
    start = source.find(marker)
    if start < 0:
        raise AssertionError(f"{name} is missing")

    body_start = source.find("{", start)
    if body_start < 0:
        raise AssertionError(f"{name} has no body")

    depth = 0
    quote: str | None = None
    escaped = False
    for index in range(body_start, len(source)):
        character = source[index]
        if quote is not None:
            if escaped:
                escaped = False
            elif character == "\\":
                escaped = True
            elif character == quote:
                quote = None
            continue

        if character in {"'", '"', "`"}:
            quote = character
        elif character == "{":
            depth += 1
        elif character == "}":
            depth -= 1
            if depth == 0:
                return source[start : index + 1]

    raise AssertionError(f"{name} has an incomplete body")


def run_javascript(function_source: str, function_name: str, values: list[object]) -> object:
    """Execute a pure workflow helper without loading the GitHub runtime."""

    script = (
        f"{function_source}\n"
        f"process.stdout.write(JSON.stringify({function_name}(...JSON.parse(process.argv[1]))));"
    )
    result = subprocess.run(
        ["node", "-e", script, json.dumps(values)],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        raise AssertionError(result.stderr or result.stdout)
    return json.loads(result.stdout)


def run_javascript_for_each(
    function_source: str, function_name: str, values: list[object]
) -> object:
    """Execute a pure workflow helper once for every value."""

    script = (
        f"{function_source}\n"
        "const values = JSON.parse(process.argv[1]);"
        f"process.stdout.write(JSON.stringify(values.map(value => {function_name}(value))));"
    )
    result = subprocess.run(
        ["node", "-e", script, json.dumps(values)],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        raise AssertionError(result.stderr or result.stdout)
    return json.loads(result.stdout)


class PullRequestSizeLabelWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.assertTrue(WORKFLOW.is_file(), "Pull request size workflow is missing")
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    # @covers AC-CI-PR-SIZE-001.1, AC-CI-PR-SIZE-001.9
    def test_recalculates_on_current_pull_request_events_and_serializes_by_number(self) -> None:
        trigger = self.workflow.partition("on:\n")[2].partition("\nconcurrency:")[0]
        self.assertEqual(
            trigger,
            "  pull_request_target: # zizmor: ignore[dangerous-triggers] "
            "base-controlled metadata-only workflow\n"
            "    types: [opened, reopened, synchronize]\n",
        )
        self.assertRegex(
            self.workflow,
            r"group: pr-size-label-\$\{\{ github\.event\.pull_request\.number \}\}",
        )
        self.assertIn("cancel-in-progress: false", self.workflow)

    # @covers AC-CI-PR-SIZE-001.2
    def test_classifier_assigns_boundaries_and_confirmed_examples(self) -> None:
        classifier = extract_function(self.workflow, "classifySize")
        expected_boundaries = {
            0: "small",
            10: "small",
            11: "medium",
            50: "medium",
            51: "big",
        }
        for count, expected in expected_boundaries.items():
            with self.subTest(count=count):
                self.assertEqual(run_javascript(classifier, "classifySize", [count]), expected)

        examples = {
            "small": (2, 7),
            "medium": (15, 17, 41),
            "big": (80, 96),
        }
        for expected, counts in examples.items():
            for count in counts:
                with self.subTest(expected=expected, count=count):
                    self.assertEqual(run_javascript(classifier, "classifySize", [count]), expected)

    # @covers AC-CI-PR-SIZE-001.3, AC-CI-PR-SIZE-001.4
    def test_file_classifier_counts_application_and_translation_paths_only(self) -> None:
        classifier = extract_function(self.workflow, "isCountedFile")
        counted = [
            "apps/backend/internal/workflow/workflow.go",
            "apps/backend/internal/workflow/workflow_test.go",
            "apps/web/src/components/task/task.tsx",
            "apps/web/src/locales/en/common.json",
            "apps/web/src/locales/zh-cn/common.json",
            "apps/desktop/src-tauri/src/main.rs",
        ]
        excluded = [
            "docs/specs/ci/requirements/pull-request-size-labels.md",
            ".github/workflows/pr-size-label.yml",
            "scripts/pr-size-label.py",
            "apps/web/src/assets/icon.svg",
            "apps/web/src/locales/en/common.yaml",
            "apps/web/node_modules/example/index.ts",
            "apps/backend/internal/generated/model.go",
            "apps/backend/internal/scripts/check.go",
            "apps/backend/cmd/sqlguard/check.go",
            "apps/backend/internal/db/sqlguard/check.go",
            "apps/web/eslint-rules/no-literal.ts",
            "apps/web/src/eslint.config.ts",
            "apps/web/src/vite.config.ts",
            "apps/web/src/playwright.setup.ts",
            "apps/web/src/tailwind.css",
        ]

        self.assertEqual(
            run_javascript_for_each(classifier, "isCountedFile", counted),
            [True] * len(counted),
        )
        self.assertEqual(
            run_javascript_for_each(classifier, "isCountedFile", excluded),
            [False] * len(excluded),
        )

    # @covers AC-CI-PR-SIZE-001.3, AC-CI-PR-SIZE-001.4
    def test_file_filter_is_explicit_and_normalizes_filenames(self) -> None:
        self.assertIn("typeof filename !== 'string'", self.workflow)
        self.assertIn("filename.startsWith('apps/')", self.workflow)
        self.assertIn("filename.split('/')", self.workflow)
        self.assertIn("scripts", self.workflow)
        self.assertIn("generated", self.workflow)
        self.assertIn("node_modules", self.workflow)
        for extension in (
            "c",
            "cc",
            "cpp",
            "css",
            "go",
            "graphql",
            "gql",
            "h",
            "html",
            "js",
            "jsx",
            "mjs",
            "mts",
            "proto",
            "rs",
            "scss",
            "sql",
            "ts",
            "tsx",
        ):
            self.assertRegex(self.workflow, rf"['\"]{extension}['\"]")
        self.assertIn(r"apps\/web\/src\/locales\/[^/]+\/[^/]+\.json", self.workflow)

    # @covers AC-CI-PR-SIZE-001.5, AC-CI-PR-SIZE-001.6
    def test_label_definitions_and_convergence_preserve_unrelated_labels(self) -> None:
        self.assertIn("const sizeLabelDefinitions =", self.workflow)
        for name, color in (
            ("small", "0E8A16"),
            ("medium", "FBCA04"),
            ("big", "D93F0B"),
        ):
            self.assertRegex(self.workflow, rf"{name}: \{{")
            self.assertIn(f"color: '{color}'", self.workflow)
        self.assertIn("github.rest.issues.getLabel", self.workflow)
        self.assertIn("github.rest.issues.createLabel", self.workflow)
        self.assertIn("github.rest.issues.listLabelsOnIssue", self.workflow)
        self.assertIn("github.rest.issues.addLabels", self.workflow)
        self.assertIn("github.rest.issues.removeLabel", self.workflow)

        stale_labels = extract_function(self.workflow, "staleSizeLabels")
        self.assertEqual(
            run_javascript(stale_labels, "staleSizeLabels", [["bug", "medium", "big"], "small"]),
            ["medium", "big"],
        )
        self.assertEqual(
            run_javascript(stale_labels, "staleSizeLabels", [["bug", "small"], "small"]),
            [],
        )

        add_index = self.workflow.index("github.rest.issues.addLabels")
        remove_index = self.workflow.index("github.rest.issues.removeLabel")
        self.assertLess(add_index, remove_index)

    # @covers AC-CI-PR-SIZE-001.7
    def test_incomplete_file_results_fail_before_label_mutations(self) -> None:
        self.assertIn("github.paginate(", self.workflow)
        self.assertIn("github.rest.pulls.listFiles", self.workflow)
        self.assertIn("per_page: 100", self.workflow)
        self.assertIn("pull_number: context.payload.pull_request.number", self.workflow)
        limit_index = self.workflow.index("changedFiles.length >= 3000")
        self.assertIn("throw new Error", self.workflow[limit_index : limit_index + 240])
        for mutation in (
            "github.rest.issues.getLabel",
            "github.rest.issues.createLabel",
            "github.rest.issues.addLabels",
            "github.rest.issues.removeLabel",
        ):
            self.assertLess(limit_index, self.workflow.index(mutation))

    # @covers AC-CI-PR-SIZE-001.8
    def test_workflow_uses_trusted_least_privilege_and_pinned_execution(self) -> None:
        permissions = self.workflow.partition("permissions:\n")[2].partition("\njobs:")[0]
        self.assertEqual("pull-requests: write", permissions.strip())
        self.assertNotIn("actions/checkout", self.workflow)
        self.assertNotIn("\n      - run:", self.workflow)
        self.assertNotIn("child_process", self.workflow)
        self.assertNotIn("eval(", self.workflow)
        self.assertRegex(
            self.workflow,
            r"uses: actions/github-script@[0-9a-f]{40} # v9\.[0-9]+\.[0-9]+",
        )

    def test_contract_is_registered_in_the_required_lint_workflow(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        self.assertRegex(
            lint_workflow,
            r"(?m)^      - name: Test pull request size label workflow contract$\n"
            r"^        run: python3 .github/scripts/pr-size-label-workflow-contract_test.py$",
        )


if __name__ == "__main__":
    unittest.main()
