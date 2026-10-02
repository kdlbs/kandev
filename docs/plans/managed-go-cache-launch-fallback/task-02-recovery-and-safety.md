---
id: "02-recovery-and-safety"
title: "Recovery flows and maintenance safety"
status: done
wave: 2
depends_on:
  - "01-lifecycle-fallback"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.8
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006.9
system_design:
  - ../../specs/system-page/system-design/storage-maintenance-01.md
---

# Task 02: Recovery flows and maintenance safety

## Summary

Prove initial launch, Resume, and Start fresh through the real backend dispatch and lifecycle integration.
Retain conservative maintenance behavior for unsafe paths and document the new operator-visible fallback.
This work order adds boundary evidence and documentation after Task 01 establishes the fallback.

## In scope

- Add `TestManagedGoCacheRecoveryFlows` under backendapp with real SQLite state, storage settings, provider, executor, and lifecycle adapter.
- Use actual `Service.RecoverSession` dispatch for `fresh_start` and `resume`, not simulated recovery request shapes alone.
- Fake external agent/agentctl transport with successful startup responses and observable process environment.
- Add real provider safety tests for adopted-root symlinks, symlinked ancestors, and adoption rejection.
- Retain existing cleanup, trash, marker, restore, and deletion safety suites.
- Updated `docs/public/operations.md` to explain fallback, retained settings, strict maintenance safety, and possible independent-cache tool errors.
- Record exact final test and document results in this package.

## Out of scope

Live task changes, host repair, browser layout changes, new diagnostics UI, expanded cleanup authority, and unrelated test suites.

## Acceptance

1. Initial launch, Resume, and Start fresh create usable executions despite an unsafe adopted cache. Recovery advances beyond cache preparation without a cache-specific `FAILED` transition or failure projection.
2. Unsafe maintenance/adoption requests preserve target sentinel bytes and paths and create no quarantine intent. Successful safe-path controls prove the maintenance operation is reachable.
3. Public operations guidance describes implemented behavior. Plan/work-order results contain actual validation evidence and synchronized completion status.

## Test implementation plan (completed)

This plan guided the implementation. Actual regression coverage and command outcomes are recorded in Results.

Use `adapters_kubernetes_launch_test.go` as the registry/manager/adapter wiring example.
Replace its intentional capture-error backend pattern with a successful isolated process fixture.
Use actual persisted storage settings to select the adopted temporary path.
Seed a recoverable failed session and normal task/executor profiles, then run real fresh-start and resume dispatch.
For fresh-start, assert the existing provider-token clearing contract. For resume, assert token preservation.
Assert successful runtime and agent-start effects, effective environment, bounded skip warning, and preserved target contents.
Add a non-cache launch failure control and a cancelled recovery attempt to prove that other failures remain authoritative.

Task 01 supplies the primary red regression.
For the full flow, establish its pre-fix failure against the parent source using an owned temporary checkout or temporary reversible patch.
Do not alter live cache paths or settings to obtain red evidence. Remove owned temporary artifacts afterwards.
Label unchanged maintenance checks as safety contracts rather than claiming they fail before this repair.

In new `provider_launch_safety_test.go`, add:

- `TestExecutionEnvironmentRejectsUnsafeSymlinks`: adopted root and dedicated cache ancestor still produce provider errors.
- `TestCleanupRejectsAdoptedRootSymlink`: normal and explicit cleanup reject an above-threshold adopted symlink before quarantine intent or mutation.
- `TestValidateAdoptionRejectsSymlinks`: root and ancestor symlinks fail with valid `ADOPT` confirmation and preserve target bytes.

Run existing `TestCleanupRejectsSymlinkedManagedCacheAncestor`, `TestCleanupRejectsSymlinkedTrashAncestor`, and `TestCleanupRejectsSymlinkedOwnershipMarker`.
Retain the full `gocache` suite for provider safety.
Restore and deletion live in `backendapp/storage_quarantine_controller.go`, with regressions in `storage_maintenance_test.go`.
Add `TestManagedGoCacheQuarantineRejectsSymlinks` in the new backendapp file.
Cover symlinked original paths during restore and symlinked quarantine payloads during restore and deletion.
Retain existing failed-retry restore and safe restore/delete controls.
Assert target sentinel contents, link identity, and durable entry state remain unchanged after rejection.
No production provider or shared safety changes are expected.

## Verification (completed)

The following validation checklist was completed:

Run from repository root:

```bash
(cd apps/backend && go test ./internal/backendapp -run '^Test(ManagedGoCacheRecoveryFlows|ManagedGoCacheQuarantineRejectsSymlinks|QuarantineController.*GoCache)' -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run '^TestManagedGoCacheRecoveryFlows$' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The exact coverage preflight in [plan.md](plan.md#exact-verification) and local `validateCoverage` over the actual changed package both passed with `ok: true` and `status: covered`.
Plan/work-order statuses and requirement/design mappings were checked before completion. Requirement 006 is Active; unrelated legacy design sections were not promoted.

## Files touched

- New `apps/backend/internal/backendapp/adapters_managed_go_cache_test.go`
- New `apps/backend/internal/system/storage/gocache/provider_launch_safety_test.go`
- `docs/public/operations.md`
- `docs/plans/managed-go-cache-launch-fallback/plan.md` and sibling work-order results
- `docs/specs/system-page/requirements/storage-maintenance.md` (requirement 006 and active status)
- `docs/specs/system-page/system-design/storage-maintenance-01.md` (extension lifecycle statement only)
- `docs/specs/system-page/system-design/storage-maintenance-02.md` (link to the launch fallback rule)
- `docs/decisions/2026-10-01-optional-managed-go-cache.md` (implementation status only)
- `docs/plans/storage-maintenance/plan.md` (completed-package cross-link)

Read-only integration: `backendapp/adapters.go`, `orchestrator/session_launch.go`, `orchestrator/executor/executor_resume.go`, and `backendapp/storage_quarantine_controller.go`.
If flow coverage exposes another production defect, update the package before expanding implementation scope.

## Dependencies

Task 01.

## Risks

Full recovery coverage requires complete isolated profile, repository, and agentctl fixtures.
A mocked successful recovery result cannot prove the product repair.
Existing storage diagnostics can still report cache unavailability after launch succeeds.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), active requirement 006.
- [Design](../../specs/system-page/system-design/storage-maintenance-01.md#managed-cache-launch-integration).
- [Task 01](task-01-lifecycle-fallback.md) implementation and regression evidence.
- Existing backendapp registry/manager/adapter tests and orchestrator recovery dispatch.
- Existing provider `recordingStore`, settings, and sentinel-preservation fixtures.
- `/docs-maintainer` and `simple-english` for the public operations explanation.

## Results

Implemented the recovery and maintenance-safety work. `TestManagedGoCacheRecoveryFlows` exercises an initial launch through the orchestrator executor and lifecycle adapter, then real SQLite-backed `RecoverSession` calls for Resume and Start fresh. The fake agentctl transport captures effective `GOCACHE` and ACP session tokens. The test also covers a non-cache configure failure and cancelled recovery.

Provider tests prove execution preparation, scheduled and explicit cleanup, and adoption validation reject adopted-root and ancestor symlinks without changing target bytes or recording quarantine intent. Backend quarantine tests reject linked original paths and linked payloads during restore and deletion while preserving link identity, target bytes, and durable entry state. Existing safe restore/delete controls remain in place.

Validation passed:

- `(cd apps/backend && go test ./internal/backendapp -run '^Test(ManagedGoCacheRecoveryFlows|ManagedGoCacheQuarantineRejectsSymlinks|QuarantineController.*GoCache)' -count=1)`
- `(cd apps/backend && go test -race ./internal/backendapp -run '^TestManagedGoCacheRecoveryFlows$' -count=1)`
- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -count=1)`
- `make -C apps/backend build`
- `python3 scripts/list-docs.py validate` (340 decisions and 1283 specifications)
- `python3 scripts/lint-spec-files.test.py` (36 tests passed)
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs` (62 tests passed)
- `node scripts/validate-public-docs.mjs` (47 published pages validated)
- `git diff --check`

Requirement 006 is Active, its lifecycle design and decision record reflect implementation, and public operations guidance explains fallback and strict maintenance safety.
