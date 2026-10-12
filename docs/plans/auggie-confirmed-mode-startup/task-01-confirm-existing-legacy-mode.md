---
id: "01-confirm-existing-legacy-mode"
title: "Confirm already satisfied legacy modes"
status: done
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
  the old cached mode matches, including when its uncorrelated late report
  arrives while idle before the successor request. A correlated mode-config
  response or a fresh report after a session transition may restore certainty;
  unrelated config responses may not. Keep all existing clamp/config/late-report
  tests.

## Out of scope

Lifecycle fixtures (Task 02), provider-name/version exceptions, generic success
acknowledgment, changes to `awaitModeSettle`, settings writes, and frontend work.

## Acceptance

- Matching certain active-session legacy state returns an applied result and
  normal confirmed event with zero provider mode RPCs; fresh transition state
  can qualify, while stale state cannot.
- Unknown, unavailable, cancelled, closed, or uncertain state cannot confirm by
  shortcut. An uncorrelated late report while idle cannot clear uncertainty; a
  correlated mode-config snapshot and a replacement session's own report can.
  Genuine mode changes and config-option clamps retain existing rules.
- All existing mode/config serialization, timeout, cancellation, and session
  isolation regressions pass, including race checks.

## Verification

From repository root. Record the primary test's expected red failure before
implementation, then run these exact commands after the correction:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestSetModeAlreadySatisfiedLegacyMode.*|TestSetModeCanceledBefore(LegacyRPC|ModeConfigRPC)DoesNotMakeOutcomeUncertain)$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp -run '^(TestSetModeAlreadySatisfiedLegacyMode.*|TestSetModeCanceledBefore(LegacyRPC|ModeConfigRPC)DoesNotMakeOutcomeUncertain|TestConcurrentSetModeRequestsCannotShareAReport|TestLateTimedOutModeReportCannotConfirmNextRequest|TestLateTimedOutModeReportWhileIdleDoesNotRestoreShortcutCertainty|TestQueuedSetModeRechecksAlreadySatisfiedLegacyMode|TestCorrelatedModeConfigSnapshotClearsLegacyModeUncertainty|TestUnrelatedConfigResponseDoesNotClearModeTimeoutUncertainty|TestModeAndOtherConfigSnapshotsShareOrdering)$' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_mode.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_state.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_satisfied_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_set_test.go` (existing config-mode regressions)
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

Implemented the guarded legacy-mode shortcut after both existing gates and
capability validation. The check reads session identity, reported value, and
uncertainty under one lock; a satisfied result reserves its settings generation
at that same boundary and uses the normal mode event. Closed adapters return an
error before a provider request. Uncorrelated mode reports no longer clear a
timed-out operation's session-scoped uncertainty. A correlated mode-config
snapshot and a replacement session's fresh report remain the only recovery
evidence.

The idle late-report regression failed before the state-handling change with
`uncorrelated idle report cleared the timed-out operation's uncertainty` and
passed afterward. Already-satisfied coverage includes `default`, `ask`, repeat
selection, result/event output, and zero provider RPCs. Guards cover unknown,
stale, mismatched, uncertain, canceled, and closed state. Transition coverage
includes new, load, reset, and a missing replacement report. The queued-operation
regression verifies that the successor rechecks mode after the preceding mode
request releases its gate. The queued test waits for the successor to perform a
failed gate attempt through a signaling context, then asserts no early result
or second provider RPC before releasing the first operation. It fails as
expected under a temporary Go overlay that caches legacy-mode state before the
gate: the successor returns unconfirmed instead of using the new report. The
overlay was deleted after the run. New satisfied-mode and idle late-report
cases live in `adapter_mode_satisfied_test.go` to keep the
focused ACP test files below the backend file-length limit. The changed ACP
test files are 402 lines (`adapter_mode_satisfied_test.go`) and 671 lines
(`adapter_mode_ordering_test.go`), both below revive's 800-line limit.
`adapter_mode_set_test.go` is unchanged at 762 lines.

PR fixup also covers cancellation at the final pre-RPC check for legacy and
mode-config setters. Neither path marks the outcome uncertain unless its
mode-setting RPC is attempted; both regressions failed before the change and
passed after it.

Validation passed:

```text
go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestSetModeAlreadySatisfiedLegacyMode.*|TestSetModeCanceledBefore(LegacyRPC|ModeConfigRPC)DoesNotMakeOutcomeUncertain)$' -count=1
go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -count=1
go test -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp -run '^(TestSetModeAlreadySatisfiedLegacyMode.*|TestSetModeCanceledBefore(LegacyRPC|ModeConfigRPC)DoesNotMakeOutcomeUncertain|TestConcurrentSetModeRequestsCannotShareAReport|TestLateTimedOutModeReportCannotConfirmNextRequest|TestLateTimedOutModeReportWhileIdleDoesNotRestoreShortcutCertainty|TestQueuedSetModeRechecksAlreadySatisfiedLegacyMode|TestCorrelatedModeConfigSnapshotClearsLegacyModeUncertainty|TestUnrelatedConfigResponseDoesNotClearModeTimeoutUncertainty|TestModeAndOtherConfigSnapshotsShareOrdering)$' -count=1
go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestQueuedSetModeRechecksAlreadySatisfiedLegacyMode$' -count=20
```
