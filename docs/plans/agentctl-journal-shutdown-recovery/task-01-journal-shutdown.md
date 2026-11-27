---
id: "01-journal-shutdown"
title: "Make journal shutdown safe for late consumers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 01: Make journal shutdown safe for late consumers

## Summary

Return a typed closed-journal error from late operations and stop instance stream owners before releasing the journal.
One stopped instance must not crash agentctl or interrupt sibling instances.

## In scope

- Audit all journal files for database access. Guard closed state under the database lifetime lock.
- Add deterministic close/replay and operation-after-close regressions before production changes.
- Cover reads, writes, replay, acknowledgment, submission state, retirement, health, compaction, and repeated close.
- Preserve commit/reopen evidence and exclusive database ownership.
- Cancel and drain instance stream owners under the existing teardown deadline.
- Handle late closed-journal errors in API writers without panic, busy loops, false empty success, or leaked goroutines.
- Exercise one stopping instance beside one continuing instance in the same agentctl process.

## Out of scope

Session recovery presentation, prompt admission changes, global panic suppression, and increased queue capacity.

## Acceptance

1. `TestJournalShutdownLifecycle` and `TestJournalCloseDuringReplay` fail on the current panic and pass with typed errors.
2. `TestInstanceTeardownDoesNotCrashSiblingStream` proves that the sibling delivers output while the stopped instance releases its stream resources.
3. Committed records replay after reopen, repeated close is safe, and the race detector reports no database lifetime race.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agentctl/server/process -count=1)
git diff --check
```

Use synchronization barriers instead of timing sleeps. Include the real shared-process boundary in the sibling regression.

## Files likely touched

- `apps/backend/internal/agentctl/journal/journal.go` and other database-access files in that package
- `apps/backend/internal/agentctl/journal/journal_shutdown_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`
- `apps/backend/internal/agentctl/server/api/instance_teardown_stream_test.go` (new)

## Dependencies

None.

## Risks

Waiting for streams while holding a lock they need can deadlock. Keep cancellation and waiting outside storage locks.
Do not close retained journals on backend detach.

## Parallelism

`sequential`

## Inputs

- [Design: journal shutdown and stream lifetime](../../specs/platform/system-design/durable-agent-reattachment.md#journal-shutdown-and-stream-lifetime).
- Existing `journal_test.go` close/reopen tests and `durableAgentStreamWriter` implementation.
- Incident stack and sequence in the plan.

## Results

Implemented closed-journal guards for every public database operation and a typed `ErrJournalClosed` result. Agent stream handlers now use process-owned admission; teardown cancels the stream, closes its WebSocket, and drains the reader, writer, and request handlers before normal journal closure. If the drain deadline expires, teardown closes storage so late consumers receive `ErrJournalClosed`; a later teardown retry can finish process cleanup.

Verification passed:

- `(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agentctl/server/process -count=1)`
- Focused shutdown regressions for operations after close, close during replay, deadline expiry, and sibling stream continuity.
- `git diff --check` and gofmt checks.
