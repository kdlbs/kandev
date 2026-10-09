---
id: "02-map-provisional-stream-records"
title: "Map provisional stream records"
status: done
wave: 2
depends_on:
  - "01-project-response-attempt-resets"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.28
system_design:
  - ../../specs/platform/system-design/provider-response-attempt-recovery.md
---

# Task 02: Map Provisional Stream Records

## Summary

Make lifecycle own the Kandev assistant and thinking IDs created during the
current provider response attempt. Convert a fenced reset into an ordered list
of retractable IDs while preserving committed records and conservative replay
evidence.

## In scope

- Attempt-local assistant and thinking record tracking.
- Top-level tool commit boundary.
- Current execution and prompt-generation fencing.
- Coalescer flush before reset publication.
- Protocol correlation, legacy buffer, and pending fallback-history cleanup.
- Provider-neutral `RetractedMessageIDs` stream payload.

## Out of scope

- Durable deletion or frontend delivery.
- Truncation of a record created before the current attempt.
- Relaxing prompt-level output or effect evidence.

## Acceptance

- A current reset detaches every assistant and thinking row first allocated in
  the active attempt, publishes their IDs once in allocation order, and clears
  pending abandoned history.
- Empty, repeated, zero-generation, stale-execution, and stale-generation
  resets cannot retract successor or earlier-turn records.
- A top-level tool boundary commits earlier rows; reuse of their protocol IDs
  cannot make those rows deletion-eligible again or relax replay safety.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(HandleResponseAttemptReset|ResponseAttemptRecords|ResponseAttemptReset)')
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_streaming.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_response_attempt_reset_test.go`

## Dependencies

- Task 01 supplies the normalized reset event and prompt generation.

## Risks

- Clearing the full protocol map would break valid message continuation across
  tool calls.
- Publishing before the stream coalescer drains can reorder deletion before
  creation.
- Resetting prompt-attempt evidence can accidentally authorize an unsafe prompt
  replay.

## Parallelism

`sequential`

## Inputs

- Provider Error Recovery criteria `.27` and `.28`.
- Lifecycle record ownership and commit-boundary sections in the design.
- Existing protocol message correlation and stream coalescer tests.

## Results

Lifecycle now records each newly allocated assistant and thinking message ID in
wire allocation order. A generation-fenced reset flushes the stream coalescer,
detaches only records created since the latest committed boundary, selectively
removes their protocol correlations, and clears legacy buffers plus pending
assistant fallback history. Top-level tool calls commit earlier records, and
prompt-level replay evidence remains conservative after a reset.

Verified current, empty/repeated, inactive, stale-generation, stale-execution,
legacy-stream, protocol-reuse, and top-level-tool boundary behavior with:

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(HandleResponseAttemptReset|ResponseAttemptRecords|ResponseAttemptReset)' -count=1)
```
