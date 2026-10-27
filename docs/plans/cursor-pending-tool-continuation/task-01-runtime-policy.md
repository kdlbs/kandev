---
id: "01-runtime-policy"
title: "Admit pending tools on retained Cursor runtimes"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.8
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.9
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 01: Admit pending tools on retained Cursor runtimes

## Summary

Introduce V3 evidence and admit valid pending foreground tools after retained
Cursor transport loss. Keep ownership, permissions, and original-replay guards.

## In scope

Implement the design's V3 enum and explicit `permissions_resolved` attestation.
Update Cursor dialect, snapshot producer, permission handling, JSON propagation,
continuation admission/refusal predicate, and retained episode identity.
Audit every `ContinuationSafety.SafeFor` caller, including
`provider_interruption_failure.go`, so broader eligibility does not misreport
exhaustion or bypass retained-runtime checks. Preserve
pending state in snapshots and exclude genuine failed/cancelled or malformed calls.
Ensure a pending episode cannot invoke restore/replace after runtime loss.
Audit lifecycle/watch events for copy/serialization rather than assuming the new
field survives each boundary. Test old helper and backend compatibility.

## Out of scope

UI copy, public documentation, native tool reconciliation, capacity policy,
and real-provider failure injection. No native subscription is required for
these deterministic regressions.

## Acceptance

1. Mixed completed/pending V3 transport failures dispatch exact `continue` on
   the retained execution, within the existing five-attempt episode; original
   prompt and tool calls are never resent by Kandev.
2. Permission, invalid-evidence, user priority, Stop, stale-identity, and
   background exclusions remain enforced. V1/V2 pending snapshots still refuse.
3. Runtime loss at admission, timer fire, preparation, or final dispatch refuses
   the pending episode without launch/load/resume or a replacement conversation.

## Verification

From the repository root. Write the named RED tests from the plan first.

```bash
(cd apps/backend && go test ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Continuation|Retriable|PromptFailure)' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(Interruption|Transient|RetainedTurn|Continuation)' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Test.*(Continuation|Retained|PromptFailure)' -count=1)
```

Run `bash scripts/lint-go-changed` with the explicit changed Go paths after
implementation. Record those paths and the actual command in Results.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/continuation.go` and its tests
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation_permissions.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_continuation_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_retriable_test.go`
- `apps/backend/internal/orchestrator/provider_interruption_continuation.go`
- `apps/backend/internal/orchestrator/provider_interruption_continuation_test.go`
- `apps/backend/internal/orchestrator/provider_interruption_refusal_test.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go` and retry preparation helpers
- `apps/backend/internal/orchestrator/event_handlers_turn_failure_test.go`
- Lifecycle terminal-snapshot and watcher propagation tests where required

## Dependencies

None. Reconcile the current base and adjacent PR changes before implementation.

## Risks

Late ACP updates can lack generation identity. Keep ambiguous handoffs ineligible;
do not attribute old tool completion to the current turn. Boolean omission must
not turn unresolved permission state into permission approval.

## Parallelism

`sequential`

## Inputs

- Owning requirements `001` and `002`, especially new criteria `001.8` and `001.9`.
- System design: Agent-managed pending-tool policy; Episode ownership and settlement.
- ADR 2026-10-26 and linked probe report; existing mixed-tool refusal tests.

## Results

Completed. Cursor now advertises V3 continuation evidence with explicit
permission-resolution attestation. Pending foreground tools are admitted only
for retained Cursor transport loss, and the continuation stays bound to the
original live execution. Runtime loss refuses without restore, replacement, or
original-request replay. V1/V2 pending evidence remains ineligible.

Passed backend tests:

- `(cd apps/backend && go test ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Continuation|Retriable|PromptFailure)' -count=1)`
- `(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(Interruption|Transient|RetainedTurn|Continuation)' -count=1)`
- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Test.*(Continuation|Retained|PromptFailure)' -count=1)`
- `(cd apps/backend && go test ./cmd/mock-agent -run 'Test.*Continuation' -count=1)`

Changed-Go lint passed with:

```bash
mapfile -t gofiles < <(git diff --name-only -- '*.go')
bash scripts/lint-go-changed "${gofiles[@]}"
```

The command covered these 26 paths:

- `apps/backend/cmd/mock-agent/interruption_continuation.go`
- `apps/backend/cmd/mock-agent/interruption_continuation_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/continuation_safety_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation_permissions.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation_permissions_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_continuation_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_mock.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_mock_continuation_test.go`
- `apps/backend/internal/agentctl/types/streams/continuation.go`
- `apps/backend/internal/agentctl/types/streams/continuation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_retained_runtime.go`
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go`
- `apps/backend/internal/orchestrator/event_handlers_turn_failure_test.go`
- `apps/backend/internal/orchestrator/provider_interruption_continuation.go`
- `apps/backend/internal/orchestrator/provider_interruption_continuation_test.go`
- `apps/backend/internal/orchestrator/provider_interruption_dispatch.go`
- `apps/backend/internal/orchestrator/provider_interruption_failure.go`
- `apps/backend/internal/orchestrator/provider_interruption_refusal_test.go`
- `apps/backend/internal/orchestrator/provider_interruption_retained_runtime_test.go`
- `apps/backend/internal/orchestrator/watcher/continuation_safety_test.go`

## Review follow-up results (2026-10-26)

Pending-tool continuation now captures and rebinds the original initiating
identity at provider admission. A missing identity refuses when scoped session
authorization is configured. Added regressions cover revocation before dispatch,
revocation at the final admission check, missing identity, and a second accepted
retry. The checker models production's unscoped-call behavior so a lost identity
would otherwise permit dispatch.

Passed:

- `(cd apps/backend && go test ./internal/orchestrator -run 'TestAgentManagedPendingContinuation' -count=1)`.
- `(cd apps/backend && go test ./internal/orchestrator -run 'Test.*(Interruption|Transient|RetainedTurn|Continuation)' -count=1)`.
- `make -C apps/backend build`.
- Changed-Go lint passed with `bash scripts/lint-go-changed` over the 26 paths
  listed above and the new
  `apps/backend/internal/orchestrator/provider_interruption_pending_auth_test.go`.

The unfiltered `go test ./internal/orchestrator` run reached its 10-minute test
timeout in `TestSchedulingOrphanPreparedDestinationKeepsAcceptedTransition`.
That test passes in isolation with a 45-second test timeout:
`(cd apps/backend && go test ./internal/orchestrator -run '^TestSchedulingOrphanPreparedDestinationKeepsAcceptedTransition$' -count=1 -timeout=45s)`.
