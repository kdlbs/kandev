---
id: "01-project-response-attempt-resets"
title: "Project response-attempt resets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.26
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-response-attempt-recovery.md
---

# Task 01: Project Response-Attempt Resets

## Summary

Recognize Codex's exact structured response-stream retry metadata on the active
prompt and emit one provider-neutral reset boundary. Preserve ordinary session
information and FIFO ordering without exposing provider diagnostics downstream.

## In scope

- Provider-neutral stream event type.
- Optional ACP dialect reset recognizer.
- Exact Codex structured metadata matcher.
- Active prompt-generation correlation.
- Ordered reset projection and ordinary session-info preservation.
- Positive, malformed, stale, zero-generation, and unsupported-dialect tests.

## Out of scope

- Lifecycle record mapping or task-message deletion.
- Terminal error classification and Kandev-owned prompt retry.
- Raw error-string matching.

## Acceptance

- The captured `willRetry=true` and `responseStreamDisconnected` shape emits
  exactly one generation-bearing reset before replacement chunks.
- A false flag, missing or malformed nested object, unsupported dialect, and
  zero or stale prompt generation emit no reset and remove no ordinary event.
- The normalized reset carries no raw provider diagnostic or provider-specific
  recovery policy.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CodexResponseAttemptReset|ObserveResponseAttemptReset|HandleACPUpdate.*ResponseAttemptReset)')
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex_retry_test.go`

## Dependencies

None.

## Risks

- Matching a broad error shape would let unrelated session metadata erase
  valid output.
- Emitting outside the notification worker would lose ordering with partial and
  replacement chunks.

## Parallelism

`sequential`

## Inputs

- Provider Error Recovery criterion `.16`.
- Adapter evidence projection in the response-attempt recovery design.
- Existing Codex capacity and Cursor retry evidence observers.
- Sanitized ACP metadata captured from the reported task.

## Results

Implemented a provider-neutral `response_attempt_reset` stream event, an
optional ACP dialect recognizer, and an exact Codex matcher for the structured
`willRetry=true` plus `responseStreamDisconnected` metadata. The ordered ACP
worker emits the generation-bearing reset before the preserved session-info
event and later replacement chunks. Unsupported dialects, malformed metadata,
and zero or stale prompt generations remain inert.

Verified with:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(CodexResponseAttemptReset|ObserveResponseAttemptReset|HandleACPUpdate.*ResponseAttemptReset)' -count=1)
```
