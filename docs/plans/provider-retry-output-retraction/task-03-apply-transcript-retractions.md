---
id: "03-apply-transcript-retractions"
title: "Apply transcript retractions"
status: done
wave: 3
depends_on:
  - "02-map-provisional-stream-records"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.28
system_design:
  - ../../specs/platform/system-design/provider-response-attempt-recovery.md
---

# Task 03: Apply Transcript Retractions

## Summary

Consume lifecycle's provider-neutral retraction list under the orchestrator's
session stream guard. Delete each abandoned row through the task service so
storage and every connected client receive the same result.

## In scope

- Narrow task-message retraction service and backend composition.
- Reset handling after existing execution, generation, and cancellation gates.
- Durable assistant and thinking message deletion.
- Existing `session.message.deleted` publication.
- Per-record failure continuation and structured logging.
- Task-service-backed orchestration tests.

## Out of scope

- Adapter metadata matching or lifecycle ID ownership.
- New task-message schema or frontend event type.
- Retrying a failed database mutation indefinitely.

## Acceptance

- A current reset deletes every supplied message ID through the task service
  and publishes one existing deletion notification per successful row.
- Empty input is a no-op; stale and completed execution events cannot delete
  messages.
- One failed deletion is logged without stopping later deletions or the live
  provider turn, and missing production wiring is caught by composition tests.

## Verification

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestHandleResponseAttemptReset')
(cd apps/backend && go test -race ./internal/backendapp -run 'Test.*StreamingMessageRetraction')
```

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_retraction_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/orchestrator_streaming_retraction_test.go`

## Dependencies

- Task 02 supplies fenced Kandev message IDs in stream order.

## Risks

- Bypassing the task service would remove durable data without notifying live
  clients.
- A handler outside the per-session stream guard can race replacement output or
  cancellation.

## Parallelism

`sequential`

## Inputs

- Provider Error Recovery criteria `.27` and `.28`.
- Durable and live cleanup section in the response-attempt recovery design.
- Existing transient-retry notice cleanup and task-service deletion patterns.

## Results

The orchestrator now consumes `response_attempt_reset` under its existing
per-session stream guard, requires the current nonzero prompt generation, and
rejects terminal executions before mutation. A dedicated retraction seam is
wired to the task service in production. Each successful deletion therefore
removes the durable row and publishes the existing `MessageDeleted` event; an
individual failure is logged and later IDs are still attempted.

The gateway now routes `MessageAdded`, `MessageUpdated`, and `MessageDeleted`
through the same ordered wildcard subscription. A NATS-like transport test
delays deletion delivery and schedules the replacement subject first, proving
that the replacement remains queued behind the deletion before journal or
client fan-out.

Verified handler behavior, durable SQLite deletion, deletion-event publication,
inactive ownership, completion fencing, failure continuation, and production
composition with:

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestHandleResponseAttemptReset' -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run 'Test.*StreamingMessageRetraction' -count=1)
(cd apps/backend && go test ./internal/gateway/websocket -run 'TestTaskEventBroadcaster_OrdersTranscriptMutationsAcrossTransportSubjects' -count=1)
```
