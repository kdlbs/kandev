---
id: "03-guard-runtime-identity"
title: "Reject ambiguous runtime identity"
status: done
wave: 3
depends_on:
  - "02-reconcile-shared-inventory"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.8
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 03: Reject ambiguous runtime identity

## Summary

Make ordinary worktree authority reads and writes reject competing active
inventory slots. Prevent timestamp selection, unscoped `LIMIT 1`, and ID-based
updates from selecting or changing the wrong owner.

## In scope

- Add a typed ambiguity error at the worktree-store boundary and propagate it
  through existing admission/cleanup error paths without guessing a repair.
- Check slot identities independently from session joins. Preserve one active
  owner with deleted history, multiple sessions of one environment, inventory-only
  preparation rows, and distinct branch slots of the same repository.
- Audit ID-based writes in `worktree/store.go` and physical-ID persistence in
  `task_environment.go`. Address a single proven slot or refuse. Serialize new
  claims by physical ID, including concurrent PostgreSQL writers.
- Keep migrations and old databases bootable. Add no global unique index or
  automatic alias election. Do not alter session conversation or shared-group policy.

## Out of scope

Automatic repair, new UI controls, and changing immutable cleanup snapshots.

## Acceptance

1. `TestStoreAliasIdentity` and `TestTaskEnvironmentAliasIdentity` refuse ambiguous
   reads/writes and preserve every row, while healthy multi-session/history and
   multi-branch controls remain functional.
2. `TestPostgresAliasIdentity` proves concurrent conflicting physical claims cannot
   both commit. Run against a real PostgreSQL instance with `-race`.
3. Existing cleanup and recovery paths never translate ambiguity into absence,
   rematerialization, generic success, or an update of multiple claimants.

## Verification

```bash
test -n "${KANDEV_TEST_POSTGRES_DSN:-}"
(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'Test(TaskEnvironmentAliasIdentity|PostgresAliasIdentity|WorkspaceInventoryRepair|TaskEnvironment)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/store.go apps/backend/internal/worktree/store_alias_identity_test.go apps/backend/internal/worktree/store_alias_identity_postgres_test.go apps/backend/internal/task/repository/sqlite/task_environment.go apps/backend/internal/task/repository/sqlite/task_environment_alias_identity_test.go apps/backend/internal/task/repository/sqlite/task_environment_alias_identity_postgres_test.go
git diff --check
```

Configure `KANDEV_TEST_POSTGRES_DSN` for a disposable database before running
the complete block. Both packages must execute their PostgreSQL cases. A skipped
test is not concurrency evidence. Update the lint invocation for final helper
paths and include every changed Go file.

## Files likely touched

- `apps/backend/internal/worktree/{store,manager}.go` and focused store helpers.
- New `store_alias_identity_test.go` and PostgreSQL tests in that package.
- `apps/backend/internal/task/repository/sqlite/task_environment.go` and a private
  physical-identity transaction helper shared with the store where needed.
- New `task_environment_alias_identity_test.go` and
  `task_environment_alias_identity_postgres_test.go` in that repository package.
- `apps/backend/internal/worktree/recovery_admission.go` only if typed propagation needs it.

## Dependencies

Tasks 01 and 02 provide the explicit recovery path before stronger refusals ship.

## Risks

Session joins can resemble duplicate owners. Tombstoned inventory can coexist
with a valid active slot. PostgreSQL serialization must precede the first claim
read and use the established transaction-scoped advisory-lock pattern.

## Parallelism

`sequential`

## Inputs

- Linked design section on runtime ambiguity prevention.
- Existing store, shared-reference, physical-refresh, and recovery-admission tests.

## Results

Implementation and required checks complete.

- RED: competing owners passed every store read and ten ID-based writes.
  Several writes changed both rows. A replacement upsert also discarded an
  ambiguous old identity.
- RED: real cleanup removed a checkout before its metadata update returned an
  ownership error. Cleanup now checks inventory before filesystem changes.
- Focused race tests passed: worktree 50.792s, repository 13.995s.
- Both PostgreSQL conflict tests passed against a disposable PostgreSQL 17
  database. Removing the physical-ID lock made both tests fail because the
  competing writer completed before the required lock wait. Restored the lock.
- RED: a PostgreSQL transition resumed after a physical-ID replacement and
  restored the stale ID. The batch guard now compares its observed slot IDs
  after acquiring all physical locks and refuses changed identities.
- SQL guard passed. Lint required moving cleanup identity helpers into
  `manager_cleanup_identity.go` to stay within the file-size limit.

The shared helper is `internal/task/physicalclaim`. It counts inventory slots
without session joins and locks physical IDs in sorted order on PostgreSQL.
No migration or global unique index was added.

The first full worktree run reached Go's 10-minute timeout. It also exposed a
cancellation-error regression in cleanup identity capture. The new check now
preserves the established `CleanupInspectionError` classification. Three
permission tests inherited the runner's restrictive `0077` umask, and one
existing test used `/bin/rm`, which this Nix host does not provide. The test shim
now resolves `rm` through PATH. The final full run uses `umask 022` and
`-timeout 30m -v`; it passed (571.641s).

Final repository race checks passed (90.714s), including the stale-transition
case. Store conformance passed with PostgreSQL enabled (320.454s). SQL guard and
scoped lint passed after the final repository change. The PostgreSQL database
uses a private temporary Unix socket and isolated per-test schemas.

The final recovery error-chain change preserves the typed ambiguity cause without
changing the displayed error. All new and changed worktree regressions passed
again after that change (147.560s), including cleanup cancellation/deadline
classification and the portable removal shim. Final scoped lint passed.

Final full-suite invocation:
`umask 022; KANDEV_TEST_POSTGRES_DSN=<disposable database> go test -trimpath -race -timeout 30m -v ./internal/worktree -count=1`.
The remaining verification commands above passed, including PostgreSQL-enabled
store conformance. Final lint selected `store.go`, `task_environment.go`,
`physicalclaim/claim.go`, and `models/worktree_identity.go`; the helper checked
all Go files in those packages. `git diff --check` passed.
