#!/usr/bin/env python3
"""Contract for the targeted lint invocation in the upgrade-regression workflow.

The workflow lints a fixed set of upgrade harness files. It must use the same
native configuration and zero-warning policy as the required web lint command,
without silently reverting to the ESLint CLI.
"""

from __future__ import annotations

import pathlib
import re
import unittest

WORKFLOW = pathlib.Path(__file__).resolve().parents[1] / "workflows" / "upgrade-regression.yml"
FILES = [
    "e2e/fixtures/upgrade-backend.ts",
    "e2e/fixtures/upgrade-backend.test.ts",
    "e2e/helpers/upgrade-*.ts",
    "e2e/tests/upgrade/*.ts",
    "e2e/upgrade-global-setup.ts",
    "e2e/upgrade.vitest.config.ts",
    "e2e/playwright.config.ts",
    "e2e/playwright.config.test.ts",
]


class UpgradeRegressionLintContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.text = WORKFLOW.read_text(encoding="utf-8")

    def test_harness_step_uses_native_configuration_and_selection(self) -> None:
        expected = (
            "(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings "
            + " ".join(FILES)
            + ")"
        )
        self.assertIn(expected, self.text)

    def test_harness_step_has_no_eslint_cli_fallback(self) -> None:
        self.assertNotIn("pnpm exec eslint", self.text)
        self.assertNotIn("eslint e2e/", self.text)

    def test_workflow_permissions_and_triggers_are_unchanged(self) -> None:
        self.assertRegex(self.text, r"(?m)^permissions:\n  contents: read$")
        self.assertRegex(self.text, r"(?m)^  push:\n    branches: \[main\]$")
        for trigger in ("schedule", "workflow_dispatch"):
            self.assertRegex(self.text, rf"(?m)^  {trigger}:$")
        jobs = re.findall(r"(?m)^  (prepare|rehearse):$", self.text)
        self.assertEqual(jobs, ["prepare", "rehearse"])


if __name__ == "__main__":
    unittest.main()
