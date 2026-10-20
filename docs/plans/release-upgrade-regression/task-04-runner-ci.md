---
id: "04-runner-ci"
title: "CI-only runner and workflow"
status: in_progress
wave: 4
depends_on:
  - 03-task-lifecycle
plan: "plan.md"
requirements:
  - REQ-PLATFORM-UPGRADE-REGRESSION-001
  - REQ-PLATFORM-UPGRADE-REGRESSION-002
  - REQ-PLATFORM-UPGRADE-REGRESSION-003
acceptance_criteria:
  - AC-PLATFORM-UPGRADE-REGRESSION-001.1
  - AC-PLATFORM-UPGRADE-REGRESSION-001.3
  - AC-PLATFORM-UPGRADE-REGRESSION-001.4
  - AC-PLATFORM-UPGRADE-REGRESSION-001.5
  - AC-PLATFORM-UPGRADE-REGRESSION-001.6
  - AC-PLATFORM-UPGRADE-REGRESSION-002.3
  - AC-PLATFORM-UPGRADE-REGRESSION-003.4
  - AC-PLATFORM-UPGRADE-REGRESSION-003.5
system_design:
  - ../../specs/platform/system-design/release-upgrade-regression.md
---

# Task 04: CI-only runner and workflow

## Summary

Expose the complete rehearsal through an unprivileged main-only CI workflow; refuse local workloads at every entry point. Preserve one baseline identity across CI engine/browser jobs and publish phase-specific evidence without treating skipped work as acceptance.

## In scope

- CI-only runner provisions owned PostgreSQL when requested and uses the managed memory-bounded E2E runner.
- Main pushes, nightly schedule and manual dispatch on main only; no pull-request triggers. Guard non-main dispatch and pin every matrix job to the same main commit.
- One resolved baseline manifest across the matrix; exact workflow/candidate identities and required scenario count checks.
- Failure logs, manifest, snapshots, semantic diff, JUnit/trace, cleanup and CI dispatch documentation.

## Out of scope

Product behavior changes, unrelated refactors, real external providers, and all
other exclusions in the paired requirement. No new UI composition.

## Acceptance

- Runner tests show missing scenarios, skipped engine, failed child process and digest failure produce nonzero status and still clean up owned resources.
- SQLite desktop, PostgreSQL desktop/API, and SQLite phone commands all execute required scenarios; failed phases retain useful diagnostics.
- CI uses pinned actions/read-only permissions, shares one baseline resolution, runs only on main with workflow-contract tests for triggers/guards/commit pinning, excludes suite from default E2E jobs, and documents exact-tag CI replay and coverage limitations.

## Verification

Run all verification commands only within GitHub Actions on main. Do not run
these workloads locally. CI installs workspace dependencies before package commands.
The CI runner provisions its own disposable PostgreSQL service when requested.

```bash
node --test scripts/upgrade/ci-only.test.mjs scripts/upgrade/prepare-release.test.mjs scripts/upgrade/run.test.mjs scripts/upgrade/workflow-contract.test.mjs
bash -n scripts/upgrade/run.sh
bash scripts/upgrade/run.sh --baseline latest --engine sqlite --project upgrade-desktop
bash scripts/upgrade/run.sh --baseline latest --engine postgres --project upgrade-desktop
bash scripts/upgrade/run.sh --baseline latest --engine sqlite --project upgrade-mobile
python3 .github/scripts/lint-action-pinning_test.py
uvx zizmor .github/workflows/upgrade-regression.yml
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `scripts/upgrade/ci-only.mjs`, `ci-only.test.mjs`, `run.sh`, `run.mjs`, `run.test.mjs` and `workflow-contract.test.mjs` (new)
- `.github/workflows/upgrade-regression.yml` (new)
- `apps/web/e2e/README.md`
- `docs/plans/release-upgrade-regression/plan.md` and work-order results

## Dependencies

03-task-lifecycle.

## Risks

Live release resolution needs network access; report network failure distinctly while failing the gate. Main-only triggers must not execute candidate code with publication credentials. Regressions are detected after merge. Provisioned PostgreSQL uses Docker on the hosted runner; do not silently fall back to SQLite.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/release-upgrade-regression.md),
  matching frontmatter REQ/AC IDs.
- [System design](../../specs/platform/system-design/release-upgrade-regression.md).
- [Plan](plan.md), including scenario matrix and execution constraints.
- Existing `fixtures/backend.ts`, `helpers/api-client.ts`,
  `tests/session/completed-session-resume.spec.ts`, and
  `tests/session/mobile-completed-session-resume.spec.ts`.
- `.github/AGENTS.md` before CI edits; `/e2e` and `/tdd` for implementation.

## Results

Implementation is present. Final verification is pending a main-only GitHub
Actions run. The user prohibited further local execution on 2026-10-20.
Earlier local evidence predates the CI-only guards and is not final acceptance.
