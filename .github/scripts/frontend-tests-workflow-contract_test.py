#!/usr/bin/env python3
"""Contract tests for the frontend unit-test workflow's NODE_ENV guard.

React exports `act()` only from its development build, which it selects with
`process.env.NODE_ENV`. `apps/web/vitest.config.ts` pins that variable to
"test" so the development build always loads; without the pin, every
`render()` throws `TypeError: React.act is not a function` for anyone whose
shell inherits the runtime image's `NODE_ENV=production` (`Dockerfile`).

The pin cannot guard itself. The CI image sets no `NODE_ENV`, so Vitest's own
`process.env.NODE_ENV ??= "test"` already yields "test" there and the suite
passes whether or not the pin exists. `frontend-tests.yml` therefore exports
`NODE_ENV=production` on its test step on purpose, to reproduce the
environment that broke and make a deleted pin fail in CI.

These assertions exist because that export reads like a mistake: removing it
breaks nothing visibly, and CI silently stops guarding the fix.
"""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "frontend-tests.yml"
VITEST_CONFIG = REPO_ROOT / "apps" / "web" / "vitest.config.ts"
NPMRC = REPO_ROOT / "apps" / ".npmrc"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"

STEP_MARKER = "      - name: Run tests\n"
NEXT_STEP_MARKER = "\n      - name: "


def run_tests_step(workflow: str) -> str:
    """Return the body of the `Run tests` step, up to the following step."""
    _, separator, remainder = workflow.partition(STEP_MARKER)
    if not separator:
        raise AssertionError("frontend-tests.yml has no 'Run tests' step")
    return remainder.partition(NEXT_STEP_MARKER)[0]


def trigger_block(workflow: str, trigger: str) -> str:
    """Return `trigger`'s block under `on:`, up to the next top-level key."""
    block = re.search(rf"(?m)^  {trigger}:\n(?:^ {{4}}.*\n|^\n)*", workflow)
    if block is None:
        raise AssertionError(f"lint-action-pinning.yml has no {trigger} trigger")
    return block.group(0)


class FrontendTestsWorkflowContractTest(unittest.TestCase):
    def test_test_step_reproduces_the_production_environment(self) -> None:
        step = run_tests_step(WORKFLOW.read_text(encoding="utf-8"))

        self.assertIn(
            "NODE_ENV: production",
            step,
            "The 'Run tests' step must export NODE_ENV=production so a deleted "
            "NODE_ENV pin in apps/web/vitest.config.ts fails CI. Without it the "
            "CI image sets no NODE_ENV, Vitest defaults it to 'test', and the "
            "suite passes with or without the pin.",
        )
        self.assertIn("pnpm --filter @kandev/web test", step)

    def test_vitest_config_still_pins_the_mode(self) -> None:
        config = VITEST_CONFIG.read_text(encoding="utf-8")

        self.assertIn(
            'process.env.NODE_ENV = "test";',
            config,
            "apps/web/vitest.config.ts must pin NODE_ENV=test. React resolves "
            "its production build otherwise, which does not export act(), and "
            "every render()/renderHook() throws before reaching an assertion.",
        )

    def test_workspace_installs_keep_dev_dependencies(self) -> None:
        npmrc = NPMRC.read_text(encoding="utf-8")

        self.assertIn(
            "production=false",
            npmrc,
            "apps/.npmrc must set production=false. This is not for the "
            "workflow's own install step, which runs before the step-scoped "
            "export and sees no NODE_ENV. It is for every install performed "
            "under the same variable the test step reproduces: inside the "
            "runtime image (`Dockerfile`) or any shell inheriting it, pnpm "
            "otherwise skips devDependencies, leaving no vitest, eslint, or "
            "tsc to run at all.",
        )

    def test_this_contract_runs_when_its_subjects_change(self) -> None:
        """The assertions above are only worth anything if CI runs them.

        Two of this file's subjects, `apps/web/vitest.config.ts` and
        `apps/.npmrc`, live outside `.github/`. `lint-action-pinning.yml` used
        to name them in a hand-maintained `paths:` list so that a PR deleting
        `production=false` and nothing else still brought this job up; every
        new subject meant remembering to extend that list, and every omission
        was a silently unguarded file.

        The list is gone. `lint-action-pinning.yml` now runs on every push and
        every pull request, which subsumes the old guarantee for every subject
        at once -- and is what lets it be a merge-queue required check, since a
        path-filtered workflow reports no conclusion when it is skipped and a
        required check that never reports blocks the queue forever.

        The step assertion at the end is weaker than the trigger ones by
        construction: this file runs only from that step, so the PR that deletes
        it also deletes the only thing that would fail. It holds for local runs
        and for a step whose command is renamed or moved elsewhere. Keeping the
        job a required check is what covers outright removal, and that lives in
        the repository ruleset, not here.
        """
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")

        for trigger in ("push", "pull_request", "merge_group"):
            block = trigger_block(lint_workflow, trigger)

            self.assertNotIn(
                "    paths:",
                block,
                f"lint-action-pinning.yml's {trigger} trigger must stay "
                "unfiltered. A `paths:` list there reintroduces both failure "
                "modes it was removed for: subjects outside the list stop "
                "being guarded, and a skipped run reports no conclusion, which "
                "a merge queue waits on forever.",
            )

        self.assertIn(
            "\n        run: python3 .github/scripts/frontend-tests-workflow-contract_test.py\n",
            lint_workflow,
            "lint-action-pinning.yml must run this file, as a live step rather "
            "than a commented-out one. Without it this file is dead code and "
            "every assertion here silently stops guarding.",
        )


if __name__ == "__main__":
    unittest.main()
