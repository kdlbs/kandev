---
id: "02-continuation-owner"
title: "Add bounded same-conversation continuation"
status: completed
wave: 2
depends_on:
  - "01-continuation-evidence"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 02: Add bounded same-conversation continuation

## Summary

Extend the existing retry owner with continuation mode, restore-only admission,
and one shared attempt budget. User actions and accepted human work must win
ownership races; unsafe or ambiguous work stays manual.

## In scope

- TDD admission with the typed snapshot and positive support from Task 01.
- Factor replay/continue mode in `handleTransientFailure` and
  `retryTransientPrompt`, leaving `promptAttemptPreResultSafe` unchanged.
- Native-only restore option through runtime/executor/lifecycle; preserve
  provider identity and selected settings and reject fallback-to-new-session.
- Static internal continuation instruction, automatic origin, normal prompt
  acceptance/audit boundary, no user-message insertion or attachment replay.
- Episode count/safety accumulation, bounded pre-dispatch restore failures,
  ambiguous acceptance refusal, cancellable timers, teardown confirmation,
  identity/config/archive/queued-work checks, cancellation before/after acceptance.
- Correct interrupted settlement, CI outcome and workflow behavior, notice
  cleanup on terminal paths, startup stale-notice cleanup with live adoption guard.
- Emit real proposed mode/phase/disposition and started count for Task 03.

## Out of scope

Provider-name checks, original prompt replay after output, effectful continuation,
dynamic/Office policy changes, new persistence tables or durable scheduler,
frontend copy, and runtime toggle promotion.

## Acceptance

- Supported output/read-only interruptions restore the same conversation and
  dispatch one continuation; unsupported, stale, unsafe, failed-native-restore,
  and ambiguous acceptance cases never silently dispatch fresh or repeated work.
- Replay, restore failures, and continuation share at most five attempts. A
  write from an earlier replacement remains unsafe. Cancel, human prompt,
  stop/archive/delete/config change, shutdown, and restart cannot revive old
  automatic work or terminate its successor.
- Interrupted settlement never reports success or advances workflow/CI outcomes;
  successful continuation completes through the ordinary path once. Projection
  cleanup uses task-service events and startup does not interrupt adopted live work.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator -run 'Test(InterruptionContinuation|HandleTransientFailure.*Replay|PromptAttemptEvidence|CursorTransportLost)' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -run 'Test(ContinuationNativeOnlyRestore|ContinuationSafetySnapshot|CursorContinuationEvidence)' -count=1)
make -C apps/backend lint
```

Include new `TestInterruptionContinuationBudget`, `Ownership`, `Settlement`,
`Restart`, and `Disabled` families in `provider_interruption_continuation_test.go`
and focused sibling files rather than exceeding file/function limits. Use fake
time/barriers, not sleeps. All listed named tests must be discovered.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/orchestrator/dynamic_evidence.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/task_operations.go` and `session_launch.go`
- `apps/backend/internal/orchestrator/turn_activity.go`, queue admission and startup hooks
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/native_restore_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and resume request types
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- New focused continuation tests in orchestrator, lifecycle, ACP

Prefer narrow new helpers/files to growing existing large orchestration files.

## Dependencies

Task 01 complete with native compatibility evidence and off-by-default wiring.

## Risks

Teardown failures, prompt acceptance uncertainty, missing generation fences,
or status-only cleanup can permit double work. Explicitly test the final prompt
admission race and cancellation of an already dispatched continuation.

## Parallelism

`sequential`

## Inputs

Design sections Recovery admission through Settlement, Rollout and restart;
existing forced teardown claims, resume locks, foreground claims, transient
notice lifecycle, backend-tests guidance, and replay-safety regression tests.

## Results

Implemented the single-owner continuation path, native-ID-only restoration,
five shared attempts, confirmed teardown, bounded preparation, cancellation and
human-supersession fences, truthful phases/counts, managed-input uncertainty,
and interruption settlement without successful completion. Focused race tests
cover refusal, identity changes, ambiguous dispatch, cancellation failure,
sticky mode, shared accounting, stale notice retirement, and live adoption.
Linux integration checks exposed startup and shutdown ownership gaps. Native-only
restore now waits at the shared startup seam and returns the actual error plus
exact failed execution to the owner. The owner confirms teardown and parks the
session before retrying, avoiding startup-grace adoption of a stopped executor.
Dispatch finishes the startup attempt at acceptance and transfers context lifetime
to the normal turn. Shutdown disarms local recovery while retaining its notice
for reconciliation and respecting accepted-runtime survival policy. Compiling
regressions failed before each fix; scoped owner/executor race tests and lint
pass. All seven affected desktop acceptance flows pass, including transient
restore, cancellation, ambiguity, and restart with and without live adoption.

PR review remediation initializes ownership before publication, refuses uncertain execution lookups, checks cancellation identity twice, releases failed teardown claims, and fails closed on unpersisted interruption settlement. Exact automation-turn binding, legacy cancellation, restored-execution identity, and abandoned replay startup records have passing focused race regressions.
