---
id: "03-task-lifecycle"
title: "Runtime and browser lifecycle"
status: done
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

CI acceptance passed on 2026-10-20 in [run 39510140130](https://github.com/kdlbs/kandev/actions/runs/39510140130).
Published Stable `v0.97.0` was compared with candidate `43e1a82ceb639d083d31d5af3b586ee05173dc34`:
SQLite desktop 3/3, PostgreSQL desktop 3/3, and SQLite phone 1/1 scenarios passed,
with no skipped or retried scenario. The harness contracts (10 Node tests and
6 fixture/oracle tests), changed-file lint, action pinning tests (9), shell syntax,
and specification/catalog checks passed. All three evidence uploads succeeded.

The user authorized this pre-merge branch validation. A temporary exact-branch
exception enabled it; the final policy-only follow-up restores main-only workflow,
runner and fixture guards and rejects this branch in the guard contract. That
follow-up is not claimed as an additional upgrade rehearsal. No upgrade workloads
ran locally after the user's CI-only instruction. Product runtime code is unchanged.

## PR review remediation

Review remediation verifies the review profile and requested workflow step after switching, archived state before cleanup, and completed/unarchived final state after actions and restart.

Remediation verification passed in [CI run 39686419086](https://github.com/kdlbs/kandev/actions/runs/39686419086)
at candidate `59bfe1d15b23eed7e25763b8cb0b3a60120fb9ce`: all seven upgrade
scenarios, 15 Node contracts and 10 fixture/config/oracle tests passed. The
worker-selection regression found in the first remediation run is covered by
an inherited-environment config test. Main-only guards are restored in the
policy/documentation follow-up; this is not an additional upgrade rehearsal.
PR checks and review/merge gates remain external and pending at commit time.
