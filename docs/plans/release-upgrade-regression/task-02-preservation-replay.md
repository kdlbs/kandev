---
id: "02-preservation-replay"
title: "Preservation and replay oracles"
status: done
wave: 2
depends_on:
  - 01-baseline-fixture
plan: "plan.md"
requirements:
  - REQ-PLATFORM-UPGRADE-REGRESSION-001
  - REQ-PLATFORM-UPGRADE-REGRESSION-002
acceptance_criteria:
  - AC-PLATFORM-UPGRADE-REGRESSION-001.4
  - AC-PLATFORM-UPGRADE-REGRESSION-002.1
  - AC-PLATFORM-UPGRADE-REGRESSION-002.2
  - AC-PLATFORM-UPGRADE-REGRESSION-002.3
  - AC-PLATFORM-UPGRADE-REGRESSION-002.4
system_design:
  - ../../specs/platform/system-design/release-upgrade-regression.md
---

# Task 02: Preservation and replay oracles

## Summary

Upgrade the real seed to the candidate and prove preservation and second-boot replay on both engines. Add independent semantic and database integrity oracles, including SQLite pre-migration backup verification.

## In scope

- Candidate boot/readiness and runtime identity verification on unchanged baseline storage.
- Immutable baseline snapshot comparisons, expected migration transformations and duplicate detection.
- SQLite integrity/FK/backup checks and PostgreSQL domain relationship/constraint checks on owned storage.
- Second candidate startup and repeated preservation assertions, with phase-specific diffs.

## Out of scope

Product behavior changes, unrelated refactors, real external providers, and all
other exclusions in the paired requirement. No new UI composition.

## Acceptance

- The same live upgrade/preservation scenario passes on SQLite and PostgreSQL 16; requested PostgreSQL without a service fails rather than skips.
- Oracle unit tests deliberately remove, mutate and duplicate sentinel content and show named failures; allowed field transformations are narrow and explicit.
- Second boot retains seed identity/content and valid relationships; SQLite backup is readable with baseline sentinels.

## Verification

Run all verification commands only within GitHub Actions on main. Do not run
these workloads locally. CI installs workspace dependencies before package commands.
Reuse the verified baseline from Task 01. The runner provisions a disposable
PostgreSQL service and drops each fixture-owned database. `--no-build` reuses the
freshly built candidate from the preceding task. Never substitute a developer database.

```bash
(cd apps/web && pnpm exec vitest run --config e2e/upgrade.vitest.config.ts e2e/helpers/upgrade-oracle.test.ts)
KANDEV_UPGRADE_MANIFEST=.cache/upgrade/baseline/manifest.json bash scripts/upgrade/run.sh --engine sqlite --project upgrade-desktop --no-build
KANDEV_UPGRADE_MANIFEST=.cache/upgrade/baseline/manifest.json bash scripts/upgrade/run.sh --engine postgres --project upgrade-desktop --no-build
```

## Files likely touched

- `apps/web/e2e/helpers/upgrade-oracle.ts` and `.test.ts` (new)
- `apps/web/e2e/helpers/upgrade-database.ts` (new)
- `apps/web/e2e/fixtures/upgrade-backend.ts`
- `apps/web/e2e/tests/upgrade/release-upgrade.spec.ts`

## Dependencies

01-baseline-fixture.

## Risks

Background reconciliation may legitimately change interrupted state. Assert that transformation explicitly while keeping message/task identity strict. Run physical checks after terminating owned processes so concurrent maintenance cannot make snapshots inconsistent.

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

Review remediation adds an independent post-action history oracle: original message IDs, contents and ordering must survive while new turns may be appended. Mutation coverage rejects deletion, content changes, replacement IDs, reordering and duplicates.

Remediation verification passed in [CI run 39686419086](https://github.com/kdlbs/kandev/actions/runs/39686419086)
at candidate `59bfe1d15b23eed7e25763b8cb0b3a60120fb9ce`: all seven upgrade
scenarios, 15 Node contracts and 10 fixture/config/oracle tests passed. The
worker-selection regression found in the first remediation run is covered by
an inherited-environment config test. Main-only guards are restored in the
policy/documentation follow-up; this is not an additional upgrade rehearsal.
PR checks and review/merge gates remain external and pending at commit time.
