---
id: "01-backend-copy-move-service"
title: "Backend copy/move service and store"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/secret-scope-transfer.md"
---

# Task 01: Backend Copy/Move Service and Store

## Acceptance

- `Service.Copy(ctx, sourceID, sourceWorkspaceID, req)` and
  `Service.Move(ctx, sourceID, sourceWorkspaceID, req)` exist with the spec's
  semantics: source resolution (Global via `Get`, workspace via
  `GetWorkspaceSecret`), target validation + destination authorization, a
  workspace-existence check that runs even in auth-disabled contexts, same
  scope+workspace rejection, and the 1-100 UTF-8 byte name rule.
- Error classification is exact and non-contradictory: store-level missing
  rows surface as `ErrNotFound` and missing/unauthorized destinations as
  `ErrWorkspaceAccessDenied` (both `404`); validation failures are wrapped with
  the `ErrSecretValidation` sentinel (`400`); every other error (raw database,
  storage, or callback lookup failure) passes through unclassified and is a
  sanitized `500`. No "any error" blanket wrapping exists anywhere.
- `name` uses presence semantics (`*string`): JSON-omitted means "use the
  source name"; present-but-empty or whitespace-only is a validation error.
- `ScopedSecretStore` gains atomic, callback-bearing `CopyScoped`/`MoveScoped`
  (`verifyDestination func(context.Context) error` invoked inside the
  transaction while the destination lock is held, before the conflict check
  and insert) implemented by `sqliteStore` in one transaction (conflict check
  + insert, plus source delete for Move, with rollback on any failure) and
  delegated by `UserVisibleStore` with an internal-ID (`github:` prefix)
  source guard. `ErrSecretNameConflict` is returned when the target name
  exists; a missing source returns `ErrNotFound`.
- The transfer transaction is atomic on both dialects with a single
  authoritative lock contract: SQLite starts with `BEGIN IMMEDIATE` (writer
  lock up front); PostgreSQL executes `BEGIN`, then
  `SELECT pg_advisory_xact_lock(<target scope key>)` on the same transaction
  BEFORE the source select and conflict check, and the PostgreSQL statement
  never runs on SQLite (dialect-gated). The lock key comes from the exported
  `WorkspaceLockKey(workspaceID)` (deterministic FNV-1a 64-bit of the
  `kandev-secret-transfer:` prefix + workspace ID) or the distinct
  `GlobalSecretTransferLockKey` constant for Global targets; a Move also
  locks the source workspace key, in sorted key order. Concurrent
  same-target-name writers serialize and exactly one wins under READ
  COMMITTED.
- Every statement in the operation (source select, conflict check, insert,
  Move delete, returned-row read) executes on the transaction-bound writer
  connection; the store's reader handle is never used inside the transaction.
  Lock-acquisition failure rolls back and surfaces as a store error (sanitized
  `500` upstream).
- Both methods return the inserted row's metadata (ID, name, scope, workspace,
  `created_at`, `updated_at`) via `RETURNING` where portable or a
  same-transaction select; tests assert non-zero timestamps matching the
  inserted target row AND exact target identity: returned `Scope` equals the
  requested target scope, `WorkspaceID` equals the requested target workspace
  (empty for Global targets), and the returned `Name` equals the target name.
- The copied row is created with `user_id = scopeOwner(ctx)`; the Move source
  delete reuses the same per-user visibility predicate as the source select.
- The conflict predicate compares the normalized scope (legacy empty stored
  scope is Global), so a legacy Global row cannot be bypassed.
- Move has no compensation window: copy and delete share a transaction; a
  crash or failure rolls back, leaving the source intact.
- The service exposes `ErrWorkspaceAccessDenied` and
  `SetWorkspaceExistenceChecker`; a nil existence checker fails closed for
  workspace destinations. Only known missing/unauthorized sentinels resolve as
  not-found; raw lookup/storage errors pass through unclassified.
- Tests cover: copy/move in all scope directions, name presence semantics
  (omitted → source name; explicit JSON `null`, present-but-empty, and
  whitespace-only → validation error; >100 UTF-8 bytes with multibyte input →
  validation error),
  same-scope rejection, name conflict (Global and workspace targets), legacy
  empty-scope Global target conflict, missing source, unauthorized source,
  unauthorized destination, nonexistent destination with the authorizer
  disabled (existence checker denies), unconfigured (nil) existence checker,
  internal `github:` source ID seeded as a real row → not-found (via
  `NewUserVisibleStore`), user A/user B ownership boundaries (copied row owned;
  A cannot move B's row), a deterministic concurrent same-target-name race on
  SQLite using a one-way schedule (A signals "lock acquired"; assert B is
  still blocked on the lock; A completes; B receives
  `ErrSecretNameConflict`; the schedule never requires both transfers to hold
  the lock at once), lock-acquisition failure rolling back to no-op, a Move
  failpoint injected AFTER the target insert and BEFORE the source delete
  asserting the transaction rolls back (source intact, no destination row —
  proving Move is not copy-then-commit-delete), a `verifyDestination`
  callback error rolling back with no insert, injected raw storage/authorizer
  errors that pass through unclassified (not `ErrNotFound`, not
  `ErrSecretValidation`), and list consistency with values verifiable only
  through reveal.
- Environment-gated PostgreSQL tests (`PostgresDSNFromEnv`) cover the same
  transfer semantics under READ COMMITTED: copy, Move rollback, scoped
  conflict, copied-row ownership, legacy empty-scope conflict, the
  concurrent race with the advisory lock (including a same-name race with a
  Global target on `GlobalSecretTransferLockKey`), and a deterministic
  delete-vs-transfer race proving no workspace-scoped secret survives its
  workspace's deletion. The PG race tests run on a multi-connection fixture
  (pool `maxConns >= 2` or two explicitly opened connections — a
  one-connection pool trivially serializes and proves nothing) with channel
  barriers pinning the interleavings: transfer locked before deletion
  cleanup, and deletion committed before the transfer's existence check.

## Files likely touched

- `apps/backend/internal/secrets/store.go` (or `models.go`):
  `ErrSecretNameConflict`, `ErrWorkspaceAccessDenied`, `ErrSecretValidation`,
  `CopyScoped`/`MoveScoped` on `ScopedSecretStore`
- `apps/backend/internal/secrets/models.go`: `CopySecretRequest`
- `apps/backend/internal/secrets/service.go`: `Copy`, `Move`,
  `SetWorkspaceExistenceChecker`, private helpers
- `apps/backend/internal/secrets/sqlite_store.go`: `CopyScoped`/`MoveScoped`
  transactions (BEGIN IMMEDIATE on SQLite, advisory xact lock on PostgreSQL)
- `apps/backend/internal/secrets/user_visible_store.go`: delegation +
  internal-ID guard
- `apps/backend/internal/agent/runtime/lifecycle/manager_auth_test.go`:
  `inMemorySecretStore` gains the two methods (compile-time interface member)
- `apps/backend/internal/backendapp/main.go`: wire the existence checker and
  the authorizer/existence adapters that map
  `repoerrors.ErrWorkspaceNotFound` → `secrets.ErrWorkspaceAccessDenied`
- New `copy_move_test.go`, `transfer_test.go`, and
  `postgres_transfer_test.go`

## Inputs

- Spec: API surface (Go service + store section), Permissions, Failure modes,
  Scenarios.
- Existing `Service` primitives: `Get`, `GetWorkspaceSecret`, `authorizeScope`,
  `validateSecretScope`; existing store scoping filters (`scopeOwner`).
- Task service workspace APIs for the existence checker
  (`apps/backend/internal/task/service/`).

## Dependencies

None.

## TDD sequence

  1. Write failing store tests (`transfer_test.go`) for `CopyScoped`/`MoveScoped`
   atomicity, ownership, legacy empty-scope conflicts, user scoping, and the
   concurrent same-target-name race using the one-way schedule (A signals
   "lock acquired"; assert B is still blocked on the lock; A completes; B
   receives `ErrSecretNameConflict`); add the environment-gated
   `postgres_transfer_test.go` equivalents. The SQLite race test observes lock
   acquisition explicitly: a test-only lock-acquisition hook on the store, or
   a two-writer helper opening separate connections to the same database file
   with explicit synchronization, so B's block is proven to be at the SQLite
   transaction lock and not merely at the single-connection Go pool (the
   existing `newTestSQLiteStore` helper pins `SetMaxOpenConns(1)` and cannot
   distinguish the two).
  2. Implement the store methods, dialect serialization, and `UserVisibleStore`
   delegation; add the two methods to `inMemorySecretStore`.
  3. Write failing service tests (`copy_move_test.go` with
   `NewUserVisibleStore(newTestSQLiteStore(t))` and the workspace-authorizer
   pattern from `service_test.go`), including nil-checker and internal-error
   pass-through cases.
  4. Implement `CopySecretRequest`, `ErrSecretNameConflict`,
   `ErrWorkspaceAccessDenied`, `Service.Copy`/`Service.Move`, the existence
   checker, and the main.go adapters.
  5. Re-run until green; keep existing secrets and lifecycle tests green.

## Verification

```bash
cd apps/backend && go test ./internal/secrets/... -run 'TestCopy|TestMove|TestTransfer' -count=1
cd apps/backend && go test ./internal/secrets/... ./internal/agent/runtime/lifecycle/...
```

(PostgreSQL-gated transfer tests run only when `PostgresDSNFromEnv` resolves,
matching the existing postgres test conventions.)

## Risks

- The service must not call the raw store directly for transfer; authorization
  and the internal-ID boundary only hold through the wrapper.
- Name default and byte-length rule must match create validation exactly.
- The transactional conflict check must use the same user-scoping filter as
  the rest of the store, and the serialization (BEGIN IMMEDIATE / advisory
  lock) is required for the atomicity guarantee on each dialect.
- The copied row must carry `user_id = scopeOwner(ctx)`; an unowned copy is a
  cross-user visibility leak in authenticated mode.
- Do not wrap raw lookup/storage errors as not-found; only the known sentinels
  (`ErrNotFound`, `ErrWorkspaceAccessDenied`) classify as 404.
- Keep values out of errors and logs (spec redaction rule).

## Output contract

Report the new symbols, transaction shape, existence-checker wiring, test
names, and both `go test` results.
