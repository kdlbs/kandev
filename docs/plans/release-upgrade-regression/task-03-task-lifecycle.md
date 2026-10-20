---
id: "03-task-lifecycle"
title: "Runtime and browser lifecycle"
status: in_progress
wave: 3
depends_on:
  - 02-preservation-replay
plan: "plan.md"
requirements:
  - REQ-PLATFORM-UPGRADE-REGRESSION-003
acceptance_criteria:
  - AC-PLATFORM-UPGRADE-REGRESSION-003.1
  - AC-PLATFORM-UPGRADE-REGRESSION-003.2
  - AC-PLATFORM-UPGRADE-REGRESSION-003.3
  - AC-PLATFORM-UPGRADE-REGRESSION-003.4
  - AC-PLATFORM-UPGRADE-REGRESSION-003.5
system_design:
  - ../../specs/platform/system-design/release-upgrade-regression.md
---

# Task 03: Runtime and browser lifecycle

## Summary

Prove the upgraded installation can execute both old and new work, then verify durable results after restart. Add desktop and phone history/follow-up smoke through the existing interfaces.

## In scope

- Pending start, waiting follow-up, selected completed-session resume and supported interrupted recovery.
- New task create/start, controlled long turn, stop/resume, workflow transition, completion and archive/unarchive.
- Passive reads without launch, selected/primary identity and duplicate historical turn assertions.
- Desktop and mobile browser history, plan and actual follow-up response on SQLite; complete API/runtime matrix on both engines.

## Out of scope

Product behavior changes, unrelated refactors, real external providers, and all
other exclusions in the paired requirement. No new UI composition.

## Acceptance

- Every seeded lifecycle and new task action proves actual deterministic agent response plus correct task/session/turn identities; no state-only success assertions.
- All post-action durable results survive the second candidate boot on both engines.
- Existing desktop/phone interfaces display old history and produce a new response in the selected conversation; causal waits and error evidence identify failures.

## Verification

Run all verification commands only within GitHub Actions on main. Do not run
these workloads locally. CI installs workspace dependencies before package commands.
Reuse the verified baseline from Task 01. The runner provisions a disposable
PostgreSQL service and drops each fixture-owned database. `--no-build` reuses the
freshly built candidate from the preceding task. Never substitute a developer database.

```bash
KANDEV_UPGRADE_MANIFEST=.cache/upgrade/baseline/manifest.json bash scripts/upgrade/run.sh --engine sqlite --project upgrade-desktop --no-build
KANDEV_UPGRADE_MANIFEST=.cache/upgrade/baseline/manifest.json bash scripts/upgrade/run.sh --engine postgres --project upgrade-desktop --no-build
KANDEV_UPGRADE_MANIFEST=.cache/upgrade/baseline/manifest.json bash scripts/upgrade/run.sh --engine sqlite --project upgrade-mobile --no-build
```

## Files likely touched

- `apps/web/e2e/tests/upgrade/release-upgrade.spec.ts`
- `apps/web/e2e/tests/upgrade/mobile-release-upgrade.spec.ts` (new)
- `apps/web/e2e/helpers/upgrade-lifecycle.ts` (new)
- `apps/web/e2e/helpers/upgrade-oracle.ts` (extend post-action assertions)

## Dependencies

02-preservation-replay.

## Risks

Recovery controls differ from passive history open. Reuse current completed-session resume and mobile patterns, and inspect source contracts before assuming a recovery action. These tests do not establish real provider or remote executor continuation.

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
