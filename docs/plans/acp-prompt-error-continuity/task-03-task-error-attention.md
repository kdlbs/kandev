---
id: "03-task-error-attention"
title: "Persist task turn-error attention"
status: done
wave: 3
depends_on:
  - "02-preserve-prompt-error-runtime"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-004
  - REQ-PLATFORM-TURN-CONTINUITY-005
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.4
  - AC-PLATFORM-TURN-CONTINUITY-004.3
  - AC-PLATFORM-TURN-CONTINUITY-004.5
  - AC-PLATFORM-TURN-CONTINUITY-005.1
  - AC-PLATFORM-TURN-CONTINUITY-005.2
  - AC-PLATFORM-TURN-CONTINUITY-005.3
  - AC-PLATFORM-TURN-CONTINUITY-005.5
  - AC-PLATFORM-TURN-CONTINUITY-005.6
  - AC-PLATFORM-TURN-CONTINUITY-005.7
  - AC-PLATFORM-TURN-CONTINUITY-005.8
system_design:
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 03: Persist task turn-error attention

## Summary

Keep a durable task error signal after a retained turn fails, without blocking the surviving conversation.
Deliver the compact projection and stamped acknowledgement contract for the UI work order.

## In scope

- Add a task-owned `last_turn_error` model in existing metadata, with source session/turn/message identity and acknowledgement metadata.
- Use the idempotent error message ID as the stamp and its persisted creation time for ordering.
  Replaying an existing error row does not give it a new occurrence time.
- Keep the latest pointer for task attention while preserving every error in conversation history.
- Record it only through the trusted failed-turn owner, before durable failure settlement releases successor admission.
- Keep automatic recovery's current notice ownership. Create attention when recovery produces its manual failed-turn entry.
- Add task service operations and conditional metadata storage for recording and acknowledging the observed stamp.
  Reuse task metadata admission, access checks, and task/summary event publication.
- Reserve the system-owned metadata key from generic public metadata mutation.
- Add `POST /api/v1/tasks/:id/turn-error/dismiss` with required `stamp`, authorized access, idempotent repeat dismissal, and stale-stamp conflict handling.
- Accept optional client `observed_turn_error_stamp` on new manual input, and validate it against the current task error and source session.
  Never substitute the backend's current stamp. Cross-session, missing, and stale observation leave attention unchanged without rejecting valid input.
- Persist the original observation with submission identity before acknowledging durable dispatch/queue acceptance.
  Include it in replay/fingerprint validation. Duplicate retries cannot add a newer stamp to an old accepted submission.
- Input queued before a failure must not clear that later error when it drains, retries, or resumes.
  Queue edits and Send now on existing entries retain their original intent. New manual submissions can carry fresh observation.
- Rejected sends, duplicate requests, workflow/internal/agent input, drafts, focus, reads, and ordinary output create no acknowledgement.
- Keep accepted input accepted if acknowledgement fails. Preserve the badge and report the acknowledgement failure without resending the prompt.
- Add the optional bounded `turn_error` task summary projection and all semantic equality, boot, rebuild, event, revision, and persistence paths.
- Keep `active_error`, task activity, runtime recovery, pending input, and sibling sessions independent.
- Cover SQLite behavior and the existing environment-gated PostgreSQL path for every changed dialect-sensitive operation.

## Out of scope

- Frontend badge selectors, controls, translations, menus, and browser tests. Task 04 owns those changes.
- New tables, schema migrations, retry owners, notification providers, or per-session acknowledgement inventories.
- Runtime stop/restart, historical message deletion, automatic replay, and workflow success changes.

## Acceptance

1. A manual retained failure persists a current task attention pointer and bounded summary without writing blocking session recovery state.
   Duplicate or stale failure delivery cannot re-arm an acknowledged older error.
2. Explicit dismissal acknowledges its observed stamp. New manual/queued input requires a matching client-observed stamp from the same session.
   Stale requests, rejected/duplicate input, foreign tasks, and storage failures cannot hide a newer failure or repeat accepted input.
3. Boot, live events, reload/restart rebuild, and semantic summary updates agree.
   Mixed-session tests preserve a newer sibling error and independent runtime/activity/pending indicators.

## Manual input admission inventory

Audit each entry point and carry acknowledgement intent to its existing durable acceptance owner:

| Entry point | Current owner | Acknowledgement policy |
| --- | --- | --- |
| Direct WS `message.add` | `task/handlers/message_handlers.go: wsAddMessage` | New client ID plus observed same-session stamp; replay lookup remains side-effect free |
| Mid-turn steer | The `wsAddMessage` steer branch and recorded-message dispatch | Same new-submission rule after accepted steering; no acknowledgement on refusal |
| Queue enqueue `message.queue.add` | `orchestrator/handlers/queue_handlers.go: wsQueueMessage` | Accept supplied observation once; retain it on the queue item/submission |
| Queue update/append/merge/reorder and edit leases | `queue_handlers.go` and `queue_operations_handlers.go` | Maintenance of existing input does not acquire new acknowledgement intent |
| Queue Send now / interrupt and automatic drain | `queue_operations_handlers.go: wsSendNow`, `wsDrainQueue` | Existing entries keep original intent; never sample a later stamp during dispatch |
| Retry/resubmit | Existing queue retry or a new `message.add` client ID | Queue retry retains intent; only an explicit new logical submission can carry fresh observation |
| Task and phone composers | `use-session-prompts.ts`, `use-queue-admission.ts`, `use-queue.ts` | Shared request field and target-session validation; phone does not get a second acknowledgement owner |
| Quick Chat | `quick-chat-session-view.tsx`, `quick-chat-content.tsx`, shared composer hooks | Same task-backed session rule; inline dismissal only, with no new normal task menu or row |

Do not let internal dispatch paths create manual origin or replace a missing observation.
If an additional route bypasses these owners, name and cover it before marking this task done.

## Verification

Start these regressions in RED before the minimum implementation:

- `TestTaskTurnErrorAttentionProjection`
- `TestTaskTurnErrorSanitizedUTF8Bounds`, covering 512/4096-byte multibyte boundaries after sanitization
- `TestDismissTaskTurnErrorIfStamp`
- `TestAcceptedManualPromptAcknowledgesTaskTurnError`, including unseen sibling errors and queued-before-failure automatic drain
- `TestTaskTurnErrorDismissalPreservesNewerSiblingFailure`

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/models ./internal/task/statussummary ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/orchestrator ./internal/orchestrator/handlers -run 'TaskTurnError|TurnErrorAttention|AcceptedManualPrompt|AgentTurnFailed|StatusSummary' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/persistence/storeconformance -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Repeat the changed PostgreSQL behavior tests with `KANDEV_TEST_POSTGRES_DSN` configured on the same Go test command.
If no PostgreSQL test database is available, record that coverage as not run, with the exact reason.
Use `bash scripts/lint-go-changed` on every changed Go production/test file before marking this work order done.
Use existing transaction admission. Do not hold SQL transactions across external calls.

## Files likely touched

- New `apps/backend/internal/task/models/turn_errors.go` and model tests.
- `apps/backend/internal/task/service/reserved_metadata.go` and metadata mutation tests.
- New `apps/backend/internal/task/service/service_task_turn_error.go` and service tests.
- `apps/backend/internal/task/repository/sqlite/metadata_launch_error_cas.go`, or a focused adjacent turn-error CAS file using its existing pattern.
- New SQLite/PostgreSQL task-turn-error behavior tests beside the current task metadata CAS tests.
- `apps/backend/internal/task/handlers/task_handlers.go`, `task_http_handlers.go`, `message_handlers.go`, and focused API/replay tests.
- `apps/backend/internal/task/statussummary/model.go`, `projector.go`, `projector_events.go`, `projector_derive.go`, and `rebuild.go` with adjacent tests.
- `apps/backend/internal/task/service/service_status_summary_rebuild_input.go` and `service_status_summary_reconcile.go`.
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go` and its tests for manual failed-turn attention ownership.
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`, `queue_operations_handlers.go`, and their accepted/rejected/duplicate tests.
- `apps/backend/internal/orchestrator/messagequeue/` input metadata/identity where observation must survive queue acceptance and redispatch.
- Existing task repository/service interfaces only where the new atomic operations require them.

## Dependencies

Task 02. Task 04 consumes the persisted summary and acknowledgement API.

## Risks

- Reusing blocking recovery metadata would disable the composer the lifetime change preserves.
- A delayed dismissal or accepted-input callback can otherwise erase a newer sibling failure.
- A generic full-task update can overwrite unrelated metadata or ownership fields. Use conditional metadata operations.
- A failed acknowledgement after accepted input must never cause duplicate submission.
- Mixed-version clients ignore the optional summary field and must keep existing recovery semantics.

## Parallelism

`sequential`

## Inputs

- [Design: durable task attention](../../specs/platform/system-design/acp-prompt-error-continuity.md#durable-task-attention)
- [Design: explicit acknowledgement](../../specs/platform/system-design/acp-prompt-error-continuity.md#explicit-acknowledgement-and-accepted-prompts)
- [Bounded task status delivery](../../specs/platform/system-design/bounded-task-status-delivery.md)
- Existing task metadata CAS, `DismissLastAgentError` authorization/event pattern, and summary rebuild tests.

## Results

Done. A failed manual retained turn writes `last_turn_error` task metadata with
the idempotent message id as its stamp. The pointer keeps only the latest
failure, preserves every transcript entry, and never enters blocking recovery
metadata. Task status summaries expose a bounded optional `turn_error`
projection without transcript payloads.

Added:

- `models.TaskTurnError` with UTF-8-safe 4096-byte message and 64-byte stamp
  bounds, plus acknowledgement metadata.
- `RecordTaskTurnError` and `AcknowledgeTaskTurnErrorIfStamp` under the task row
  lock (PostgreSQL advisory lock included), preserving unrelated metadata.
- `POST /api/v1/tasks/:id/turn-error/dismiss` with required stamp, idempotent
  repeat, 409 for a stale stamp, and existing access hiding.
- Optional `observed_turn_error_stamp` on direct messages and queue admission.
  Acknowledgement requires a new accepted manual submission, the same target
  session, and a matching current stamp. Omitted or stale evidence never rejects
  input.
- Summary projector load/refresh/rebuild paths and gateway wiring.

Verification completed:

- `go test -tags fts5 ./internal/task/models ./internal/task/statussummary ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/orchestrator ./internal/orchestrator/handlers ./internal/backendapp`: passed.
- `go run ./cmd/sqlguard ./internal`: passed.
- `go test -tags fts5 ./internal/persistence/storeconformance`: passed.
- A generic full-row task write now preserves the live `last_turn_error` value
  (including acknowledgement) from the locked row, for both `UpdateTask` and
  `UpdateTaskPreservingDeferredLaunch`, including a nil stale metadata snapshot.
  `MergeTaskMetadata` excludes the server-owned key. A SQLite regression covers
  the stale-snapshot race; the matching PostgreSQL regression is provided and
  skips unless `KANDEV_TEST_POSTGRES_DSN` is configured.
- Recording ordering uses the persisted failed-turn message's immutable
  `CreatedAt` (resolved through the message creator), not the recording clock.
  A delayed older settlement cannot replace or re-arm newer attention.
- Boot/list reconciliation now compares and repairs `turn_error` on existing
  summaries from task metadata, with the normal CAS revision guard.
- Accepted direct and queued prompts return a nonfatal
  `turn_error_acknowledgement_failed` response field on a persistence failure;
  the prompt is still accepted exactly once and the client shows localized
  guidance to dismiss manually.
- New regressions: `TestTaskTurnErrorAttentionProjection`,
  `TestTaskTurnErrorSanitizedUTF8Bounds`, `TestDismissTaskTurnErrorIfStamp`,
  `TestTaskTurnErrorDismissalPreservesNewerSiblingFailure`,
  `TestAcceptedManualPromptAcknowledgesTaskTurnError`,
  `TestQueueAdmissionAcknowledgesOnlyNewObservedTurnError`,
  `TestAcceptedManualPromptWithoutObservationLeavesAttention`,
  `TestStaleFullTaskUpdatesPreserveTurnErrorAttention`,
  `TestPostgresStaleFullTaskUpdatePreservesTurnErrorAttention`,
  `TestReconcileExistingSummaryRepairsTurnErrorAttention`,
  `TestDelayedOlderTurnErrorCannotReplaceNewerAttention`, and
  `TestAcceptedManualPromptReportsAcknowledgementFailure` /
  `TestQueueAdmissionReportsAcknowledgementFailure`.
- PostgreSQL behavior was not repeated because no `KANDEV_TEST_POSTGRES_DSN` was
  available in this environment.
