---
id: "01-baseline-fixture"
title: "Baseline-first release fixture"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-UPGRADE-REGRESSION-001
acceptance_criteria:
  - AC-PLATFORM-UPGRADE-REGRESSION-001.1
  - AC-PLATFORM-UPGRADE-REGRESSION-001.2
  - AC-PLATFORM-UPGRADE-REGRESSION-001.3
  - AC-PLATFORM-UPGRADE-REGRESSION-001.4
system_design:
  - ../../specs/platform/system-design/release-upgrade-regression.md
---

# Task 01: Baseline-first release fixture

## Summary

Acquire a verified published Stable runtime and create a baseline-first upgrade fixture. Establish an API-generated seed and prove an actual mock-agent turn on the released backend before the candidate can touch storage.

## In scope

- Release selection, checksums, manifest, safe extraction and explicit-tag replay.
- Dedicated upgrade fixture, version-scoped helpers and baseline API adapter.
- Seed version/IDs and representative pending, waiting, completed, archived and interrupted tasks, including multiple sessions, plans and a retained worktree file.
- Dedicated upgrade-desktop/upgrade-mobile projects, exclusion from ordinary projects, and a baseline preflight E2E test.

## Out of scope

Product behavior changes, unrelated refactors, real external providers, and all
other exclusions in the paired requirement. No new UI composition.

## Acceptance

- Unit tests reject bad digests, unsafe paths, unavailable engines and candidate-before-baseline spawn; teardown handles failure in every phase.
- The published baseline creates all seed data and completes a deterministic turn; no candidate schema constructor or raw SQL creates the seed.
- A manifest names the actual release/source/archive/helper identities and effective configuration; unsupported old behavior fails with a named capability error.

## Verification

Run all verification commands only within GitHub Actions on main. Do not run
these workloads locally. CI installs workspace dependencies before package commands.
The CI runner provisions its own disposable PostgreSQL service when requested.

```bash
node --test scripts/upgrade/ci-only.test.mjs scripts/upgrade/prepare-release.test.mjs
node scripts/upgrade/prepare-release.mjs --tag latest --out .cache/upgrade/baseline
(cd apps/web && pnpm exec vitest run --config e2e/upgrade.vitest.config.ts e2e/fixtures/upgrade-backend.test.ts)
(cd apps/web && KANDEV_UPGRADE_MANIFEST=../../.cache/upgrade/baseline/manifest.json KANDEV_UPGRADE_ENGINE=sqlite pnpm e2e:run --host --project upgrade-desktop -- tests/upgrade/release-upgrade.spec.ts --grep 'baseline seed provenance')
```

## Files likely touched

- `scripts/upgrade/prepare-release.mjs` and `prepare-release.test.mjs` (new)
- `apps/web/e2e/fixtures/upgrade-backend.ts` and `.test.ts` (new)
- `apps/web/e2e/helpers/upgrade-api.ts`, `upgrade-seed.ts` (new)
- `apps/web/e2e/tests/upgrade/release-upgrade.spec.ts` (new)
- `apps/web/e2e/playwright.config.ts`

## Dependencies

None.

## Risks

The old release may require a matching mock helper or request adaptation. Inspect its source at the peeled tag before adapting. Never silently replace its backend with a source build. Fixture process ownership must be testable before introducing candidate startup.

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
