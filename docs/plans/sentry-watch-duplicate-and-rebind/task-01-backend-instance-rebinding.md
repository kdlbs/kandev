---
id: "01-backend-instance-rebinding"
title: "Backend Sentry watch instance rebinding"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.3
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.4
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.5
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.6
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.7
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.8
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.9
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.10
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.11
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.12
system_design:
  - ../../specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md
---

# Task 01: Backend Sentry Watch Instance Rebinding

## Summary

Accept `sentryInstanceId` on the Sentry watch PATCH. An effective change
atomically rebinds the watch and clears its import history, last-polled
time, and last error. Reserve, attach, and release reject events from a
no-longer-current scope. Process-owned admission excludes destructive
operations while dispatch runs; revision CAS catches completed scope changes.

## In scope

- `UpdateIssueWatchRequest.SentryInstanceID *string`
  (`json:"sentryInstanceId,omitempty"`); update all three immutability comments
  in `models.go`: `IssueWatch`, create request, and update request.
- `UpdateIssueWatchRequest.ExpectedScopeRevision *int64`
  (`json:"expectedScopeRevision,omitempty"`): a supplied equality precondition
  is checked against the authorized current row and passed unchanged to the
  store CAS. Never substitute the current pre-read revision when the supplied
  value is stale. Omitted retains the older caller's server-pre-read behavior;
  explicit 0 is a real precondition, not omission. Test pre-read-stale scope,
  A->B->A, and Reset, for unchanged-instance and changed-instance requests.
- Rebind decision in `Service.UpdateIssueWatch`: blank rejects
  (`ErrInstanceRequired`), `requireInstance` validates workspace ownership,
  equal is a no-op, and any explicit legacy binding clears history. Do not
  infer history ownership from `resolveWatchInstanceID`.
- Add `scope_revision` to fresh schema and an idempotent additive migration
  after the existing instance-table rebuild. Extend `issueWatchRow`,
  `IssueWatch`, and `NewSentryIssueEvent` with the internal revision and carry
  it from the polled watch snapshot. Test fresh schema, old rows, and replay.
- `Store.UpdateIssueWatch` receives expected revision and rebind flag. Every
  update compares the captured revision; rebind increments it and atomically
  updates binding, deletes history, and clears poll/error state. New
  `ErrIssueWatchConflict` maps to HTTP 409 `SENTRY_WATCH_CONFLICT`; no automatic
  retry. Include A->B->A between pre-read and write.
- Return the committed `scopeRevision` as read-only watch response evidence,
  including after rebind; never return the captured pre-update revision.
  `httpDeleteIssueWatch` must use `writeIssueWatchError` instead of its
  hardcoded 500. Required DELETE and Reset HTTP tests hold real Dispatch or
  Reset ownership and assert 409 `SENTRY_WATCH_CONFLICT` with no effects.
- One Sentry-service-owned admission coordinator: shared live dispatch count
  or one exclusive rebind/Reset/deletion. Acquire an idempotent release handle
  before any dispatch preflight through optional
  `WatcherDispatchGuard.BeginDispatch` in the shared coordinator, inside the
  actual goroutine; release only when Dispatch exits. The Sentry implementation
  validates current writer-read revision/instance after admission and fails
  closed when unwired. Other watcher sources keep existing behavior.
- Rebind and watch deletion acquire exclusive try-admission before store
  mutation. Reset acquires it before listing IDs and retains it through
  `watchreset.Run`, including cancellation-detached task deletion. The Sentry
  resetter captures revision/snapshot and finalizes by revision CAS, clearing
  state and incrementing revision. No DB connection/transaction is retained
  across task-creation, deletion, or auto-start I/O.
- Add `instanceID` and `scopeRevision` to Reserve, Assign, and Release through
  `SentryService`, `sentryServiceAdapter`, and all source methods. All
  history-mutating transactions lock parent before children, including
  `DeleteIssueWatch` and `ResetIssueWatchState`. Do not leave either old
  child-first path. Update all callers/fakes and Sentry nil-service tests.
- Abandoned SQL reservations are safe to clear only after exclusive admission
  proves no live Dispatch owns them. Rebind may discard them with old history;
  Reset may clear them after a failed attach or restart. No age heuristic.
- Fold `TestService_UpdateIssueWatch_InstanceImmutable` and its obsolete
  comment into `..._KeepsHistoryWhenInstanceUnchanged`; cover omitted and equal
  instances with existing history, not absence of a request field.
- Regressions: history guards with instance/revision mismatch; paused live
  Create prevents Reset/rebind/delete with 409 and no effects; delayed event
  after rebind or Reset cannot reserve. A paused task deleter inside the real
  Sentry resetter blocks rebind/dispatch/delete and retains ownership through
  caller cancellation; reverse ordering resets only the new binding. Failed
  attach and process-restart cases prove safe abandoned-row recovery.
- Parent-before-child lock regressions cover Reset/Delete versus rebind,
  reserve, assign, and release on independent database connections in both
  orderings. Do not substitute a service gate test for SQL lock-order proof.
- Reset retention: pass a Sentry-only deleter adapter to `watchreset.Run`,
  always forwarding `DeleteTaskTree(ctx, id, false)`. Do not change shared
  watchreset or other integrations' cascade behavior. Extend the handoff
  non-cascade boundary to one observed-child normalize/detach transaction,
  proposed `DetachObservedChildrenForDelete`. It takes existing repository
  hierarchy admission, compares the complete current structural child ID set
  and workspace/parent/mode/orphan guards with the snapshot, then normalizes
  and detaches only validated IDs atomically. Child-set or guard drift
  conflicts without partial mutation; never bulk-detach an unseen child.
  Permit archived `inherit_parent` children without the active-only orphan
  guard; preserve archive state/current unrelated metadata and compensation
  preimages. Keep the final root-delete child check and no DB lock across
  lifecycle I/O. Cut over Handoff without an unsafe fallback; remove obsolete
  split helpers, leaving independent reparent callers alone.
  The real handoff-backed Reset regression includes an archived non-recorded
  inheriting child and an archived recorded inheriting child under a recorded
  parent, in both enumeration orders. All recorded IDs must disappear and
  finalization succeed; the non-recorded child stays archived, detached, with
  valid normalized workspace metadata. Correct Reset/Preview comments/counts.
- Race regression through real repository/handoff-backed Sentry Reset:
  pause after child enumeration, commit a non-recorded inheriting child,
  resume guarded detach. It must conflict safely or preserve/normalize every
  detached child, never leave a top-level inheriting survivor. Also cover
  observed mode/metadata drift and a new child after detach before final root
  deletion; verify root guard and conditional compensation. Use independent
  writer connections/barriers for SQLite and PostgreSQL, not fake echoes.
- Include `apps/backend/internal/integrations/AGENTS.md`'s obsolete immutable-
  binding statement in the implementation contract-text cutover.
- Info log on rebind (watch ID, old and new instance IDs).
- `@covers` anchors on new tests.

## Out of scope

- Frontend, i18n, and public docs (Tasks 02 and 03).
- Per-instance history or changes to other integrations' Reset semantics.
- Guarding the stale poll's last-polled/error stamps (remain a bounded risk).

## Acceptance

- A PATCH with a different in-workspace instance persists it, removes all
  history rows, nulls `last_polled_at` and the error, and leaves tasks and
  their metadata untouched. Rollback or stale revision leaves all data
  unchanged; an opening precondition fails even when A->B->A or Reset completed
  before the request's server pre-read. Do not rewrite the persisted revision
  from a supplied precondition.
- Omitted or equal `sentryInstanceId` leaves history intact; a foreign
  instance returns 404 `SENTRY_INSTANCE_NOT_FOUND`; blank returns 400
  `SENTRY_INSTANCE_REQUIRED`; any explicit legacy binding clears history.
- Live dispatch excludes Reset/rebind/delete; Reset excludes rebind and new
  dispatch until its destructive loop finalizes. Abandoned empty rows do not
  permanently block recovery. All history mutations are revision/instance-
  guarded and parent-first, with real multi-connection SQL proof.

## Verification

```bash
(cd apps/backend && go test -count=1 -run 'TestService_UpdateIssueWatch_|TestService_ResetIssueWatch_|TestStore_IssueWatchTask_|TestStore_SentryWatch_|TestHTTP_UpdateIssueWatch_|TestHTTP_DeleteIssueWatch_OperationConflict|TestHTTP_ResetIssueWatch_OperationConflict' ./internal/sentry)
(cd apps/backend && go test -count=1 -run 'TestSentrySource_|TestSentryDispatch_OperationAdmission' ./internal/orchestrator)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a reachable PostgreSQL test DSN; skipped tests do not satisfy acceptance}"
(cd apps/backend && go test -v -count=1 -run 'TestPostgresSentryWatch_LockOrder|TestPostgresSentryWatch_ScopeRevision|TestPostgresStoreSchemaReplay' ./internal/sentry)
(cd apps/backend && go test -count=1 ./internal/sentry ./internal/orchestrator ./internal/backendapp)
make -C apps/backend build
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
```

New required tests: `TestStore_SentryWatch_ScopeRevisionMigration`,
`TestStore_SentryWatch_LockOrder`, `TestService_UpdateIssueWatch_RevisionABA`,
`TestService_ResetIssueWatch_OperationOwnership`,
`TestService_ResetIssueWatch_CurrentHistoryHierarchyBoundary`,
`TestDeleteTaskTree_NoCascadeArchivedInheritingChildren`,
`TestDeleteTaskTree_NoCascadeChildSetChanged`,
`TestService_ResetIssueWatch_ConcurrentChildAttachment`,
`TestRepository_DetachObservedChildrenForDelete`,
`TestPostgresDetachObservedChildrenForDelete`,
`TestService_UpdateIssueWatch_OpeningScopeConflict`,
`TestHTTP_UpdateIssueWatch_OpeningScopeConflict`,
`TestSentryDispatch_OperationAdmission`, `TestPostgresSentryWatch_LockOrder`,
and `TestPostgresSentryWatch_ScopeRevision`. Assert they execute, not just
that a regex command exits successfully.
Also required: `TestHTTP_DeleteIssueWatch_OperationConflict` and
`TestHTTP_ResetIssueWatch_OperationConflict`, covering dispatch-owned and
Reset-owned admission with unchanged watch/history/tasks on rejection.

Focused dependency check (from repository root):

```bash
(cd apps/backend && go test -count=1 -run 'TestDeleteTaskTree_NoCascade' ./internal/task/service)
(cd apps/backend && go test -count=1 -run 'TestRepository_DetachObservedChildrenForDelete' ./internal/task/repository/sqlite)
: "${KANDEV_TEST_POSTGRES_DSN:?A skipped PostgreSQL race test is not acceptance}"
(cd apps/backend && go test -v -count=1 -run 'TestPostgresDetachObservedChildrenForDelete' ./internal/task/repository/sqlite)
```

The new handoff case must prove archived-child preservation/normalization and
retained compensation/CAS behavior, not only a mocked cascade flag.

For PostgreSQL concurrency, use one isolated schema, at least two physical
connections (verify distinct `pg_backend_pid()` values), pool capacity >= 2,
and deterministic barriers. A local concurrency fixture may raise
`OpenIsolatedPostgres`'s pool limit without changing shared test defaults; all
connections must retain its scoped search path. Use a shared file-backed
SQLite database with independently opened writer connections and the normal
busy timeout, not `newTestStore`'s one-connection `:memory:` fixture. Bound
waits and establish ordering with barriers, never timing sleeps.

## Files likely touched

- `apps/backend/internal/sentry/models.go`
- `apps/backend/internal/sentry/store.go`
- `apps/backend/internal/sentry/service.go`
- `apps/backend/internal/sentry/watch_operations.go` (new admission helper)
- `apps/backend/internal/sentry/migration_test.go`
- `apps/backend/internal/sentry/service_issue_watch.go`
- `apps/backend/internal/sentry/store_issue_watch.go`
- `apps/backend/internal/sentry/handlers_issue_watch.go`
- `apps/backend/internal/sentry/service_issue_watch_test.go`
- `apps/backend/internal/sentry/store_issue_watch_test.go`
- `apps/backend/internal/sentry/store_postgres_test.go`
- `apps/backend/internal/sentry/poller_issue_watch_test.go`
- `apps/backend/internal/sentry/handlers_test.go`
- `apps/backend/internal/orchestrator/event_handlers_sentry.go`
- `apps/backend/internal/orchestrator/watcher_dispatch.go`
- `apps/backend/internal/orchestrator/source_sentry.go`
- `apps/backend/internal/orchestrator/source_sentry_test.go`
- `apps/backend/internal/orchestrator/event_handlers_sentry_test.go` (new dispatch/rebind race suite)
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/integrations/AGENTS.md`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_cascade_test.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/task_hierarchy_admission.go`
- `apps/backend/internal/task/repository/sqlite/workspace_orphan_guard.go` (transaction-bound CAS reuse if needed)
- `apps/backend/internal/task/repository/sqlite/task_hierarchy_test.go`
- `apps/backend/internal/task/repository/sqlite/task_hierarchy_admission_postgres_test.go`

## Dependencies

None.

## Risks

- The admission handle must span Dispatch inside its goroutine; scoping it to
  bus delivery recreates the reserve/create race.
- Reset must own admission from before task enumeration through finalization,
  without holding database locks across the task deleter.
- Schema replay and legacy instance-table rebuild must retain scope revision;
  parent-first locking must include Reset and watch deletion.
- The supported single-backend-replica contract makes live ownership
  process-local. Do not claim multi-replica safety from PostgreSQL alone.
- PostgreSQL proof requires a reachable DSN and competing physical connections.
- Go lint function-length limits in `UpdateIssueWatch`: extract the rebind
  decision into a helper.

## Inputs

- `docs/specs/integrations/requirements/sentry-watch-duplicate-and-rebind.md`
- `docs/specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md`
- `docs/decisions/2026-10-24-sentry-watch-instance-rebinding.md`
- `apps/backend/AGENTS.md`

## Results

Pending.
