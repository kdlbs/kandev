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

# The subjects above that live outside `.github/`, and so are not covered by
# this job's `.github/workflows/**` trigger.
UNTRIGGERED_SUBJECTS = ("apps/web/vitest.config.ts", "apps/.npmrc")


def run_tests_step(workflow: str) -> str:
    """Return the body of the `Run tests` step, up to the following step."""
    _, separator, remainder = workflow.partition(STEP_MARKER)
    if not separator:
        raise AssertionError("frontend-tests.yml has no 'Run tests' step")
    return remainder.partition(NEXT_STEP_MARKER)[0]


def trigger_paths(workflow: str, trigger: str) -> str:
    """Return the `paths:` list of `trigger`'s block under `on:`.

    Scoped to the `paths:` key rather than the whole trigger block. GitHub
    rejects `paths` and `paths-ignore` on the same event, so renaming the key
    inverts the filter wholesale: the job would then run on everything *except*
    these files, and a match against the block as a whole would not notice.
    """
    block = re.search(rf"(?m)^  {trigger}:\n(?:^ {{4}}.*\n|^\n)*", workflow)
    if block is None:
        raise AssertionError(f"lint-action-pinning.yml has no {trigger} trigger")

    paths = re.search(r"(?m)^    paths:\n((?:^      - .*\n)+)", block.group(0))
    if paths is None:
        raise AssertionError(
            f"lint-action-pinning.yml's {trigger} trigger has no `paths:` list. A "
            "`paths-ignore:` key in its place inverts the filter, and the job stops "
            "running for the very files listed under it."
        )
    return paths.group(1)


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

        `lint-action-pinning.yml` triggers on `.github/workflows/**`, which
        covers `frontend-tests.yml` but neither subject in
        `UNTRIGGERED_SUBJECTS`. `frontend-tests.yml` does not cover
        `apps/.npmrc` either, so without an explicit trigger here a PR that
        deletes `production=false` and nothing else runs no job at all and the
        assertion above never executes. Mirrors the equivalent test in
        `release-workflow-contract_test.py`.

        The step assertion at the end is weaker than the path ones by
        construction: this file runs only from that step, so the PR that deletes
        it also deletes the only thing that would fail. It holds for local runs
        and for a step whose command is renamed or moved elsewhere. Keeping the
        job a required check is what covers outright removal, and that lives in
        branch protection, not here.
        """
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")

        for trigger in ("push", "pull_request"):
            paths = trigger_paths(lint_workflow, trigger)

            for subject in UNTRIGGERED_SUBJECTS:
                self.assertIn(
                    f'      - "{subject}"\n',
                    paths,
                    f"{subject} must be a {trigger} path trigger in "
                    "lint-action-pinning.yml. It is a subject of this contract "
                    "but lives outside .github/, so nothing else brings this "
                    "job up when it changes.",
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
