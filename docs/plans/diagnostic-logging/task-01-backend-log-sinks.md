---
id: "01-backend-log-sinks"
title: "Backend log sinks"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 01: Backend log sinks

## Acceptance

- Backend owns an owner-only `<ResolvedHomeDir>/logs/backend-logs.log`, appends
  across same-UTC-day restarts, and rolls it at UTC midnight or the first
  startup after a day boundary.
- Completed files use `backend-logs-YYYY-MM-DD.log`; cleanup preserves only the
  current UTC day and two preceding days while leaving unrelated neighbors.
- Normal, debug, and verbose file/stdout thresholds match the spec. The backend
  constructor does not add a new dependency on the legacy in-memory buffer.
- Log producers never wait for file or stdout I/O. Independent file/stdout
  queues enforce the exact entry/byte/reserved capacities and 256 KiB entry cap
  from the spec, shed debug/info first, and expose per-sink loss counters.
- A read-only atomic sink-statistics snapshot is available to the later bundle
  manifest without retaining log entries.
- Close drains for at most two seconds; terminal DPanic/Panic/Fatal behavior
  uses the bounded direct-stderr fallback from the spec.
- File-open and cleanup failures do not prevent startup. The backend reports
  fallback and retries file activation every 30 seconds.
- Destination and lumberjack rotation settings no longer influence backend
  logging; the daily writer exposes the strict active/dated file set consumed
  by the later bundle service.

## Verification

```bash
cd apps/backend
go test ./internal/common/logger ./internal/common/config ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/common/logger/logger.go`
- `apps/backend/internal/common/logger/async_sink.go`
- `apps/backend/internal/common/logger/daily_writer.go`
- `apps/backend/internal/common/logger/logger_test.go`
- `apps/backend/internal/common/logger/daily_writer_test.go`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/config_test.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

None.

## Parallelism

Sequential. This establishes the shared logging contract consumed by every
later task.

## Inputs

- Spec: What, Persistence guarantees, and the first five Scenarios.
- Plan: Backend / Fixed diagnostic file and dual sinks.
- ADR: fixed path, three-day UTC history, removed destination/rotation config.
- Existing patterns: `logger.NewLogger` and file-sink lifecycle.

## Risks

- Cleanup must never use a broad glob or delete unrelated log-directory files.
- The daily writer needs an explicit persisted day marker or equally robust
  mechanism; an externally changed file mtime cannot silently choose its date.
- A rollover transaction journal must resume every tested crash point exactly
  once and must never overwrite an existing dated destination.
- Midnight rollover must serialize with writes and retry without losing the
  entry that observed the day boundary.
- Slow stdout/files must not cause request, agent, or workflow goroutines to
  wait on logging. The implementation must make its finite queue and recovery
  accounting testable with a blocked writer.
- A blocked stdout queue must not delay or shed entries accepted by a writable
  file queue, and vice versa.
- `Logger.WithFields` and `Close` must retain every backend sink.
- File creation and cleanup must behave consistently on Windows and Unix.

## Output contract

Report the threshold matrix, UTC rollover/retention behavior, cleanup matching
rule, files changed, exact test command/result, blockers or risks, and update
this task plus `plan.md` status in the same conversation.
