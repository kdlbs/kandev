---
id: "01-coalesce-agent-stream-ingress"
title: "Coalesce agent stream ingress"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/bounded-task-status-delivery.md"
---

# Task 01: Coalesce Agent Stream Ingress

## Acceptance

- The first non-empty assistant/reasoning chunk for a message publishes immediately;
  adjacent append chunks publish no more than once per 100 ms window.
- A 30,000-chunk protocol reasoning stream persists byte-for-byte content with
  a bounded publication/write count.
- Tool, permission, completion, error, disconnect, replacement, and shutdown
  boundaries flush pending content before the boundary event.
- Interleaved message IDs/kinds preserve wire order, repeat flushes are
  idempotent, and teardown leaks no timers or goroutines.
- Content-free counters/logs report received/coalesced/flushed chunk totals.

## TDD sequence

1. RED: add fake-time tests for immediate first publication, 100 ms append
   cadence, exact burst concatenation, typed ID isolation, and semantic order.
2. RED: cover disconnect, prompt reset, execution replacement, shutdown, and a
   timer callback racing a synchronous flush.
3. GREEN: add an execution-owned adjacent-segment coalescer, then route both
   protocol and legacy streaming publications through it.
4. GREEN: flush at every lifecycle boundary and dispose the coalescer from all
   execution teardown paths.
5. REFACTOR: isolate timer/segment mechanics, publish outside locks, and add
   content-free observability without growing lifecycle event handlers.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_streaming.go`
- lifecycle disconnect/reset/shutdown files that own execution teardown
- new `apps/backend/internal/agent/runtime/lifecycle/stream_coalescer.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go`
- new focused coalescer tests
- focused orchestrator/task-service streaming integration tests

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... \
  ./internal/orchestrator/... ./internal/task/service/...
```

## Dependencies

None. This is the earliest amplification boundary and preserves the current
downstream message event contract.

## Parallelism

Sequential. Task 02 assumes the reduced but still defensive full-message update
stream produced here.

## Inputs

- Spec section **Session stream overload isolation**.
- Incident baseline: 28,967 reasoning events, peak 63/s.
- Existing protocol-ID and semantic-boundary tests in
  `manager_events_test.go`.

## Risks

- Publishing while holding `messageMu` can deadlock callbacks; detach then
  publish.
- A boundary missed during teardown can lose a final partial chunk.
- Coalescing across interleaved IDs or a tool boundary can corrupt transcript
  order; only adjacent same-key chunks may combine.

## Output contract

Report exact input/published/write counts for the 30,000-chunk test, final
content equality, covered flush boundaries, metrics added, test commands, and
files changed.

## Verification results

- `go test -race ./internal/agent/runtime/lifecycle/...` — passed (1,059 tests
  across lifecycle and lifecycle/skill packages).
- `go test ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... ./internal/task/service/...` — passed.
- The coalescer test feeds 30,000 chunks and emits two ordered segments while
  preserving the exact concatenated content.
- Boundary coverage includes tool calls/results, completion, disconnect,
  prompt handoff/reset, execution removal, timer flush, interleaved IDs, and
  close-time stats logging.
- Added structured close-time counters for received, coalesced, and flushed
  stream segments; no transcript content is logged.

## Files changed

- `apps/backend/internal/agent/runtime/lifecycle/stream_coalescer.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_coalescer_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_streaming.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
