---
id: "01-retain-failed-turn-runtime"
title: "Retain failed-turn runtimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-001.1
  - AC-PLATFORM-TURN-CONTINUITY-001.2
  - AC-PLATFORM-TURN-CONTINUITY-001.3
  - AC-PLATFORM-TURN-CONTINUITY-001.6
  - AC-PLATFORM-TURN-CONTINUITY-001.7
  - AC-PLATFORM-TURN-CONTINUITY-001.8
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.4
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 01: Retain failed-turn runtimes

## Summary

Fail the observed provider turn while preserving its positively validated ACP runtime.
Deliver the complete manual-follow-up path, including wire evidence, runtime settlement, durable turn ownership, and caller deduplication.
This work order must pass independently before automatic recovery reuse is added.

## In scope

- Add optional, validated `PromptFailureDisposition` to agent events, JSON, completion snapshots, and retained outcomes.
- Attest retention after a tested transient error settles, the notification queue drains, and the current ACP connection remains open.
- Cover Codex capacity and tested application-level overload/rate replies, plus Cursor's settled transient stream diagnostic.
- Reject closed SDK transport, uncertain settlement, uninitialized sessions, stale generations, generic internal errors, and unknown/Data-only shapes.
- Preserve process, execution binding, native conversation, client, workspace, settings, span, and execution slot.
- Introduce and wire `AgentTurnFailed`; keep runtime failure separate from normal successful `AgentReady`.
- Preserve foreground admission until durable settlement; use exact turn/generation identity for late activity and completion fences.
- Route blocking callers through a typed owned failure without a second `handlePromptError` settlement.
- Settle failed/uncertain managed input once, then persist one sanitized, nonblocking turn-error record in existing message storage.
- Preserve existing review reconciliation, error actions, Auto-run queue ownership, and true terminal cleanup.
- Restrict retention to supported concrete-profile interactive tasks; negative tests retain existing unsupported-context behavior.
- Record bounded outcome diagnostics without raw provider payloads or synthetic exit codes.

## Out of scope

- Reusing runtimes inside automatic retry or continuation dispatch; work order 02 owns that integration.
- Historical UI copy correction, new localized copy, mock browser fixtures, and public docs; work order 03 owns those outcomes.
- New schedulers, providers, flags, schema migrations, runtime adoption, or relaxed replay safety.

## Acceptance

1. A capacity failure after output or tool activity leaves the exact runtime available for a subsequent manual prompt and supported model change.
   It creates one failed-turn record and no blocking startup recovery state.
2. Authoritative process loss, unsupported context, invalid evidence, or a stale callback cannot retain or resurrect an execution.
   Barrier-controlled tests prove exit precedence, single settlement, and no successful workflow advancement.
3. Wire/outcome round trips and blocking callers preserve failure ownership.
   A storage failure withholds successor admission without falsely claiming readiness or tearing down ACP for the provider error alone.

## Verification

Start with failing tests for the reported capacity shape and settlement race.
Add the planned test names from the manifest; run each focused regression before changing its implementation.
Run this complete block from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/instance -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher ./internal/orchestrator -run 'TurnFailure|TransientTurnFailure|CapacityAfterTools|PromptFailureDisposition|SessionRecovery|Completion' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use protocol fixtures to prove the same connection receives the next prompt without initialize/new/load/resume.
Round-trip valid, omitted, and invalid disposition values through process and retained-outcome paths.
Exercise writes and unknown tool results: retain the runtime while refusing unsafe automatic work.
Scope tests must include dynamic, Office, utility, passthrough, and automation-owned executions.
Hold durable settlement behind a barrier and attempt a successor prompt before releasing it.
Test real exit both before and after attestation, and duplicate/stale delivery around a later generation.
Browser evidence is owned by work order 03, rather than artificially duplicated here.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`, adjacent disposition type and tests, and `types/types.go` aliases if required.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`, `adapter_continuation.go`, and dialect evidence helpers.
- New ACP `adapter_prompt_capacity_continuity_test.go` and application-error fixture tests.
- `apps/backend/internal/agentctl/server/process/manager.go`, `blocking_send.go`, `turn_outcome.go`, and adjacent tests.
- `apps/backend/internal/agentctl/server/instance/turn_outcome.go`, `manager_turn_outcome.go`, and existing outcome/wiring tests.
- Agentctl runtime client event/outcome decoding if it has independent wire types.
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`, `manager_interaction.go`, `event_types.go`, `events.go`, `session.go`, and new `manager_turn_failure_test.go`.
- `apps/backend/internal/events/types.go` for the internal turn-failure event.
- `apps/backend/internal/orchestrator/watcher/watcher.go` and new `turn_failure_test.go`.
- `apps/backend/internal/orchestrator/event_handlers_agent.go`, `task_operations.go`, and new turn-failure handler/tests.
- Existing execution, turn, activity, and managed-input helpers where their current terminal fence assumes every error ends the execution.

## Dependencies

None.

## Risks

- Generic `EventTypeError` currently feeds terminal execution state; every intermediate conversion must retain the distinction.
- Runtime-ready status can admit a successor too soon. Keep the prompt fence until the durable turn boundary is certain.
- A real stream close can follow attestation. Terminal precedence must survive asynchronous watcher delivery.
- Queued dispatch acceptance and foreground outcome are separate facts; do not requeue already accepted work.
- Existing remote components omit the field and must remain conservative.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/transient-turn-runtime-continuity.md): runtime preservation and nonblocking durable failure.
- [Design](../../specs/platform/system-design/transient-turn-runtime-continuity.md): evidence contract, runtime settlement, ownership, and durable projection.
- [Decision](../../decisions/2026-10-03-transient-turn-runtime-lifetime.md).
- Existing Codex capacity fixtures, continuation safety snapshots, retained turn outcomes, lifecycle completion claims, and watcher conversion tests.
- Existing provider-neutral recovery and interruption continuation contracts linked from the design.

## Results

Implemented host-validated retained-turn evidence, lifecycle settlement, caller ownership, watcher propagation, and durable error projection. The runtime remains tracked and ready after an eligible failed turn, while unsupported contexts and real disconnects retain terminal behavior.

Verification passed:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/instance -count=1)`
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher ./internal/orchestrator -run 'TurnFailure|TransientTurnFailure|CapacityAfterTools|PromptFailureDisposition|SessionRecovery|Completion' -count=1)`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

Review follow-up closed the cross-package lock-order and automation ownership gaps. Retained failure state is captured under the prompt lifecycle lock, published synchronously after releasing it, and held behind a generation settlement fence until callbacks finish. Automation-owned task origins are excluded from interactive retention and routed through their existing terminal failure owner when encountered by the orchestrator guard.

Additional verification passed:

- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestTransientTurnFailure(PublishesOutsidePromptLockAndFencesSuccessor|KeepsExecutionReady|RequiresCurrentEligibleRuntime|DuplicateDoesNotRepublish|DoesNotOverrideRuntimeDisconnect)' -count=1`: passed, including the synchronous memory-bus/cancel barrier and successor-generation fence.
- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher ./internal/orchestrator -run 'TurnFailure|TransientTurnFailure|CapacityAfterTools|PromptFailureDisposition|SessionRecovery|Completion' -count=1`: all three packages passed.
- Automation-origin regression tests cover both `automation_run` and `automation_task` after observable work, including single terminal settlement and cleanup with no retained interactive failure.
- `go test -trimpath -race -tags fts5 ./internal/backendapp -run '^TestLifecycleAdapter_SatisfiesRetainedPromptFailureAcknowledgementSeam$' -count=1`: passed; the production lifecycle adapter forwards the generation acknowledgement required to release successor admission.
- The managed desktop E2E `integration: read continues in the same live native conversation without original prompt replay` passed against the real backend and mock ACP runtime after the adapter wiring fix.
