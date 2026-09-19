---
id: "02-auto-merge-admission"
title: "Integrate automatic merge admission"
status: completed
wave: 2
depends_on: ["01-auto-merge-repository"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-auto-merge.md"
---

# Task 02: Integrate automatic merge admission

## Acceptance

1. `messagequeue.Service` has a default-on automatic setting independent from
   manual merge; normal capped admissions return the surviving target when
   folded, preserve a separate source on skip/error, and do not auto-process
   append, coalesce, restore, retry, or reserved lifecycle APIs.
2. File-backed WebSocket admissions hold per-session admission serialization,
   insert a distinct rollback-safe source, claim attachments, then finalize the
   snapshotted fold. No later admission can target the provisional source;
   claim failure leaves the prior target byte-for-byte unchanged, and status
   publishes only the final state.
3. Integration tests prove concurrent ordering/drain behavior, cap-before-fold,
   all four manual/automatic switch combinations, and exact-ID peer/parent
   interrupt delivery with excluded workflow, clarification, plugin-source,
   append, coalesce, and restore paths.

## Verification

```bash
cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/orchestrator ./internal/mcp/handlers ./internal/backendapp
cd apps/backend && go test -race -count=1 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers
```

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/service_auto_merge_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_auto_merge_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification_test.go`
- `apps/backend/internal/mcp/handlers/message_task_test.go`
- `apps/backend/internal/backendapp/adapters_plugin_messenger_test.go`
- Existing queue/service/handler tests whose setup intentionally needs separate
  compatible rows.

## Dependencies

Task 01.

## Parallelism

Sequential.

## Inputs

- Spec: `What`, `API surface`, `Admission lifecycle and concurrency`, `Failure
  modes`, and all admission scenarios.
- Plan: `Backend > Admission integration` and corresponding test rows.
- Existing patterns: `WithSessionAdmission`, queue attachment claim rollback,
  `QueueAndInterruptForPeerMessage`, `QueueMessageWithCoalesceKey`,
  `RestoreMessage`, and status publishers.

## Risks

- Never report post-insert fold failure as failed admission.
- The deferred attachment API must carry the admission-time setting snapshot
  and retain the admission lock through claim/finalize; a settings change or
  later admission cannot retroactively alter the provisional source.
- A returned merged ID may identify an older target. Every exact-ID caller must
  use that returned value intentionally.

## Output contract

Report summary, files changed, exact commands/outcomes, blockers, risks, and
update this task plus `plan.md` status in the same conversation.

## Results

- Added an independent default-on live automatic-merge setting and admission
  folding that snapshots capacity/policy under the per-session lock.
- Added deferred after-insert finalization so staged attachments are claimed
  before folding; rollback and later admissions remain serialized by source ID.
- Kept capacity as a precondition, degraded post-insert fold errors to separate
  success, and preserved explicit append/coalesce/restore/retry contracts.
- Added service, handler, concurrency, storage-error, four-switch-combination,
  and exact surviving-ID parent-interrupt coverage. Legacy tests that require
  multiple compatible rows now explicitly disable automatic merging.
- Review remediation serializes queue drains, targeted takes, removals, edits,
  reorders, manual merges, cancel-all, and send-now claims with admission
  finalization; drain/removal race coverage now guards the provisional source.
- Verification passed:
  - `cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/orchestrator ./internal/mcp/handlers ./internal/backendapp`
  - `cd apps/backend && go test -race -count=1 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers`
- Blockers: none.
