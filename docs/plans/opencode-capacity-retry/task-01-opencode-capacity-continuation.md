---
id: "01-opencode-capacity-continuation"
title: "Admit OpenCode capacity continuation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.5
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11
  - AC-PLATFORM-TURN-CONTINUITY-001.1
  - AC-PLATFORM-TURN-CONTINUITY-001.2
  - AC-PLATFORM-TURN-CONTINUITY-001.3
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.2
  - AC-PLATFORM-TURN-CONTINUITY-003.1
  - AC-PLATFORM-TURN-CONTINUITY-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.3
  - AC-PLATFORM-TURN-CONTINUITY-003.4
  - AC-PLATFORM-TURN-CONTINUITY-003.6
  - AC-PLATFORM-TURN-CONTINUITY-003.7
  - AC-PLATFORM-TURN-CONTINUITY-003.8
  - AC-PLATFORM-TURN-CONTINUITY-003.9
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 01: Admit OpenCode capacity continuation

## Summary

Connect the exact OpenCode capacity diagnostic to retained-runtime recovery.
Use the existing scheduler and continuation instruction after completed effects.
Preserve provider, generation, tool-safety, and lifecycle boundaries.

## In scope

- Add classifier and reset-hint tests first, using the exact sanitized incident text.
  Preserve structured precedence, prose negatives, unrelated provider negatives, and account-limit rules.
- Add the OpenCode dialect and versioned live-capacity capability.
  Extend application-error retention only for the tested OpenCode capacity shape.
- Reuse the ordered ledger and existing typed propagation.
  Cover completed read, execute, edit, and MCP outcomes plus mixed completed/unsafe collections.
- Reject stale generations, missing evidence, permissions, background/child work, invalid support values, queue-drain failure, and closed transport.
- Verify the host remains idle during native retry metadata and starts recovery only after a settled terminal result.
- Admit pre-result replay or completed-work continuation through the current owner.
  Preserve episode identity, selected model, permissions, workspace, native identity, and actual dispatch accounting.
- Apply a validated short reset hint as a floor for the first five capacity delays.
  Test 21 seconds against the 5-second slot and 9 seconds against the 40-second slot.
  Retain later delays, the thirty-minute deadline, and non-capacity timing.
- Capture native compatibility on a disposable OpenCode process with a local controlled provider.
  Configure temporary state and return capacity errors locally, then accept a continuation.
  Record the installed CLI version and sanitized method/timing evidence.
  Never use live credentials, the reported conversation, or an external model service for this test.

## Out of scope

UI markup, locale changes, production runtime configuration, new schedulers,
new error categories, broad provider retention, and transport-loss restoration support.

## Acceptance

1. The observed diagnostic reaches high-confidence capacity classification and current-generation retained failure evidence through the real ACP prompt-error path.
2. Eligible recovery continues on the same native runtime with bounded backoff and no replay of completed work; unsafe or superseded attempts never dispatch.
3. Protocol/service tests and native compatibility prove the contract. Existing Codex, Cursor, remote omission, and non-capacity policies remain covered.

## Verification

Run from the repository root. Use `/tdd`; record the expected Red failure before production changes.
Create the assigned tests before these commands. A missing test match is not passing evidence.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/routingerr -run 'OpenCode|Signature|Classify|ResetHint' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'OpenCodeCapacity|Capacity|Continuation|PromptFailure|Retained|TurnFailure' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'OpenCodeCapacity|Capacity|Transient|AgentTurnFailed|Retained|Continuation' -count=1)
(cd apps/backend && KANDEV_TEST_OPENCODE_NATIVE=1 go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestOpenCodeCapacityNativeCompatibility$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The native command is assigned test infrastructure, not an existing option.
Its opt-in test resolves the installed OpenCode runtime and fails when the binary or controlled provider is unavailable.
It must not report a skip as native compatibility success.
Before production support is enabled, record terminal settlement and another prompt on the same initialized native session.
Run `bash scripts/lint-go-changed` with every changed Go file as a separate argument.
Run the [documentation coverage preflight](plan.md#documentation-coverage-preflight).

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/{rules.go,routingerr.go,resethint.go,opencode_capacity_test.go}`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/{dialect.go,dialect_opencode.go,adapter_prompt_failure.go}`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/{adapter_opencode_capacity_test.go,adapter_opencode_capacity_native_test.go}`.
- `apps/backend/internal/agentctl/types/streams/{capacity_continuation.go,capacity_continuation_test.go}`.
- `apps/backend/internal/orchestrator/{provider_interruption_continuation.go,provider_capacity_retry_policy.go,event_handlers_transient_opencode_test.go}`.
- `apps/backend/internal/orchestrator/provider_interruption_opencode_capacity_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/capacity_continuation_test.go`.
- `apps/backend/internal/orchestrator/watcher/` and process tests only where propagation evidence needs extension.
- Paired requirement and design amendments linked above.

## Dependencies

None. Read scoped backend and agentctl guidance before implementation.

## Risks

- Matching capacity prose can misclassify an unrelated assistant message.
- A null native retry update does not prove that the prompt settled.
- OpenCode child/background activity must not become completed evidence through generic normalization.
- Native runtime availability can block the required compatibility evidence. Report the missing evidence without enabling unproven support.
- A continuation that fails without another tool must retain its original deadline and completed-work binding.

## Parallelism

`sequential`

## Inputs

- [Plan evidence and compatibility matrix](plan.md).
- Agents design: OpenCode admission refusal amendment.
- Platform design: OpenCode capacity support, capacity admission, and episode timing.
- Existing `adapter_prompt_capacity_continuity_test.go`, `adapter_capacity_continuation_test.go`,
  `provider_interruption_capacity_test.go`, and `provider_interruption_capacity_delayed_test.go`.
- Accepted runtime-lifetime and capacity-continuation ADRs linked by the Platform design.

## Results

Delivered and locally verified.

- `routingerr` adds `opencode.capacity.admission_refused.v1` for the exact opening
  upstream diagnostic and derives a bounded `(after N s)` reset hint from 1
  through 60 seconds. Structured status and earlier account-limit rules keep
  precedence. `TestOpenCodeAdmissionCapacityClassification`,
  `TestOpenCodeAdmissionCapacityResetHint`, the collision negatives, and the
  unrelated-provider negatives pass.
- `streams.CapacityContinuationOpenCodeLiveSessionV1` (`opencode_live_session_v1`)
  is accepted by `CapacityContinuationSnapshot.SafeFor` and the orchestrator
  support check. Unknown values stay ineligible.
- `newOpenCodeACPDialect` selects that capability for `opencode-acp`.
  `Adapter.retainableACPApplicationError` accepts only the bounded OpenCode
  admission rule, so a broad capacity phrase, auth, quota, a generic internal
  error, or an unrelated refusal token keeps the existing path.
- The capacity snapshot uses the existing ordered ledger. Completed read,
  execute, edit, and MCP outcomes are safe; pending, failed, unknown, duplicate,
  permission, and subagent cases are not. Native `opencode/retry` metadata
  produces no host dispatch and does not consume completed evidence.
- The lifecycle retention allowlist now includes `opencode-acp`.
  `TestCapacityContinuationRetainsOpenCodeRuntime` proves the retained turn
  publishes `agent.turn_failed` with the OpenCode snapshot and leaves the
  execution ready.
- The capacity policy applies a valid short hint as a floor for the first five
  slots only: 21 seconds against the 5-second slot, 9 seconds against the
  40-second slot, and unchanged later slots, deadline, and non-capacity timing.
  `TestOpenCodeCapacityContinuationUsesSameRuntime`,
  `TestOpenCodeCapacityContinuationSupersession`,
  `TestOpenCodeCapacityContinuationEvidence`,
  `TestOpenCodeCapacityRetryHintFloor`, and
  `TestOpenCodeCapacityRetryHintFloorNotice` pass.
- Native compatibility passed against `opencode v2.2.0` with a disposable local
  provider: 12 provider requests, terminal settlement after 82.7 seconds, and a
  continuation prompt after 293.8 milliseconds on the same process, connection,
  and native session. `KANDEV_TEST_OPENCODE_NATIVE=1` gates the test; once
  opted in it fails rather than skips when the binary or provider is missing.
  The remediation run passed again (85.1 seconds).
- Commands recorded in the plan's verification results. Changed-Go lint reports
  zero issues.

One implementation detail extended the plan's file list: the lifecycle
retainable-provider allowlist in `manager_events.go` had to admit `opencode-acp`,
and the existing unsupported-provider negative now uses an unproven identity.
No other unplanned production path changed.

Review remediation:

- The capacity ledger now records a detached launch from the execute call's
  `background` input and keeps it unresolved until terminal workload evidence
  arrives for the same native shell. This is independent of the background-work
  inspection feature, so a completed launch tool can no longer pass `SafeFor`
  while its job is still running. Missing, malformed, or contradictory launch
  metadata keeps the snapshot unsafe. `TestOpenCodeCapacityDetachedLaunchBlocksContinuation`
  covers inspection off and on with the captured launch shape;
  `TestOpenCodeCapacityDetachedLaunchFailsClosed` covers missing and
  contradictory evidence, including an exit code with no observed launch
  identity and conflicting identity spellings with or without a status;
  `TestOpenCodeCapacityOrdinaryForegroundExecuteStaysSafe` is the positive
  control. `TestOpenCodeCapacityUnresolvedBackgroundDoesNotDispatch` proves the
  service creates no retry owner and dispatches nothing.
- The relative reset suffix is read only from the line that matched the
  admission-refusal signature. Unrelated stderr/stdout lines and a malformed
  diagnostic hint no longer supply a delay. `TestOpenCodeAdmissionCapacityHintIgnoresUnrelatedLines`
  covers both channels, prefix and suffix noise, a malformed diagnostic hint
  plus an unrelated valid suffix, and a valid hint with surrounding output.
