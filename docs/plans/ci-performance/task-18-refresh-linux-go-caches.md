---
id: "18-refresh-linux-go-caches"
title: "Refresh Linux Go caches"
status: in_progress
wave: 2
depends_on: ["17-repair-e2e-dependency-caches"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-008
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-005.6
  - AC-PLATFORM-CI-PERFORMANCE-008.1
  - AC-PLATFORM-CI-PERFORMANCE-008.2
  - AC-PLATFORM-CI-PERFORMANCE-008.3
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 18: Refresh Linux Go caches

## Summary

Migrate legacy Linux cache consumers to the existing shared Go action.
Resolve actual paths and refresh compatible compilation generations without repeating PR publication work.

## In scope

- Preserve baseline command blocks and cache timings before migration.
- Extend the helper's finite family set for four test shards, three repository cohorts, PostgreSQL cohorts/packages, and E2E helpers.
- Retain installed compiler/environment identities, native Go invalidation, and daily UTC generations.
- Add the backward-compatible `save-mode` contract from the design.
- Give main static checks the common module/lint publication role. Give other migrated main roles their own build publication.
- Keep migrated PR, merge-group, and dispatch consumers restore-only.
- Set the static linter's absolute cache path and include shared cache inputs in backend change detection.
- Cover all publication modes, default compatibility, optional failures, rollover, and role isolation with executable fixtures.

## Out of scope

- Windows, sidebar-resource, PostgreSQL 18, and existing host E2E build cache migrations.
- Compiler upgrades, test flags, commands, timeouts, runner placement, matrices, and required joins.
- Persistent remote caches, image redesign, publication, and workflow dispatch.

## Acceptance

1. Every scoped caller uses resolved module/build paths and compatible daily snapshots, with explicit publication ownership.
2. Cold, invalid, unavailable, or incompatible caches leave every original build/test command operational and preserve its failure exit.
3. Fixtures and hosted evidence cover main publication, PR restore-only behavior, exact hits, rollover, and unchanged current-source verification.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/go-cache-identity.test.mjs .github/scripts/cache-go-action.test.mjs
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/backend-tests.yml .github/workflows/e2e-tests.yml
zizmor .github/workflows
git diff --check
```

`cache-go-action.test.mjs` covers publication and failure behavior, not only YAML string matching.
Compare the original and migrated native command blocks byte-for-byte after removing only cache setup.
Record hosted paths, identity, role, generation, cache state, and restore/publication durations through Task 20.
Include cold population caused by the shared recipe change.
Pending hosted evidence leaves this work order `in_progress`.

## Files likely touched

- `.github/actions/cache-go/action.yml`.
- `.github/scripts/go-cache-identity.mjs` and `.github/scripts/go-cache-identity.test.mjs`.
- `.github/scripts/cache-go-action.test.mjs` (new).
- `.github/workflows/backend-tests.yml`: static, ordinary tests, repository, ambient, and PostgreSQL 16 cache setup.
- `.github/workflows/e2e-tests.yml`: `helper_compile` cache setup.
- `.github/scripts/backend-tests-workflow-contract_test.py` and `.github/scripts/e2e-tests-workflow-contract_test.py`.
- `.github/workflows/lint-action-pinning.yml`: register the new action fixtures in its existing job.
- `docs/plans/ci-performance/evidence-2026-10-26.md`.

## Dependencies

Task 17 owns the preceding edits to the same E2E workflow.
Do not overlap the two implementations.

## Risks

A shared action recipe change invalidates existing build generations, including unchanged callers.
New PR dependencies remain cold until a compatible main publication exists.
Main module publication is an optimization dependency, never a prerequisite for native verification.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [Design](../../specs/platform/system-design/ci-performance.md#october-26-cache-and-partition-continuation).
- Existing cache action, identity tests, and host E2E build caller.
- [Capacity decision](../../decisions/2026-10-08-capacity-aware-ci-verification.md).

## Results

Implementation is in place for the five backend jobs and the E2E helper compiler. The action accepts `all`, `build`, and `none`; omitted inputs retain `all`. Main static checks own module, build, and lint publication. The other migrated main roles publish only their own build generation. Other event types restore only. Identity failures and cache service errors remain optional.

Local checks passed: `node --test .github/scripts/go-cache-identity.test.mjs .github/scripts/cache-go-action.test.mjs` (12 tests), `python3 .github/scripts/backend-tests-workflow-contract_test.py` (26 tests), `python3 .github/scripts/e2e-tests-workflow-contract_test.py` (28 tests), `python3 .github/scripts/lint-action-pinning_test.py` (9 tests), actionlint v1.7.12 on the changed workflows, and `git diff --check`. The migration preserved 24 native `run:` blocks byte-for-byte across six jobs. Cache action fixtures cover publication modes, cache hits, failure tolerance, and command exit preservation.

Zizmor reported no findings for `e2e-tests.yml` or `lint-action-pinning.yml`. Its only finding in the changed backend workflow is the existing Windows `shell: cmd` misfeature; the Go cache changes produced no findings. The full workflow scan still reports 271 findings across the repository, outside this cache change.

Hosted cold population, warm restore, publication cost, and source-only cache refresh evidence remains pending. The recipe change invalidates prior build generations. Record these measurements through Task 20; keep this work order `in_progress` until the required hosted evidence is available.
