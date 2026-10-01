---
id: "01-confirm-existing-legacy-mode"
title: "Confirm already satisfied legacy modes"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.8
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.9
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.10
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.11
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 01: Confirm already satisfied legacy modes

## Summary

Make `SetMode` satisfy an already confirmed legacy selection without sending a
redundant provider mutation. Keep actual mutations, authoritative config-option
responses, and uncertain outcomes on their existing strict paths.

## In scope

- Add the four adapter test groups in [plan.md](plan.md#tests) using existing
  ACP pipe fixtures. First reproduce initial `default` plus silent mode RPC.
- Implement the guarded no-op after capability validation and both gates, before
  beginning mutation uncertainty. Preserve normal mode event/generation output.
- Cover repeated selections, new/load/reset, stale and missing state, closed
  adapter, cancellation, and no-op requests queued behind another operation.
- Assert that a preceding unconfirmed mutation disables the shortcut even when
  the old cached mode matches. Keep all existing clamp/config/late-report tests.

## Out of scope

Lifecycle fixtures (Task 02), provider-name/version exceptions, generic success
acknowledgment, changes to `awaitModeSettle`, settings writes, and frontend work.

## Acceptance

- Matching certain active-session legacy state returns an applied result and
  normal confirmed event with zero provider mode RPCs; fresh transition state
  can qualify, while stale state cannot.
- Unknown, unavailable, cancelled, closed, or uncertain state cannot confirm by
  shortcut. Genuine mode changes and config-option clamps retain existing rules.
- All existing mode/config serialization, timeout, cancellation, and session
  isolation regressions pass, including race checks.

## Verification

From repository root. Record the primary test's expected red failure before
implementation, then run these exact commands after the correction:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestSetModeAlreadySatisfiedLegacyMode' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp -run '^(TestSetModeAlreadySatisfiedLegacyMode.*|TestConcurrentSetModeRequestsCannotShareAReport|TestLateTimedOutModeReportCannotConfirmNextRequest|TestModeAndOtherConfigSnapshotsShareOrdering)$' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_state.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_set_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_ordering_test.go`

## Dependencies

None.

## Risks

Returning before the shared epilogue can lose the confirmed event. Checking
state before the config gate can race session replacement. Clearing uncertainty
or accepting a request value as an observation can defeat strict startup.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/permission-control-integrity.md), sections 002 and 007.
- [Design](../../specs/agents/system-design/agent-permission-control-integrity.md#already-satisfied-legacy-modes).
- `newSetModeTestAdapter`, `reportModeFromAgent`, session-only mode catalog and
  config snapshot tests; `adapter_mode_state_test.go` and ordering tests.
- Root, backend, and agentctl `AGENTS.md`; `/tdd`.

## Results

Pending.
