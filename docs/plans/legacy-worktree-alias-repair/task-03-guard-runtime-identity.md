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

Configure `KANDEV_TEST_POSTGRES_DSN` for a disposable database before running
the complete block. Both packages must execute their PostgreSQL cases. A skipped
test is not concurrency evidence. Update the lint invocation for final helper
paths and include every changed Go file.

```bash
test -n "${KANDEV_TEST_POSTGRES_DSN:-}"
(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'Test(TaskEnvironmentAliasIdentity|PostgresAliasIdentity|WorkspaceInventoryRepair|TaskEnvironment)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/store.go apps/backend/internal/worktree/store_alias_identity_test.go apps/backend/internal/worktree/store_alias_identity_postgres_test.go apps/backend/internal/task/repository/sqlite/task_environment.go apps/backend/internal/task/repository/sqlite/task_environment_alias_identity_test.go apps/backend/internal/task/repository/sqlite/task_environment_alias_identity_postgres_test.go
git diff --check
```

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

### PR validation follow-up

CI exposed two integration gaps on head `2fe6e5b`:

- A local-to-worktree launch used an empty physical branch key while environment
  persistence used `main`. Creation now receives the same canonical branch key
  before it writes the inventory. The primary checkout keeps its flat path.
  `TestSingleRepoWorktreeTransitionRetainsCanonicalSlot` failed before the fix
  for checkout, base, and default branches. All three cases now pass against
  the real SQLite repository and worktree store.
- The PostgreSQL recovery CAS fixture reused physical IDs across subtests in
  one database. Its worktree IDs now include each case's suffix. The original
  test reproduced the three CI failures before this correction.

Checks completed on the corrected code:

```bash
(cd apps/backend && go test -race ./internal/orchestrator/executor -run 'Test(SingleRepoWorktreeTransition|ApplyRepositoryConfig|RepositoryCheckoutDefaultsConsumerProjection|PersistTaskEnvironment|BuildTaskEnvironmentRepos|BuildRepoSpecs)' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/worktree -run 'Test(TaskEnvironmentAliasIdentity|PostgresAliasIdentity|StoreAliasIdentity|PersistTaskEnvironmentTransition|PostgresCommitWorkspaceRecoveryError)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_execute.go apps/backend/internal/orchestrator/executor/executor_branch_identity_transition_test.go apps/backend/internal/task/repository/sqlite/workspace_recovery_error_cas_postgres_test.go
```

The PostgreSQL command used `KANDEV_TEST_POSTGRES_DSN` for the disposable
PostgreSQL 17 database. All listed tests passed with race detection. Scoped lint
reported zero issues. The ownership guard was not relaxed.

Browser checks used fresh managed builds and disabled retries. All four desktop
workflow-focus cases and the mobile workflow-focus case passed. The canvas
review case and the mobile runtime-summary reconnect case passed. The preview
case exposed a separate fixture race: the template workflow could finish a late
transition and select another primary session after the fixture's explicit
selection. Its dedicated manual workflow preserves all tab assertions. The
canvas-then-preview sequence failed before that fixture change and passed after
it (two tests, 39.2s). Scoped ESLint passed.

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/workflow/workflow-session-focus.spec.ts e2e/tests/kanban/preview-session-tabs.spec.ts e2e/tests/canvas/plugin-canvas.spec.ts -- --grep 'Workflow session focus|shows all sessions as tabs|keeps the desktop release review wide' --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/workflow/mobile-workflow-session-focus.spec.ts e2e/tests/settings/mobile-agent-runtime-notifications.spec.ts -- --grep 'shows the new committed recipient|backend runtime summary reconnect replay' --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project chromium e2e/tests/canvas/plugin-canvas.spec.ts e2e/tests/kanban/preview-session-tabs.spec.ts -- --grep 'keeps the desktop release review wide|shows all sessions as tabs' --retries=0
```

The first desktop command passed five cases and exposed the preview fixture race.
The final two commands passed after the relevant fixes. On this Nix host, the
commands set `KANDEV_SERVER_HOST=127.0.0.1` and supplied Chromium's installed
library directories through `LD_LIBRARY_PATH`. The initial bind-address and
missing-library failures occurred before the tests exercised application code.
Temporary diagnostic capture was removed. No product UI changed.

PR review follow-up (2026-10-18): retained deleted rows no longer block a unique
live owner during transition or inventory retirement. Revival and ambiguous
history remain blocked. SQLite and disposable PostgreSQL tests cover transition,
update, slot deletion, environment deletion, task-environment deletion, revival,
and history-only ambiguity. Six healthy cases failed before the fix. `go test
-race ./internal/task/repository/sqlite -run
'Test(TaskEnvironmentAliasIdentity|PostgresAliasIdentity)' -count=1` passed with
`KANDEV_TEST_POSTGRES_DSN` set to the disposable PostgreSQL fixture.

Current-base integration exposed transient HTTP 409 move responses in the
workflow-focus fixtures. The setup only excluded `CREATED`, so it could admit a
secondary session that was still `STARTING` or `RUNNING`. It now waits for
`WAITING_FOR_INPUT`, as the move precondition requires. A complete six-case
canvas/preview/workflow sequence passed on the combined branch before this
readiness refinement; focused post-change results are recorded below.

The four desktop workflow-focus tests passed after the readiness refinement
(1.3 minutes, retries disabled) on the synthetic merge with base
`70c884c84b8e16e3fb2f71736a791e93016ba00a`. Scoped ESLint passed. The command was
`pnpm --dir apps/web e2e:run --no-build --project chromium
e2e/tests/workflow/workflow-session-focus.spec.ts -- --retries=0`, with the same
host and Chromium library settings described above.
