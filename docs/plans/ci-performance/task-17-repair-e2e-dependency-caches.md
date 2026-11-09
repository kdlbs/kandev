---
id: "17-repair-e2e-dependency-caches"
title: "Repair E2E dependency caches"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-002
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-002.1
  - AC-PLATFORM-CI-PERFORMANCE-002.2
  - AC-PLATFORM-CI-PERFORMANCE-002.3
  - AC-PLATFORM-CI-PERFORMANCE-005.6
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 17: Repair E2E dependency caches

## Summary

Repair the confirmed pnpm cache-path failure in the four E2E execution environments.
Retain frozen installation, native commands, and the current job graph.

## In scope

- Capture baseline cache/installation steps before the workflow change.
- Resolve the absolute store path and pnpm version from `apps/` before each affected cache restore.
- Apply the existing frontend OS/architecture/pnpm-version/lockfile key contract.
- Keep cache failures optional and dependency installation authoritative.
- Add workflow regressions for actual path wiring, compatibility, order, and unconditional frozen installation in every affected family.
- Record cold save, compatible restore, and unavailable-cache installation evidence from natural or separately authorized runs.

## Out of scope

- Host E2E build/report cache changes, browser caches, image changes, dependency upgrades, and package-directory transfers.
- Runner, permission, matrix, test-selection, worker, retry, timeout, or gate changes.
- Publication or workflow dispatch without separate authorization.

## Acceptance

1. The four execution families resolve and cache their actual install store, with compatible keys and optional cache operations.
2. A missing cache permits installation and publication. A later compatible run restores it without missing-path warnings.
3. Cache errors leave frozen installation operational. A lockfile error still fails installation and the required gate.

## Verification

Run from the repository root:

```bash
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/e2e-tests.yml
zizmor .github/workflows
git diff --check
```

Record existing Zizmor findings separately from new findings.
For hosted evidence, use the Task 20 collection command with an actual run and attempt.
Preserve source, container, store, version, key, cache result, install exit, and full setup duration.
A controlled miss must use task-owned cache inputs. Do not delete shared caches.
Pending hosted evidence leaves this work order `in_progress`.

## Files likely touched

- `.github/workflows/e2e-tests.yml`: `e2e`, `e2e-containers`, `e2e-kubernetes-compatibility`, and `desktop-e2e` dependency steps.
- `.github/scripts/e2e-tests-workflow-contract_test.py`.
- `docs/plans/ci-performance/evidence-2026-10-26.md`: append actual results and provenance.

## Dependencies

None. Task 02 supplies the implemented frontend pattern, not an additional delivery dependency.

## Risks

Cache transfer can erase download savings. Task 20 decides performance adoption using total setup costs.
The browser-cache package retains its own pending rollout evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [Design](../../specs/platform/system-design/ci-performance.md#october-26-cache-and-partition-continuation).
- [Diagnostic evidence](evidence-2026-10-26.md).
- `.github/workflows/frontend-tests.yml`, resolved-store steps, and their workflow contract tests.

## Results

Implementation is in place for `e2e`, `e2e-containers`, `e2e-kubernetes-compatibility`, and `desktop-e2e`. Each job resolves its absolute pnpm store and version inside `apps/`, uses the frontend platform/architecture/version/lockfile key, and keeps cache setup optional before the unconditional frozen install.

Local checks passed: `python3 .github/scripts/e2e-tests-workflow-contract_test.py` (28 tests), `python3 .github/scripts/lint-action-pinning_test.py` (9 tests), `actionlint` v1.7.12 on the changed workflows, targeted `zizmor .github/workflows/e2e-tests.yml` (no findings), and `git diff --check`. The full-workflow zizmor scan reported findings only in other workflows.

Hosted candidate save, compatible restore, and cache-unavailable installation evidence remains pending. Existing diagnostic baseline is recorded in [evidence-2026-10-26.md](evidence-2026-10-26.md). Keep this work order `in_progress` until Task 20 records the required hosted evidence.
