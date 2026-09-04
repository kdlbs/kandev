---
id: "01-project-cursor-terminal-evidence"
title: "Project Cursor terminal evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.12
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.13
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 01: Project Cursor Terminal Evidence

## Summary

Recognize Cursor's exact HTTP/2 `RetriableError` control frame on the active
prompt. Suppress it and settle one structured provider error after the prompt
notification barrier. Treat later provider progress as Cursor-owned retry
activity. Thus, a superseded marker does not become a Kandev failure.

## In scope

- Cursor dialect matcher and stable safe diagnostic.
- Turn-scoped pending evidence with generation fencing.
- Ordered notification observation and control-chunk suppression.
- Post-barrier `EventTypeError` projection and Cursor provider-error source.
- Exact, negative, stale-generation, internal-progress, and settlement tests.

## Out of scope

- Error classification or orchestrator recovery policy.
- Generic assistant-output scanning.
- Provider, model, or UI changes.

## Acceptance

- The exact captured chunk is matched only for `cursor-acp`, suppressed, and
  converted into one valid structured error after all preceding notifications.
- Embedded prose, partial signatures, zero or stale generations, and non-Cursor
  events are not matched or suppressed.
- Same-generation provider progress clears a pending marker, while a later
  matching marker can become terminal evidence again.

## Verification

```bash
cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CursorRetriable|ObserveCursorRetriable|SendPrompt.*Cursor)'
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_retriable_test.go`
- `apps/backend/internal/agentctl/types/streams/provider_error.go`

## Dependencies

None.

## Risks

- The hot observer runs for every chunk, so cheap identity and prefix checks
  must precede the full case-insensitive fingerprint.
- Notification and prompt-response races can emit complete before the error if
  the barrier is bypassed.

## Parallelism

`parallel-safe` with Task 02. The files are disjoint.

## Inputs

- Provider Error Recovery requirement criteria `.12` and `.13`.
- Cursor projection and retry-ownership sections in the system design.
- The Codex capacity evidence observer and prompt-settlement precedent.
- The embedded raw ACP evidence in the task request.

## Results

- Implemented Cursor-specific, generation-scoped ACP evidence projection and
  post-barrier structured error settlement.
- Added exact, anchored, stale-generation, adapter-scope, progress-reset, and
  re-arm tests.
- Verification passed:
  `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CursorRetriable|ObserveCursorRetriable|SendPrompt.*Cursor)'`.
