---
id: "01-cursor-error-category"
title: "Preserve and classify actual Cursor diagnostics"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.12
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.13
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.14
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.15
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.30
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.31
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-error-recovery-cursor.md
---

# Task 01: Preserve and classify actual Cursor diagnostics

## Summary

Remove the fabricated HTTP/2 cause from Cursor terminal diagnostics. Retain
sanitized observed text and introduce the explicit retryable resource-exhaustion
category without broadening original replay or generic data-only ACP errors.

## In scope

- TDD `TestCursorRetriableDiagnosticProjection` with a synthetic resource marker
  after two successful execute calls. The pre-fix failure must show the fixed
  HTTP/2 message where the resource message was expected.
- Store bounded sanitized diagnostic/time with the prompt-local Cursor marker;
  preserve clearing/re-arming, cancellation veto, notification barrier, single
  terminal event, and raw-diagnostic privacy constraints.
- Narrow category-specific fingerprints, `provider_resource_exhausted` semantics,
  catalogue/class/policy/invariant coverage, and runtime-usability recognition.
- Preserve actual HTTP/2 reset, unavailable, stalled, unknown, hard-quota, foreign
  provider, oversized/multibyte, stale-generation, and cancellation distinctions.

## Out of scope

Continuation eligibility, message visibility rules, new timers, inferred reset
deadlines, provider switching, UI-specific raw-string checks, and HTTP compatibility
configuration. Do not modify the reported live session or copy its transcript.

## Acceptance

- The raw resource fixture produces exactly one sanitized structured resource
  failure after settlement, with no invented transport text; provider progress
  before settlement still suppresses a superseded marker.
- Exact retryable resource exhaustion is transient and short same-provider
  retryable without account-quota or fallback claims. Unknown/foreign/cancelled
  and explicit hard failures remain under their existing policy.
- Retained runtime recovery recognizes the new transient category while replay
  still refuses output or tool activity. All new named regression tests are
  discovered and produce behavioral RED/GREEN evidence.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/routingerr -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/types/streams -run 'Test(CursorRetriable|CursorContinuationEvidence|ProviderError)' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(CursorResource|Retained.*Resource|CursorTransportLost|PromptAttemptEvidence)' -count=1)
git diff --check
```

Add `TestClassifyCursorRetriableCategory`, `TestCursorResourceTurnRetention`, and
`TestRetainedRuntimeResourceRefusal` in focused sibling files. Never accept a
zero-test selector as validation. UI display proof is owned by Task 03.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_retriable_test.go`
- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`, `runtime_rules.go`, `policy.go`, and fixtures/tests
- `apps/backend/internal/agent/runtime/lifecycle/` and `apps/backend/internal/orchestrator/` focused retained-turn tests

## Dependencies

None. Read backend and agentctl scoped guidance and both provider-error artifacts.

## Risks

Sanitization must not erase category or accidentally authorize a different cause.
Unknown marker handling must retain terminal failure rather than report success.
Do not reinterpret previously persisted HTTP/2 errors as resource exhaustion.

## Parallelism

`sequential`

## Inputs

- Provider error design: Cursor projection, classifier matrix, and replay fence.
- Existing `dialect_cursor_retriable_test.go`, catalogue and retained-turn tests.
- Plan evidence and the October 5 decision; no private live frames in fixtures.

## Results

Task completed. Fixed diagnostic projection to preserve sanitized observed Cursor diagnostics instead of hardcoding HTTP/2 stream reset. Added CodeProviderResourceExhausted category in routingerr with deterministic classification, short same-provider retry policy, and retained-runtime handling. Added TestClassifyCursorRetriableCategory, TestCursorResourceTurnRetention, TestRetainedRuntimeResourceRefusal, and TestCursorRetriableDiagnosticProjection. All regression tests pass.

PR review remediation narrows resource exhaustion to its verified complete
envelope, anchors unavailable/stalled categories, rejects the Cursor subagent
title alone, and refuses continuation after a prompt-gate handoff because late
permission/tool frames cannot prove originating ownership. Mock episodes are
consumed once. V1 positive admission/dispatch coverage remains; the lifecycle
fixture asserts a valid stored diagnostic. Reload checks wait for persisted
content, disabled-mode traces admit only the original prompt, and the seeded
resource-exhaustion test claims presentation coverage only. Specification
statuses and public foreground/uncertain-work boundaries are synchronized.
Remote CI and review disposition remain pending until the final fixup snapshot.
