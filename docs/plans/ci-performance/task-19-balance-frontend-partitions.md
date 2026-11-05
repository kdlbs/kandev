---
id: "19-balance-frontend-partitions"
title: "Balance existing frontend partitions"
status: completed
wave: 3
depends_on: ["18-refresh-linux-go-caches"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-003.1
  - AC-PLATFORM-CI-PERFORMANCE-003.2
  - AC-PLATFORM-CI-PERFORMANCE-003.3
  - AC-PLATFORM-CI-PERFORMANCE-003.6
  - AC-PLATFORM-CI-PERFORMANCE-003.7
  - AC-PLATFORM-CI-PERFORMANCE-005.6
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 19: Balance existing frontend partitions

## Summary

Use measured file costs to balance the existing two frontend test jobs.
Current native discovery remains authoritative. Both jobs consume one frozen timing input.

## In scope

- Capture the current two-shard performance baseline before adding timing instrumentation.
  Exclude that pre-instrumentation run from `compare`, which requires timing-enabled artifacts from both sides.
- Add bounded profile, snapshot, deterministic allocation, and result-reconciliation helpers without package dependencies.
- Restore an optional compatible timing profile and publish one immutable input artifact inside `changes`.
- Publish producer identity and digest outputs. Preserve the original producer identity during a single-shard rerun.
- Add a CI-only `BaseSequencer` subclass with native fallback and complete disjoint current-file allocation.
- Add a CI-only reporter for public module diagnostics, actual completed identities, and native outcomes.
- Preserve default within-shard sorting and native local/unsharded behavior.
- Add profile and assignment evidence to the existing per-shard report artifacts.
- Reconcile both artifacts inside the existing gate without pnpm installation, new permissions, or another job.
- Publish compatible timing profiles only after successful push-to-main verification.
- Include the new shared helper and tests in frontend change detection and the existing helper-test workflow.

## Out of scope

- More shards, new jobs, worker changes, altered environments, disabled isolation, or test-result caching.
- Test-only exemptions, omitted assertions, changed retries, longer timeouts, and application UI.
- Persistent bytecode caches, remote services, new fleet capacity, or changed gate placement.
- Adoption based on file-duration estimates alone. Task 20 owns the hosted performance decision.

## Acceptance

1. Valid profiles balance the same complete native file inventory across two jobs. Cold or invalid profiles select one shared deterministic fallback.
2. Native fixtures prove exact file/test identity and outcome parity across projects, including new/changed files and single-shard reruns.
3. The existing gate rejects failed, cancelled, missing, duplicate, or inconsistent evidence. It preserves deliberate skips and optional cache publication.

## Test cases

- Allocation: stable ordering, tied weights, empty/one-file fixtures, changed hashes, absent estimates, new projects, and complete disjoint union.
- Profile validation: oversize input, too many entries, traversal, duplicate identities, malformed hashes, nonfinite/negative durations, and incompatible schemas.
- Snapshot use: common fallback, corrupt digest, foreign source/run/producer, absent artifact, and reused successful siblings during reruns.
- Native behavior: real Vitest discovery across all three existing projects, nested tests, existing skips, collection errors, and unchanged unsharded/filtered local commands.
- Reporter: separate diagnostics, no double-counted import details, incomplete modules, native failure exits, and every required current file.
- Gate: missing shard, duplicate file, wrong source/snapshot, failed static job, cancelled tests, irrelevant-change skip, and optional cache failure.

Use an isolated small Vitest fixture through the public runner API for native integration cases.
Pure planner tests alone do not prove that the actual sequencer and reporter preserve selection.

## Verification

Run from the repository root after the planned files exist:

```bash
node --test .github/scripts/frontend-timings.test.mjs
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/ci-gates_test.py
python3 .github/scripts/lint-action-pinning_test.py
(cd apps/web && pnpm exec vitest run scripts/vitest-ci-sequencer.test.ts scripts/vitest-ci-reporter.test.ts scripts/vitest-project-selection.test.ts scripts/vitest-worker-budget.test.ts vitest-environment.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint scripts/vitest-ci-sequencer.ts scripts/vitest-ci-reporter.ts scripts/vitest-ci-sequencer.test.ts scripts/vitest-ci-reporter.test.ts vitest.config.ts)
(cd apps/web && pnpm run i18n:ratchet)
actionlint .github/workflows/frontend-tests.yml .github/workflows/lint-action-pinning.yml
zizmor .github/workflows
git diff --check
```

A fresh worktree first needs `(cd apps && pnpm install --frozen-lockfile)`.
Use the existing worker budget and one local suite at a time.
Record native fixture identities and actual focused results. Full hosted parity and performance remain Task 20 acceptance.

## Files likely touched

- `.github/scripts/frontend-timings.mjs` and `.github/scripts/frontend-timings.test.mjs` (new).
- `apps/web/scripts/vitest-ci-sequencer.ts` and `apps/web/scripts/vitest-ci-sequencer.test.ts` (new).
- `apps/web/scripts/vitest-ci-reporter.ts` and `apps/web/scripts/vitest-ci-reporter.test.ts` (new).
- `apps/web/vitest.config.ts`.
- `.github/workflows/frontend-tests.yml`.
- `.github/scripts/frontend-tests-workflow-contract_test.py` and `.github/scripts/ci-gates_test.py`.
- `.github/workflows/lint-action-pinning.yml`: register helper fixtures in the existing job.
- `docs/plans/ci-performance/evidence-2026-10-26.md`.

## Dependencies

Task 18 completes the preceding cache candidate before another workflow experiment starts.
The existing runner allocator, project selection, and repaired frontend dependency cache remain in place.

## Risks

Bootstrap, artifact, and gate overhead can erase balancing gains.
Historical durations can vary with runner class and imports. Current tests always run despite missing estimates.
An advisory profile can be invalid. Only the shared producer can choose fallback.
The implementation must preserve the existing contributor and external-runner trust boundaries.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [Design](../../specs/platform/system-design/ci-performance.md#october-26-cache-and-partition-continuation).
- Installed Vitest 4.1.11 `BaseSequencer`, `TestSpecification`, `TestModule.diagnostic()`, and reporter declarations.
- Existing `vitest.config.ts`, project-selection guards, frontend workflow contracts, and artifact retry action.
- [Diagnostic evidence](evidence-2026-10-26.md).

## Results

Implemented the bounded timing profile, frozen snapshot, deterministic two-shard allocation, CI-only Vitest sequencer/reporter, native report reconciliation, and main-only profile publication in the existing frontend workflow. Test selection and job graph remain unchanged. The reporter keeps retries and flaky-test counts available to Task 20; comparison output summarizes pass/fail/skip/pending/todo/disabled outcomes and retry counts.

Review fixes close four implementation gaps: the gate checks out the workflow SHA before its conditional local-action calls; profile restore and save use the same workspace-relative directory and compatible key prefix; valid count-fallback snapshots reuse Vitest's native SHA-1 shard placement across all three projects; and enabled CI consumers fail before assignment on missing, unreadable, invalid, foreign, or digest-mismatched snapshots. Disabled and unsharded runs retain native behavior.

Local checks passed: `.github/scripts/frontend-timings.test.mjs` (9 tests, including the compare CLI); frontend and gate contracts (16 and 5 tests); action pinning (9 tests); backend/E2E contracts (26 and 28 tests); Go cache helpers (12 tests); focused Vitest (5 files, 24 tests); targeted ESLint; i18n ratchet; Actionlint v1.7.12; targeted Zizmor on both changed frontend workflows (no findings); `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`; `pnpm run build:vite`; and `git diff --check`. The full `zizmor .github/workflows` scan reported 273 repository-wide findings, with no findings in the changed frontend workflows.

The public Vitest runner fixture reconciled four files and eight outcomes, including skips, across both shards and preserved the original snapshot producer across attempts. A local full shard run passed shard 1 (1,554 files, 14,136 tests). Its first shard 2 attempt recorded 3 failed tests, 14,092 passed, and 4 skipped. The shard 2 retry passed 14,095 tests with 4 skipped and no failures; the implicated Linear watch suite also passed independently in production/CI mode. The failed attempt remains in the evidence note. This is local implementation evidence, not a matched hosted performance pair.

Task 20 retains hosted coverage and adoption measurements. No hosted parity or speedup is claimed.
