---
id: "03-automatic-delivery"
title: "Deliver automatic foreground cycles once"
status: blocked
wave: 3
depends_on:
  - "01-launch-observations"
  - "02-opencode-producer"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-007.3
  - AC-AGENTS-BACKGROUND-WORK-007.5
  - AC-AGENTS-BACKGROUND-WORK-008.1
  - AC-AGENTS-BACKGROUND-WORK-008.2
  - AC-AGENTS-BACKGROUND-WORK-008.3
  - AC-AGENTS-BACKGROUND-WORK-008.4
  - AC-AGENTS-BACKGROUND-WORK-008.5
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 03: Deliver automatic foreground cycles once

## Summary

Consume the proven OpenCode producer contract through the existing ACP adapter
and durable delivery path. Separate job outcomes, client-origin foreground
cycles, and automatic foreground cycles before journaling.

## In scope

- Add pure metadata parsing in proposed dialect_opencode.go. Negotiate only the
  exact supported contract when the shared feature is enabled.
- Add session-scoped provider-cycle ownership, with connection/reset fencing.
  Capture identity at notification admission, before asynchronous conversion.
- Map client-origin cycles to their admitted prompt generation. Preserve
  session/prompt response completion ownership.
- Map an automatic cycle to one normalized start, its content, and one terminal
  event after the notification barrier. Keep usage attribution once.
- Route identified job results to existing workload/output observations without
  reopening the original launch tool as a foreground operation.
- Reuse the journal, watcher, task turn service, and current authenticated
  owner/generation/cursor fences. Keep native identity replay-stable.
- Bypass the idle-based async finalizer only for negotiated OpenCode cycles.
  Preserve the legacy Claude async behavior and unsupported peers.

## Out of scope

Rewriting durable delivery, adding a second transcript writer, new SQL tables,
synthetic prompt polling, job controls, and workflow policies.

## Acceptance

1. A captured producer fixture produces one automatic response, one foreground
   completion, and one usage attribution after the original prompt has settled.
2. Interleaved jobs, queued conversion, user-send races, journal replay,
   reset/replacement, and late predecessor terminals preserve exact ownership.
3. Legacy/malformed/feature-off peers keep existing prompting. A quiet automatic
   tool remains active until explicit terminal evidence arrives.

## Verification

From the Kandev root:

~~~bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(OpenCodeAutomatic|OpenCodeContinuity|OpenCodeBackground|OpenCodePromptOwnership|EnqueueACPUpdate|HandleACPUpdate)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/watcher ./internal/task/service -run 'Test(OpenCodeAutomatic|OpenCodeBackground|NativeAsync|NativeReplies|NativeMessage|RecoveredLiveCompletion|DurableEvent)' -count=1)
git diff --check
~~~

Proposed tests include TestOpenCodeAutomaticCycleSingleCompletion,
TestOpenCodeAutomaticUsageOnce, TestOpenCodePromptOwnershipAtEnqueue, and
TestOpenCodeBackgroundReplayAndReset. Use channel/barrier races instead of sleeps.

## Files likely touched

- Proposed ACP dialect_opencode.go, opencode_automatic_turn.go, and their tests.
- Existing ACP dialect.go, adapter_helpers.go, adapter_tools.go,
  adapter_updates.go, adapter_prompt.go, and adapter_async_complete.go.
- Existing streams/agent.go only for an additive lifecycle discriminator if
  required; test its omission with older peers.
- Existing runtime/lifecycle/native_async_turn_identity_test.go, watcher
  delivery tests, and orchestrator/event_handlers_streaming.go.
- Proposed focused automatic-turn integration tests alongside these owners.

## Dependencies

Tasks 01 and 02. Resolve Task 02's exact accepted wire fixture and packaged
artifact before coding the consumer. A consumer-only fixture cannot waive it.

## Risks

Current prompt generation is not native automatic-cycle identity. Late
background results must not complete, bill, or acquire the next human turn.
The existing idle finalizer is a compatibility path, not proof of provider idle.

## Parallelism

sequential

## Inputs

- [Automatic foreground design](../../specs/agents/system-design/background-work-opencode.md#automatic-foreground-turns).
- Existing adapter_updates_prompt_generation_test.go, async_complete_test.go,
  native_async_turn_identity_test.go, and journal reconciliation tests.
- Task 02's accepted producer artifact and sanitized full-wire fixture.

## Results

Blocked on Task 02. The negotiation, cycle ownership, and automatic-turn
consumer would otherwise be coded against an unaccepted wire spelling. The
existing durable-delivery, usage, and async-finalizer paths are unchanged.

Task 01 already separates launch completion from workload completion and settles
unverified legacy work as unknown, which is the correct 2.2.0 behavior until a
producer can deliver later outcomes.
