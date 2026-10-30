---
id: "02-preserve-prompt-error-runtime"
title: "Preserve runtimes after ACP prompt errors"
status: done
wave: 2
depends_on:
  - "01-sdk-error-origin"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-004
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-001.1
  - AC-PLATFORM-TURN-CONTINUITY-001.2
  - AC-PLATFORM-TURN-CONTINUITY-001.3
  - AC-PLATFORM-TURN-CONTINUITY-001.6
  - AC-PLATFORM-TURN-CONTINUITY-001.7
  - AC-PLATFORM-TURN-CONTINUITY-001.8
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-004.1
  - AC-PLATFORM-TURN-CONTINUITY-004.2
  - AC-PLATFORM-TURN-CONTINUITY-004.3
  - AC-PLATFORM-TURN-CONTINUITY-004.4
  - AC-PLATFORM-TURN-CONTINUITY-004.5
  - AC-PLATFORM-TURN-CONTINUITY-004.6
  - AC-PLATFORM-TURN-CONTINUITY-004.8
  - AC-PLATFORM-TURN-CONTINUITY-004.9
system_design:
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 02: Preserve runtimes after ACP prompt errors

## Summary

Deliver the shared path from a proven remote prompt error to a failed turn and later input on the same runtime.
Use the existing failed-turn owner and preserve actual runtime-loss handling and automatic recovery policy.

## In scope

- Replace application-error provider/classification allowlists with the SDK-origin and current-runtime checks.
- Cover Claude, Codex, OpenCode, Cursor, Gemini, and a previously unknown ACP identity through real wire fixtures.
- Cover generic internal/storage/auth/quota/model/unknown errors, wrapped errors, and empty or sensitive diagnostics.
- Keep cancellation, context expiry, actual disconnect, local errors, malformed replies, and superseded generations outside generic retention.
- Keep richer correlated Codex quota evidence without bypassing retained-turn settlement.
- Remove lifecycle's provider-name restriction and mock-agent exception for the trusted ACP attestation while preserving owner and stream checks.
- Remove `acpDialect.retainedApplicationErr`, `mockRetainedApplicationError`, and the field assignment in `dialect_mock.go`.
  Keep independent mock continuation/capacity capabilities. Mock retention gets origin only through the real SDK reply path.
- Rewrite the old generic/internal/auth/quota and unknown-provider rejection expectations for the new evidence boundary.
  `TestRetainableACPApplicationErrorRequiresClassifiedProviderCapacity` must use remote-error positives and local/no-origin negatives.
  Replace the unproven-provider negative in `TestTransientTurnFailureRequiresCurrentEligibleRuntime` with invalid attestation or actual loss.
  Replace `TestRetainableACPAgentCoversOpenCode` when removing the obsolete lifecycle name predicate.
  Do not preserve the old negative expectations by reintroducing classification or mock shortcuts.
- Suppress successful turn-finished notifications for authoritative error-terminated turns at the gateway subscriber.
  Keep genuine success/clarification delivery, existing unread accounting, and the absence of a dedicated failure-push event.
  Add `TestErrorTerminatedTurnDoesNotNotify` with retained and runtime-loss error cases.
- Reuse `AgentTurnFailed`, failed/uncertain delivery settlement, idempotent status persistence, and successor fencing.
- Cover repeat failures, actual exit before/after attestation, duplicates, storage failure, and a successor racing old callbacks.
- Verify unknown or hard errors gain no automatic retry and that capacity/continuation support remains separately gated.
- Add `TestOpenCodeLockTimeoutNativeCompatibility` using disposable HOME/XDG data and a local controlled provider.
  Hold an isolated SQLite writer transaction only after the prompt reaches the provider barrier.
  Observe the lock failure, release the transaction, then attempt a distinct follow-up without restarting OpenCode.
  Record native version, terminal event, process and conversation identity, and the follow-up outcome.

## Out of scope

- New retry loops or budgets, new continuation providers, automatic model switching, and runtime feature flags.
- Startup/restore/configuration-operation policy, non-ACP transports, or autonomous owners.
- Production UI or browser fixtures. Task 03 owns durable task attention. Task 04 owns rendered acknowledgement and public guidance.
- Live-instance locking, restarts, process termination, or database edits.

## Acceptance

1. A proven prompt error preserves the same execution, process, connection, and native conversation across providers.
   It produces one sanitized failed-turn entry and accepts a distinct later prompt after durable settlement.
2. Local failures and genuine runtime loss retain their authority. Stale callbacks cannot settle or stop a successor.
   Scope exclusions, durable delivery, cancellation, and failed persistence preserve their existing owners and fences.
3. Retention does not expand automatic recovery. Existing capacity evidence and retry policies remain intact.
   Native OpenCode evidence proves continuity when its process survives, or truthfully verifies runtime-loss fallback if it exits.

## Verification

First run `TestPromptErrorRetainsACPConnectionForFollowUp` and `TestPromptErrorRetentionUsesHostAttestation` in RED.
Run each focused regression before its minimum correction.
Run this complete block from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'PromptError|RetainableACPApplicationError|PromptFailure|OpenCodeCapacity|CodexCapacity|Retained' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/instance ./internal/agentctl/types/streams ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'PromptError|PromptFailure|Retained|TurnFailure|TransientTurnFailure|CapacityContinuation' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'GenericRetainedPromptError|AgentTurnFailed|Retained|OpenCodeCapacity|Capacity|Transient' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/backendapp ./internal/notifications/service -run 'ErrorTerminatedTurn|TurnFinished|Clarification|AbandonedTurn' -count=1)
(cd apps/backend && KANDEV_TEST_OPENCODE_NATIVE=1 go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestOpenCodeLockTimeoutNativeCompatibility$' -count=1 -timeout 5m)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_failure.go apps/backend/internal/agent/runtime/lifecycle/manager_events.go apps/backend/internal/orchestrator/event_handlers_turn_failure.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The opted-in native command requires an installed `opencode` executable on PATH.
Use the existing native fixture pattern. Keep all database, provider, process, and child resources isolated and bounded.
If native prerequisites are unavailable, record the exact blocker and leave native validation pending.
A mock success is not a substitute for native evidence. A native runtime exit is a compatibility result, not a retained-session success.
Include every changed Go test in the selected package checks and changed-file lint before marking done.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_failure.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`, if safe diagnostic propagation needs adjustment.
- Existing `adapter_prompt_failure_test.go`, `adapter_prompt_capacity_continuity_test.go`, and `adapter_opencode_capacity_test.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`, `dialect_mock.go`, and affected mock dialect tests.
- `apps/backend/internal/agent/runtime/lifecycle/capacity_continuation_test.go` for obsolete provider-predicate coverage.
- `apps/backend/internal/backendapp/gateway.go` and new `gateway_turn_notifications_test.go`.
- `apps/backend/internal/notifications/service/service_test.go` for unchanged successful and clarification delivery.
- `apps/backend/internal/task/service/service_turns.go` and adjacent tests if terminal metadata propagation needs correction.
- New `adapter_prompt_error_continuity_test.go` and `adapter_opencode_lock_native_test.go` beside those tests.
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go` and `manager_turn_failure_test.go`.
- `apps/backend/internal/orchestrator/event_handlers_turn_failure.go`, its existing tests, and new `event_handlers_prompt_error_test.go`.
- Existing process/instance retained-outcome and watcher tests if propagation needs changes.

## Dependencies

Task 01 and its recorded immutable SDK revision.

## Risks

- Some native agents can terminate their own process after an error. An open SDK connection at one instant is not future liveness.
- Rich diagnostic wrappers can accidentally bypass the common retained path or expose raw Data.
- Preserving arbitrary provider identities must not grant capacity continuation or affect another transport.
- Native lock injection needs explicit provider barriers and an owned subprocess tree.

## Parallelism

`sequential`

## Inputs

- [Shared design](../../specs/platform/system-design/acp-prompt-error-continuity.md)
- [Existing settlement design](../../specs/platform/system-design/transient-turn-runtime-continuity.md)
- Existing `manager_turn_failure_test.go` and `event_handlers_turn_failure_test.go` barrier and owner fixtures.
- Existing `adapter_opencode_capacity_native_test.go` isolated process/provider pattern.

## Results

Done. Prompt errors are retained from SDK response provenance instead of provider
or transient allowlists. The lifecycle name allowlist and the mock retention
dialect hook are gone. Codex quota evidence now settles through the common
retained path, and error-terminated turns no longer send successful turn
notifications.

Verification completed:

- `go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp`: passed.
- `go test -tags fts5 ./internal/agent/runtime/lifecycle`: passed.
- `go test -tags fts5 ./internal/orchestrator`: passed.
- `go test -tags fts5 ./internal/backendapp`: passed.
- `go test -tags fts5 ./internal/orchestrator/handlers`: passed.
- Native OpenCode 2.2.0 lock reproduction:
  `KANDEV_TEST_OPENCODE_NATIVE=1 go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestOpenCodeLockTimeoutNativeCompatibility$' -count=1 -timeout 7m`
  passed in 14.03s. Holding `XDG_DATA_HOME/opencode/opencode.db` with
  `BEGIN IMMEDIATE` after the provider barrier and releasing it after OpenCode's
  own writer timeout produced `Internal error: Failed to execute statement` with
  `retain_runtime`. A distinct follow-up completed on the same initialized
  native session (for example `ses_e75a1595effe3MllVH7Oli8QxG`; ids vary per
  run), on the same process. OpenCode
  retried the provider call once during the lock window; the follow-up was the
  next provider request.
- Codex quota retention now also requires SDK remote-response provenance. A
  correlated generic remote error still preserves the quota guidance and
  retains; quota evidence followed by a local deadline stays on the conservative
  `providerPromptError` path and does not retain (`TestCodexQuotaEvidenceWithLocalDeadlineDoesNotRetain`).
- New regressions: `TestPromptErrorRetainsACPConnectionForFollowUp` (provider and
  error matrix), `TestPromptErrorRetainsAfterRepeatedFailures`,
  `TestPromptErrorPreservesCodexQuotaDiagnostic`,
  `TestRetainableACPApplicationErrorRequiresClassifiedProviderCapacity`
  (retargeted to remote-origin positives and local negatives),
  `TestOpenCodeCapacityRetentionUsesResponseOrigin`,
  `TestReplayFixtureRetainsGenericMockPromptError`,
  `TestPromptErrorRetentionUsesHostAttestation`,
  `TestErrorTerminatedTurnDoesNotNotify`, `TestAgentTurnFailedGenericPromptError`,
  and `TestGenericRetainedPromptErrorDoesNotRetry`.

Legacy rejection tests were retargeted to remote-origin positives and local or
evidence-loss negatives rather than preserving the provider restrictions.
