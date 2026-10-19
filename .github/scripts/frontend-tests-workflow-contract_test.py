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


def frontend_job(workflow: str) -> str:
    """Return the frontend job body, up to the required gate."""
    _, separator, remainder = workflow.partition("  frontend:\n")
    if not separator:
        raise AssertionError("frontend-tests.yml has no frontend job")
    return re.split(r"\n  [\w-]+:\n", remainder)[0]


def frontend_gate_job(workflow: str) -> str:
    """Return the required frontend gate body."""
    _, separator, remainder = workflow.partition("  frontend-gate:\n")
    if not separator:
        raise AssertionError("frontend-tests.yml has no frontend-gate job")
    return remainder


def trigger_block(workflow: str, trigger: str) -> str:
    """Return `trigger`'s block under `on:`, up to the next top-level key."""
    block = re.search(rf"(?m)^  {trigger}:\n(?:^ {{4}}.*\n|^\n)*", workflow)
    if block is None:
        raise AssertionError(f"lint-action-pinning.yml has no {trigger} trigger")
    return block.group(0)


class FrontendTestsWorkflowContractTest(unittest.TestCase):
    def test_compiler_heap_is_scoped_to_the_full_typecheck(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        job = frontend_job(workflow)
        step = job.partition("      - name: Run typecheck\n")[2].partition(NEXT_STEP_MARKER)[0]

        self.assertIn("NODE_OPTIONS: --max-old-space-size=8192", step)
        self.assertIn("run: pnpm --filter @kandev/web typecheck", step)
        self.assertNotIn("NODE_OPTIONS", run_tests_step(workflow))
        self.assertNotIn("NODE_OPTIONS", job.partition("    steps:\n")[0])
        self.assertNotIn("NODE_OPTIONS", workflow.partition("jobs:\n")[0])

    def test_cache_resolves_the_container_pnpm_store_before_restore(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        job = frontend_job(workflow)
        resolve_index = job.find("- name: Resolve pnpm store path")
        cache_index = job.find("- name: Cache pnpm store")
        install_index = job.find("- name: Install dependencies")

        self.assertGreaterEqual(resolve_index, 0, "store resolution step is missing")
        self.assertGreaterEqual(cache_index, 0, "cache step is missing")
        self.assertGreaterEqual(install_index, 0, "dependency installation step is missing")
        self.assertLess(resolve_index, cache_index)
        self.assertLess(cache_index, install_index)
        resolve_step = job[resolve_index:cache_index]
        cache_step = job[cache_index:install_index]

        self.assertIn("id: pnpm-store", resolve_step)
        self.assertIn("pnpm store path --silent", resolve_step)
        self.assertIn('pnpm --version', resolve_step)
        self.assertIn('mkdir -p "${STORE_PATH}"', resolve_step)
        self.assertIn('printf \'path=%s\\n\' "${STORE_PATH}"', resolve_step)
        self.assertIn("path: ${{ steps.pnpm-store.outputs.path }}", cache_step)
        self.assertIn("runner.os", cache_step)
        self.assertIn("runner.arch", cache_step)
        self.assertIn("steps.pnpm-store.outputs.version", cache_step)
        self.assertIn("hashFiles('apps/pnpm-lock.yaml')", cache_step)
        self.assertIn(
            "restore-keys: pnpm-${{ runner.os }}-${{ runner.arch }}-${{ steps.pnpm-store.outputs.version }}-",
            cache_step,
        )
        self.assertIn("continue-on-error: true", cache_step)

    def test_cache_failure_does_not_replace_frozen_installation(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        job = frontend_job(workflow)
        cache_index = job.index("- name: Cache pnpm store")
        install_index = job.index("- name: Install dependencies")
        install_step = job[install_index:].partition(NEXT_STEP_MARKER)[0]

        self.assertLess(cache_index, install_index)
        self.assertIn("pnpm install --frozen-lockfile", install_step)

    def test_static_checks_and_unit_shards_are_independent(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        job = frontend_job(workflow)
        for name in ("Run lint", "Run typecheck", "Build"):
            self.assertIn(f"- name: {name}\n", job)
        self.assertNotIn("- name: Run tests\n", job)
        shards = workflow.split("  frontend_tests:\n", 1)[1].split("  frontend-gate:", 1)[0]
        self.assertIn("needs: [changes]", shards)
        self.assertIn("fail-fast: false", shards)
        self.assertIn("matrix: ${{ fromJSON(needs.changes.outputs.plan).frontend_tests_matrix }}", shards)
        self.assertIn("pnpm --filter @kandev/web run test --shard=", shards)
        self.assertIn("NODE_ENV: production", shards)
        self.assertIn("pnpm install --frozen-lockfile", shards)
        self.assertNotIn("continue-on-error", run_tests_step(workflow))

    def test_frontend_planner_covers_every_shard_once(self) -> None:
        import json
        workflow = WORKFLOW.read_text(encoding="utf-8")
        families = workflow.split("families: >-\n", 1)[1].split("burst:", 1)[0]
        family = next(f for f in json.loads(families) if f["name"] == "frontend_tests")
        self.assertEqual(family["tier"], "standard")
        self.assertEqual(family["instances"], [{"shard": n} for n in range(1, 3)])
        step = run_tests_step(workflow)
        self.assertIn('FRONTEND_TEST_SHARD: ${{ matrix.shard }}', step)
        self.assertIn('--shard="${FRONTEND_TEST_SHARD}/2"', step)

    def test_reports_are_required_and_partitioned(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        step = run_tests_step(workflow)
        self.assertIn('--reporter=json', step)
        self.assertIn('--outputFile="test-results-${FRONTEND_TEST_SHARD}.json"', step)
        upload = workflow.split("      - name: Upload frontend test report\n", 1)[1].split("\n  frontend-gate:", 1)[0]
        self.assertIn("if: always()", upload)
        self.assertIn("if-no-files-found: error", upload)
        self.assertIn("name: frontend-test-results-${{ matrix.shard }}", upload)
        self.assertIn("path: apps/web/test-results-${{ matrix.shard }}.json", upload)
        self.assertNotIn("continue-on-error", upload)

    def test_required_gate_requires_every_frontend_verification_result(self) -> None:
        gate = frontend_gate_job(WORKFLOW.read_text(encoding="utf-8"))
        self.assertIn("needs: [changes, frontend, frontend_tests]", gate)
        self.assertIn("FRONTEND_TESTS_RESULT: ${{ needs.frontend_tests.result }}", gate)
        self.assertIn('"frontend_tests:${FRONTEND_TESTS_RESULT}"', gate)

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
        self.assertIn("pnpm --filter @kandev/web run test", step)

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
