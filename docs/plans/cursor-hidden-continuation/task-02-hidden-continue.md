---
id: "02-hidden-continue"
title: "Dispatch hidden continuation after completed tools"
status: complete
wave: 2
depends_on:
  - "01-cursor-error-category"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.7
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 02: Dispatch hidden continuation after completed tools

## Summary

Admit a new internal `continue` turn after supported transient provider errors
and successful completed foreground tools. Reuse the existing dispatch-only
continuation, native identity, retained runtime, episode, and cancellation paths.

## In scope

- TDD V2 ordered completion ledger and immutable wire snapshot, preserving V1
  read-only semantics and fail-closed old/unknown helper behavior. Include
  successful shell/write/MCP outcomes, resolved allowed permissions, mixed
  pending outcomes, invalid transitions, bounds, and pre-sweep snapshot ordering.
- Generalize continuation cause admission to the shared short-retry predicate
  including Task 01's resource category. Keep the original replay evidence fence.
- Set the literal payload to `continue`, without task-specific augmentation,
  original prompt, attachments, prompt caching, or user-message/history insertion.
- Keep one five-attempt episode, strict same-native restore, usable runtime reuse,
  human queue priority, supersession, exact-generation fencing, and settlement.
- Update mock recognition to literal `continue` only when its native session has
  an owned recovery fixture episode. Ordinary user `continue` stays ordinary input.

## Out of scope

New provider implementations, default-on rollout, public UI controls, hiding
messages by text, retrying ambiguous prompt acceptance, any fresh-session fallback,
and original-prompt replay after completed work.

## Acceptance

- V2 output plus all-successful foreground shell/write/MCP work receives exactly
  `continue` in the same native session. Mixed/uncertain outcomes and unknown
  support dispatch nothing. V1 remains limited and original replay stays fenced.
- Internal recovery creates an accepted turn with observable output and status,
  but no user-message or prompt-index row and no augmentation. An explicit human
  `continue` remains visible. Actual transport payload and repository history
  assertions prove this independently of the mock's success text.
- Repeated accepted failures and pre-dispatch restoration failures share the
  existing five-attempt backoff. Cancellation, new human work, archive/delete,
  config changes, backend restart, and ambiguous acceptance cannot duplicate work,
  reset the budget, or stop/settle a successor. Disabled mode derives no V2 evidence.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/types/streams ./internal/orchestrator/watcher -run 'Test(CursorContinuationEvidence|ContinuationSafetySnapshot|ContinuationConfig)' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'Test(InterruptionContinuation|PromptAttemptEvidence|HandleTransientFailure.*Replay)' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Test(ContinuationNativeOnlyRestore|Retained.*Continuation)' -count=1)
(cd apps/backend && go test -tags fts5 -race ./cmd/mock-agent -run 'Test.*Continuation' -count=1)
git diff --check
```

Add `TestInterruptionContinuationCompletedTools` and
`TestInterruptionContinuationHiddenContinue` beside existing admission/dispatch
tests. Run each behavioral regression RED before changing production behavior.
Native readiness and complete desktop/phone proof are required by Task 03.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/continuation.go` and snapshot tests
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go` and mock dialect
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_continuation_test.go`
- Prompt-local state in `adapter.go` and existing permission/background evidence hooks
- `apps/backend/internal/agent/runtime/lifecycle/` and `internal/orchestrator/watcher/` snapshot propagation/tests
- `apps/backend/internal/orchestrator/provider_interruption_continuation.go`
- `apps/backend/internal/orchestrator/provider_interruption_dispatch.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`, `provider_interruption_*_test.go`
- Existing `task_operations.go` internal continuation integration only where a test proves a gap
- `apps/backend/cmd/mock-agent/interruption_continuation.go` and its tests

## Dependencies

Task 01 complete. Read the completed-tool ADR, amended continuation artifacts,
runtime continuity requirements, and existing native compatibility evidence.

## Risks

Completion permits a new turn, not repeatability of a prior action. No instruction
can guarantee model idempotency. Never loosen V1 or fabricate provider completion
from prompt-end sweeps. Track unresolved permissions separately from generic event
serialization and explicit cancellation intent.

## Parallelism

`sequential`

## Inputs

- Design sections: Compatibility and evidence; Admission; Internal dispatch;
  Episode ownership and settlement; Rollout.
- Existing continuation ownership, budget, managed-input, retained-runtime, and
  restart tests; Tasks prompt-index/message projection as visibility evidence.

## Results

Task completed. Introduced native_saved_history_completed_tools_v2 support with bounded completion ledger admitting all successfully completed foreground tools (reads, writes, execute, MCP). Set internal continuation instruction to exact text "continue". Generalized continuation admission to all short-retryable transient provider errors. Added TestInterruptionContinuationCompletedTools and TestInterruptionContinuationHiddenContinue. Review remediation rejects Cursor subagent/background raw payloads and mixed
unknown/pending outcomes. Permission ownership follows the original turn;
approved tools become eligible only after completion, regardless of notification
ordering, while denied, cancelled, or unresolved requests remain ineligible.
Focused race tests pass.
