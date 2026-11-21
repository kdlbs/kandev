---
id: "01-persist-auto-sort-merge-policy"
title: "Persist Auto-sort-merge policy"
status: cancelled
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
---

# Task 01: Persist Auto-sort-merge policy

## Summary

Cancelled: superseded by [Task 06](task-06-fold-into-auto-merge.md), which folds
the sorted behaviour into Auto-merge and removes the separate setting.

Add the global setting (default OFF) and the per-session override with
revisions. Resolve the preference the same way as Auto-merge, project it on
queue status, and expose `message.queue.auto_sort_merge.set`. This work order
changes no fold or ordering behaviour.

## In scope

- The `queuesettings` field, patch, legacy-omitted `false`, revision bump on
  change, live publication, startup hydration, and admin-only PATCH.
- `queue_session_state.auto_sort_merge_override` and
  `auto_sort_merge_revision` in the fresh schema and replay-safe migration for
  SQLite and PostgreSQL, plus the memory map. Incarnation reset, transfer
  cleanup, deletion cleanup, and archive preservation.
- One combined `GlobalQueuePolicy` snapshot and loader (one settings read,
  one atomic publication) replacing the Auto-merge-only pointer and loader,
  resolution, and the in-transaction sort-policy validation helper. The
  loader is never called inside a repository transaction: admission
  attempts/retries and session policy writes load it before their
  transaction; `Service.Snapshot` keeps today's order (`repo.Snapshot`
  allocates the generation and commits, then the loader runs).
- The `QueueStatus` tuple with an unavailable marker, on `message.queue.get`
  and every session-scoped status producer.
- The WebSocket action, handler, controller interface, triplet validation,
  and status publication. `SetSessionAutoMerge` and `SetSessionAutoSortMerge`
  return `SessionPolicyResult`; `QueueAutoMergeController`, the new
  `QueueAutoSortMergeController`, and all test doubles migrate.
  `CompactedCount` is always `0` until Task 03.

## Out of scope

- Sorted folds (Task 02), compaction (Task 03), and UI (Task 04).

## Acceptance

- A fresh install or a legacy settings value without the field resolves global
  Auto-sort-merge to OFF. An admin PATCH persists it and bumps the revision
  only on a value change.
- A session without an override reports `auto_sort_merge_source: "global"` and
  follows later global changes. After `auto_sort_merge.set`, it reports
  `session` with revision ≥ 1 and survives a restart. An incarnation change or
  deletion clears it.
- An override read failure produces `auto_sort_merge_available: false` while
  the Auto-merge fields and entries still publish.
- A resolution paused across a concurrent two-field settings PATCH observes a
  wholly old or wholly new pair.
- A delayed failing status loader cannot produce a higher generation than a
  later successful status (deterministic channel-controlled test).
- Status reads, admission policy-changed retries, and both session switches
  succeed with the real settings loader and queue repository sharing one
  single-connection SQLite writer pool.

## Verification

```bash
test -n "$KANDEV_TEST_POSTGRES_DSN" || { echo "KANDEV_TEST_POSTGRES_DSN unset: PostgreSQL cases will skip; record as unverified"; }
(cd apps/backend && go test ./internal/system/queuesettings/... ./internal/backendapp/... -run 'QueueSettings|AutoSortMerge|Revision|ResolveQueue')
(cd apps/backend && go test ./internal/orchestrator/messagequeue/... -run 'AutoSortMerge')
(cd apps/backend && go test ./internal/orchestrator/handlers/... -run 'AutoSortMerge|AutoMerge')
```

## Files likely touched

- `apps/backend/internal/system/queuesettings/{types,store,service,handler}.go` and tests
- `apps/backend/internal/backendapp/orchestrator.go`, `message_queue_settings_test.go`
- `apps/backend/internal/orchestrator/messagequeue/{types,repository,repository_sqlite,repository_memory,service}.go`
- New `messagequeue/repository_auto_sort_merge_override_test.go` and `service_auto_sort_merge_policy_test.go`
- `apps/backend/internal/orchestrator/handlers/{queue_handlers,queue_operations_handlers}.go` and the new `queue_handlers_auto_sort_merge_test.go`
- `apps/backend/pkg/websocket/actions.go`
- Test doubles that implement the queue service interfaces, such as `handlers/queue_admission_timeout_test.go`

## Dependencies

None.

## Risks

- A missed incarnation-reset or transfer path would leave a stale override on
  a recreated session ID.
- If `storedSettings` uses a non-pointer bool, an omitted value could be
  confused with an explicit `false` on a later write.

## Parallelism

`sequential`

## Inputs

- System design: Policy model, Persistence, Global settings, Status contract.
- Patterns: `GetAutoMergeOverride`/`SetAutoMergeOverride`,
  `SetAutoMergePolicy`, `wsSetAutoMerge`, and `repository_auto_merge_override_test.go`.

## Results

- `pnpm install --frozen-lockfile` (from `apps/`): passed. Environment uses
  Node 22.22.3 rather than the declared Node 24 engine; no install failure.
- RED: focused settings, status, override, session-switch, and handler tests
  failed before their implementations. The transfer regression also caught
  missing memory-source preference cleanup; fixed without changing Auto-merge.
- `test -n "$KANDEV_TEST_POSTGRES_DSN" || { echo "KANDEV_TEST_POSTGRES_DSN unset: PostgreSQL cases will skip; record as unverified"; }`:
  unset. PostgreSQL override/upgrade/transfer cases skipped, unverified.
- `go test ./internal/system/queuesettings/... ./internal/backendapp/... -run 'QueueSettings|AutoSortMerge|Revision|ResolveQueue'`:
  passed. Initial attempts failed fetching `webpush-go` and `jwt/v5` through
  the blocked Go proxy. `GOPROXY=direct go mod download` for both dependencies
  succeeded, then the same check passed.
- `go test ./internal/orchestrator/messagequeue/... -run 'AutoSortMerge'`:
  passed after the memory transfer cleanup fix.
- `go test ./internal/orchestrator/handlers/... -run 'AutoSortMerge|AutoMerge'`:
  passed after preserving unbound-memory-session identity resolution.
- The startup integration test uses the real settings store and queue
  repository on one single-connection SQLite pool, exercising status,
  both session switches, and admission. Policy-changed admission retry is
  covered with the combined admission snapshot in Task 02.
- Race checks are recorded with the final change-aware checks in the plan.
