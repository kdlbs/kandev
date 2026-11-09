---
id: "04-activity-and-input"
title: "Preserve activity and prompt ownership"
status: blocked
wave: 4
depends_on:
  - "03-automatic-delivery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
  - REQ-AGENTS-BACKGROUND-WORK-009
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-007.4
  - AC-AGENTS-BACKGROUND-WORK-007.5
  - AC-AGENTS-BACKGROUND-WORK-008.3
  - AC-AGENTS-BACKGROUND-WORK-008.4
  - AC-AGENTS-BACKGROUND-WORK-009.1
  - AC-AGENTS-BACKGROUND-WORK-009.2
  - AC-AGENTS-BACKGROUND-WORK-009.3
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 04: Preserve activity and prompt ownership

## Summary

Show verified OpenCode background activity while its idle foreground accepts
ordinary prompts. Keep user input ordered when automatic execution starts at
the same time.

## In scope

- Feed a generation-fenced OpenCode provider sample through the existing
  BackgroundProbe/parked projection. Keep Claude's process-probe path unchanged.
- Preserve root coarse state. Working-in-background is a verified activity
  display, not a synthetic RUNNING turn or a workflow move.
- Retain old jobs across beginPromptTurn and new prompt generations. Recompute
  background activity from current provider evidence after foreground settlement.
- Clear parked activity on automatic foreground start. Restore it after that
  cycle ends only when another job has fresh live evidence.
- Use the producer's conditional idle-revision admission. A definite
  not-accepted response defers the same user message once through the existing
  queue. Separate immutable dispatch-attempt identity from message identity.
- Preserve uncertain-delivery recovery. Do not automatically retry input after
  a missing response or manufacture a root completion from queue deferral.
- Keep unsupported active-turn steering false. Generating input queues.
  Foreground-idle input sends normally without stopping sibling jobs.
- On reset, disconnect, incomplete snapshot, and stale ownership, report unknown
  activity and retain inspectable history. Persisted rows alone are not liveness.

## Out of scope

Changing workflow transition policies, task goals, notification policy, Claude
handoff, host process scans for OpenCode, or same-turn steering.

## Acceptance

1. Foreground-idle plus verified jobs shows background activity and accepts
   an ordinary prompt. Starting that prompt preserves every prior job.
2. Automatic-start/user-send races queue or dispatch each user message once.
   Active OpenCode foreground execution never acquires unsupported steering.
3. Lost or stale provider evidence clears verified activity without declaring
   completion. Existing workflow, notification, and Claude admission tests pass.

## Verification

From the Kandev root:

~~~bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator ./internal/orchestrator/messagequeue ./internal/agent/runtime/agentctl -run 'Test(OpenCodeBackground|OpenCodeAutomaticUserPromptRace|OpenCodeProviderParkedEvidence|Parked|BackgroundProbe)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(OpenCodePrompt|OpenCodeBackground|OpenCodeAutomatic|PromptHandoff)' -count=1)
git diff --check
~~~

Proposed cases: TestOpenCodeBackgroundPromptAdmission,
TestOpenCodeAutomaticUserPromptRace, TestOpenCodeProviderParkedEvidence, and
TestOpenCodeBackgroundReconnectUnknown. Include definite rejection versus
uncertain dispatch, several surviving jobs, stale idle revision, and reset.

## Files likely touched

- Existing orchestrator/background_probe.go, parked_projection.go,
  background_work_attestation.go, event_handlers_streaming.go, and prompt/queue admission.
- Existing agent/runtime/agentctl/background_probe.go and optional adapter
  probe interfaces, only to carry normalized provider evidence.
- Proposed ACP opencode_automatic_turn.go admission/revision handling.
- Existing orchestrator/messagequeue and durable submission handling for typed
  not-accepted deferral. Use their current storage rather than another queue.
- Proposed orchestrator/opencode_background_activity_test.go and focused queue tests.

## Dependencies

Task 03 and the producer's proven conditional admission from Task 02.

## Risks

A Kandev mutex does not serialize a native automatic start inside OpenCode.
Definite rejection and uncertain delivery have different retry meanings.
beginPromptTurn currently clears attestation, but earlier jobs can outlive it.

## Parallelism

sequential

## Inputs

- [Activity and input design](../../specs/agents/system-design/background-work-opencode.md#activity-and-input).
- [Ownership and delivery](../../specs/agents/system-design/background-work-opencode.md#ownership-and-delivery).
- Existing parked_projection_test.go, parked_projection_turn_finished_ordering_test.go,
  background_probe_test.go, and background launch extensibility tests.

## Results

Blocked on Task 03 for the verified provider sample and expected-idle-revision
admission. The parts that do not require the producer are covered by existing
behavior plus Task 01:

- An idle foreground accepts an ordinary prompt while retained jobs remain
  inspectable (the launch binding no longer fences later prompts after
  settlement).
- Unverified OpenCode launches never attest parked/working activity, so the
  session keeps its durable coarse state and history instead of a synthetic
  RUNNING turn.
- Active-turn input keeps the existing queue behavior; OpenCode advertises no
  steering.

The generation-fenced provider probe sample, automatic-start/user-send race
serialization, and definite not-accepted deferral remain pending with Task 03.
