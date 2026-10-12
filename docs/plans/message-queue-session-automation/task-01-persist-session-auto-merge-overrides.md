---
id: "01-persist-session-auto-merge-overrides"
title: "Persist Session Auto-merge Overrides"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.5
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.7
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.8
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.11
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.12
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.13
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.16
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.17
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.18
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.19
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.21
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.22
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.23
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.24
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.25
system_design:
  - ../../specs/ui/system-design/message-queue-automation-controls.md
---

# Task 01: Persist Session Auto-merge Overrides

## Summary

Add a nullable, queue-owned per-session automatic-merge override with a
monotonic revision and resolve it against the revisioned live install-wide
fallback. Give every task-session row an immutable queue incarnation UUID.
Project that incarnation, status epoch/generation, policy availability, value,
source, and revision through every session-scoped producer before any frontend
control consumes it.

## In scope

- Write failing queue-settings, startup wiring, service, repository, handler,
  orchestrator, gateway, and MCP producer tests before production code.
- Persist absent, explicit OFF, and explicit ON as distinct states with a
  monotonic session revision in memory, SQLite, and PostgreSQL-compatible
  storage.
- Introduce required `QueueSessionIdentity{TaskID, SessionID,
SessionIncarnationID}` arguments across every service and repository
  operation that inserts, appends, replaces, restores, reserves, acknowledges,
  consumes, updates, removes, merges, reorders, clears, transfers, or changes
  session policy. Admission, coalescing, retry, pending-move, drain, Send Now,
  Auto-run, and Auto-merge paths retain captured identities; remove every
  textual-ID mutating overload and `QueueMessageWithMetadataAfterInsert`.
- Require `task_id`, `session_id`, and `session_incarnation_id` on every browser
  queue read or mutation. Gateway and handler authorize the supplied pair, then
  pass the unchanged triplet to persistence. Stale identity returns the existing
  non-enumerating session-not-found response without mutation, claim, dispatch,
  attachment, response-state, or status-publication effects.
- Define `SQLSessionAuthority` and `MemorySessionAuthority` in `messagequeue`.
  The SQL seam locks tasks, validates sessions, and claims a typed
  `QueueAttachmentClaim` on the queue transaction; the memory seam supplies an
  equivalent claim-capable guarded boundary. Backend wiring injects the task
  repository into the queue repository. Remove `tasksTablePresent` and every
  missing-schema or nil-authority bypass. SQL fixtures create task, session, and
  attachment authority rows; memory fixtures seed a lock-protected authority
  whose guard encloses bucket mutation.
- Add one repository policy-aware admission operation carrying the identity.
  SQL locks the active task row, then the durable `queue_session_locks` row,
  revalidates that exact session incarnation, and reads the override plus
  inserts or directly folds in one transaction. Route override writes and
  lifecycle-coalescing inserts through the same identity check and lock order.
  The memory repository validates through task-session authority while holding
  its incarnation-tagged bucket mutex.
- Resolve authenticated attachment owner/workspace and staged IDs before queue
  admission. Pass the claim value into the policy-aware repository operation;
  identity validation, insert, attachment claim, and eligible fold share one
  transaction or memory critical section. Any failure rolls back every queue
  and attachment change. A two-connection test pauses inside the claim seam and
  races session deletion/recreation in both orders.
- Add deterministic two-connection tests for every mutation category and
  identity-bound status publication against session/task/workspace recreation,
  including a pause after mutation commit before status snapshot. Cover
  opposite transfer ordering and deferred-transition pauses before writes and
  after commit before effects. Channel-controlled tests recreate after a drain,
  Auto-run, or Send Now claim and before prompt/restore. Prove stale work cannot
  mutate, dispatch to, publish, restore into, block, or clear replacement state.
- Persist an internal global revision with the automatic-merge setting, advance
  it atomically only when that boolean changes, publish one immutable
  value/revision runtime snapshot after commit, and hydrate the same pair during
  backend startup.
- Preserve inheritance for untouched sessions across later global changes and
  isolate explicit overrides by session.
- Preserve the shipped automatic-fold timestamp behavior: the target row
  survives while its queued time becomes the later target/source time.
- Apply one effective policy tuple to ordinary and eligible direct folds.
- Keep staged-attachment admissions at or above capacity out of the direct fold
  path, with no attachment claim or tail mutation.
- Add queue-owned policy cleanup under a common transaction helper that locks
  sorted unique task rows through the injected authority, then sorted
  repository-local and durable queue-session locks, then revalidates every
  identity. Session, task, and workspace deletion remove state atomically;
  archive preserves it. Transfer validates exact source and destination
  identities in that order, removes source state, and leaves the destination
  override unchanged.
- Add a separate deletion-only post-commit callback carrying complete
  task/session/incarnation identities to the memory repository. Keep archive's
  purger/recount notifier separate. Memory cleanup CAS-purges only exact
  incarnation-tagged buckets.
- Add `task_sessions.queue_incarnation_id`, generate it once during session
  creation, and expose it on `TaskSession`. Register its replay-safe ALTER and
  distinct empty-row UUID backfill after both existing `task_sessions` rebuild
  migrations; require future rebuild projections to preserve it. Test legacy
  rebuild order, partial-backfill restart, stable reread, and textual ID reuse.
- Add the authorized `message.queue.auto_merge.set` mutation with required
  `session_incarnation_id` plus status epoch/generation,
  `auto_merge_available`, and the effective policy tuple. Compare the expected
  incarnation again under the persistence locks and return the existing
  non-enumerating session-not-found response on mismatch.
- Add `pending_moves.session_incarnation_id`; backfill it by joining the owning
  task session and delete unmatched legacy rows. Carry `QueueSessionIdentity`
  through set/read/take, retry, reaper CAS, snapshot/restore, and transfer.
- Make deferred apply one transaction that CASes captured identity, pending move,
  expected source, and target; applies direct writes; increments task workflow
  transition generation; and returns an immutable effect token. Same-target
  uses the same operation.
- After commit, task effects CAS token generation/target and session/queue
  effects validate token identity/generation. Publication, history, exit/enter,
  prompt cleanup, drain, and pull-next fail closed after recreation or a
  successor move.
- Replace separate `GetStatus`/`SessionTaskID` assembly with
  `Snapshot(QueueSessionIdentity)`. Browser requests supply expected identity;
  internal work retains it. Responses/events snapshot that same identity after
  commit and publish nothing on mismatch.
- Put identity plus operation generation on queue reservations, `SendNowClaim`,
  queued dispatch/cancellation, deferred dispatch, and in-flight ownership.
  Restore/ack revalidates through the repository; cancellation, prompt, and
  cleanup verify identity/generation against task-session and execution
  ownership without holding database locks across agent I/O.
- Update queue handler, session-scoped orchestrator lifecycle, and MCP producers
  and tests for ordered available and unavailable shapes. Give task-only
  status-summary signals an explicit scope and keep them out of frontend
  session reconciliation.
- Replace the post-delete `CancelAll` path in `task_operations.go` with the
  repository-owned transactional cleanup and memory callback. Publish only
  `publishTaskQueueStatusEvent(taskID, "")` after deletion.
- Make gateway queue routing consume explicit task-scoped status internally
  without browser broadcast. Drop invalid empty-session queue events rather
  than falling through to an empty-workspace broadcast.
- Preserve user-visible snapshot/restore, manual merge, Auto-run, and Send Now
  semantics while migrating their internal ownership to exact identities.

## Out of scope

- Frontend state or queue-panel controls.
- A clear-override API.
- Changing automatic-merge compatibility, content/entity combination, or
  retroactively folding queued rows. Queued-time behavior stays aligned with
  the existing `latestQueuedAt` implementation.
- Changing the global settings endpoint or administrator permissions.

## Acceptance

- Tests first fail on missing inheritance, authority/claim injection, complete
  request and internal identities, task-row/session-lock ordering, attachment
  rollback, atomic deferred writes, guarded post-commit effects, identity-bound
  publication, async drain/Auto-run/Send Now ownership, stale mutation/status/
  transfer/pending-move rejection, lifecycle cleanup, migrations, task-only
  routing, startup snapshots, unavailable status, capacity, or producer shape;
  minimum implementation makes them pass.
- Untouched sessions use every later committed global snapshot; restart retains
  its persisted revision, and an explicitly changed session retains its value
  and monotonic revision across queue drain and repository reconstruction
  without changing another session.
- Every queue read/mutation uses exact typed identity and task-then-session
  revalidation. Ordinary admission uses one policy snapshot; staged claim,
  insert, and fold share the transaction. Claim failure rolls back all writes;
  mismatch publishes no status; task-only signals never reach browsers.
- Deferred writes CAS identity/move/source/target in one transaction and return
  an effect token. Task effects CAS transition generation/target; session
  effects validate identity/generation. Pre-write, post-commit, recreation, and
  successor-move races cannot redirect an effect.
- Drain, Auto-run, deferred dispatch, and Send Now reservations/claims retain
  identity/generation through cancel, prompt, restore, acknowledgement, and
  cleanup. Delayed old work cannot dispatch to, block, or clear a replacement.
- Session/task/workspace deletion removes policy; archive preserves it;
  transfer removes source state and preserves destination policy. Session
  delete performs no second `CancelAll` and emits only a task recount.
- Every browser read/mutator and internal producer carries expected identity.
  Responses/events snapshot that identity and publish nothing after mismatch.
  A recreated textual ID receives a new incarnation; task-only status has none.
- Automatic fold tests retain the target identity and assert that its queued
  time is the later target/source time.

## Verification

```bash
(cd apps/backend && go test -race ./internal/system/queuesettings ./internal/backendapp ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/orchestrator ./internal/mcp/handlers ./internal/gateway/websocket ./internal/task/repository/sqlite)
(cd apps/backend && go test ./pkg/websocket)
if [ -n "${KANDEV_TEST_POSTGRES_DSN:-}" ]; then (cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/orchestrator/messagequeue); else printf '%s\n' 'SKIP: KANDEV_TEST_POSTGRES_DSN unavailable'; fi
git ls-files --cached --others --exclude-standard -- '*.go' | xargs -r gofmt -l
```

The final formatting command must print no files. The conditional PostgreSQL
command reruns every changed package containing DSN-gated coverage: task-session
ALTER/rebuild/backfill and destructive lifecycle tests in
`internal/task/repository/sqlite`, plus queue policy and cross-connection tests
in `internal/orchestrator/messagequeue`. Its explicit output records an
environment skip when the DSN is unavailable.

## Files likely touched

- `apps/backend/internal/system/queuesettings/types.go`
- `apps/backend/internal/system/queuesettings/store.go`
- `apps/backend/internal/system/queuesettings/service.go`
- `apps/backend/internal/system/queuesettings/queuesettings_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/orchestrator_wiring_test.go`
- `apps/backend/internal/backendapp/adapters_plugin_messenger.go`
- `apps/backend/internal/backendapp/adapters_plugin_messenger_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/session_test.go`
- `apps/backend/internal/task/repository/sqlite/session_incarnation_migration_test.go`
- `apps/backend/internal/task/repository/sqlite/base.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/attachment.go`
- `apps/backend/internal/task/repository/sqlite/attachment_postgres_test.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/workspace.go`
- `apps/backend/internal/task/repository/sqlite/task_queue_purge_notify_test.go`
- `apps/backend/internal/task/repository/sqlite/session_queue_state_cleanup_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_auto_merge_override_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service_auto_merge_test.go`
- `apps/backend/internal/orchestrator/messagequeue/pending_move_identity_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_postgres_pending_move_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_auto_merge_policy_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_publish_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_send_now_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_merge_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_reorder_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/queue_admission_identity_test.go`
- `apps/backend/internal/orchestrator/workflow_store.go`
- `apps/backend/internal/orchestrator/workflow_store_deferred_identity_test.go`
- `apps/backend/internal/orchestrator/queued_dispatch.go`
- `apps/backend/internal/orchestrator/queued_dispatch_identity_test.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_test.go`
- `apps/backend/internal/orchestrator/queue_auto_run_test.go`
- `apps/backend/internal/orchestrator/queue_auto_run_review_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_general_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_test.go`
- `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/session_delete_contract_test.go`
- `apps/backend/internal/orchestrator/pending_move_identity_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/queue_status_event_test.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/gateway/websocket/task_notifications_test.go`
- `apps/backend/internal/gateway/websocket/dispatch_scope.go`
- `apps/backend/internal/gateway/websocket/dispatch_scope_test.go`
- `apps/backend/internal/mcp/handlers/config_task_handlers.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

None.

## Risks

- Converting a missing override to `false` would stop untouched sessions from
  following the global value; converting it to `true` would ignore a global OFF
  value.
- An upsert that replaces the whole state row could reset the independent
  Auto-run value.
- A service-only admission gate does not serialize another repository
  connection. Admission, override writes, and deletion must all acquire task
  row before session lock and validate expected incarnation; reversing the
  order deadlocks task/workspace purge, while omitting validation lets a stale
  waiter target a recreated ID.
- Reusing the archive task purger for policy cleanup either erases surviving
  archive policy or leaves deleted memory overrides. A separate deletion-only,
  incarnation-aware callback owns memory policy cleanup.
- Reusing a textual session ID without a distinct stable incarnation lets a
  delayed status from the deleted row restore its irreversible session-source
  tuple. Creation and legacy backfill must always supply a non-empty UUID, and
  every session-scoped producer must echo it.
- Adding the incarnation migration before a legacy `task_sessions` rebuild
  silently drops the column. It runs after all current rebuilds, and every
  future rebuild must project it.
- A status producer that re-resolves identity after mutation commit can publish
  replacement state. Responses/events snapshot the operation's expected
  identity; browser reads carry the same triplet through authorization.
- A textual-ID mutator or async ownership record lets delayed work target or
  clear a replacement. Compile-time signatures, identity/generation-tagged
  claims, and post-claim race tests enforce the complete inventory.
- Letting the queue repository probe for task tables or accept a nil authority
  makes production identity validation optional and isolated tests
  unrepresentative. Constructors require injected authorities, and fixtures
  explicitly seed them.
- Transfer that locks session rows before both task rows in deterministic order
  can deadlock deletion or move rows across recreated identities. It uses the
  common task-then-sorted-session transaction helper and validates both ends.
- The current after-insert callback runs after the queue transaction commits,
  so compensation cannot prevent a delete/recreate claim race. Attachment claim
  becomes a typed authority operation inside the admission transaction; the
  callback and rollback delete are removed.
- Atomic deferred writes do not protect effects after commit. Task effects CAS
  committed move/target while session and queue effects retain identity, with
  deterministic recreation tests on both sides of commit.
- PostgreSQL and SQLite represent nullable booleans differently at the driver
  boundary; tests must exercise the repository contract rather than raw SQL
  text.
- Running only message-queue PostgreSQL tests misses task-session migration and
  lifecycle code; the conditional DSN pass covers both repository packages plus
  orchestrator and gateway integration.
- Missing one session-scoped producer leaves clients vulnerable to value
  regression even if the primary queue handler is correct; task-only signals
  need an explicit exemption.
- The persisted global value and revision must share one write, and the runtime
  service must publish, hydrate, and read them as one immutable snapshot; a
  boolean-only startup path or two atomics can create a policy generation that
  never existed.
- Treating a status-read failure as authoritative OFF would replace valid client
  state; an unsequenced unavailable marker could also disable newer success.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001`
- `docs/specs/ui/system-design/message-queue-automation-controls.md`
- `docs/decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_auto_run_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`

## Results

Completed. Added durable task-session queue incarnations, revisioned global and
per-session Auto-merge policy, identity-bound browser and internal queue
producers, pending-move identity retention, destructive lifecycle cleanup, and
ordered status snapshots. Admission skips folding when policy reads fail;
status reads preserve entries while reporting policy unavailable. SQLite,
orchestrator, handler, gateway, MCP, migration, and race-focused tests pass.
