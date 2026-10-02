---
status: current
system: platform
requirements:
  - REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001
---

# Prompt completion ownership design

## Purpose and boundaries

This design completes the existing generation-ownership contract from ADR 0035.
It covers lifecycle dispatch acknowledgement, completion finalization, and subsequent autonomous completion.
The [workflow reset design](../../tasks/system-design/workflow-step-agent-start-ownership.md) retains cancellation and reset ownership.

## Requirement mapping

| Criteria for REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001 | Design section |
| --- | --- |
| `.1`, `.2`, `.5` | Completion finalization |
| `.3`, `.4` | Dispatch acknowledgement |
| `.4`, `.6` | Failure and compatibility |

## Components and state

The implementation belongs in `apps/backend/internal/agent/runtime/lifecycle/`.

- `SessionManager.sendPrompt` serializes ordinary prompt calls with `promptMu`.
- `finishAcceptedPrompt` handles dispatch-only acknowledgement and blocking completion waiting.
- `claimPromptCompletion` validates execution identity and generation before terminal side effects.
- `finishPromptCompletion` settles terminal state, signals completion, and publishes an immutable terminal event after releasing the lifecycle lock.
- `ExecutionStore` protects execution identity and generation snapshots.

`dispatchedPromptPending` represents unresolved dispatch-only completion, not an unconsumed historical signal.
`promptCompletionGeneration` records an accepted numbered completion.
`promptLifecycleMu` fences acknowledgement bookkeeping and numbered completion processing.
The existing `promptDoneCh` remains the completion channel; this correction adds no competing receiver.

## Completion finalization

Keep generation validation at `claimPromptCompletion`.
A stale, duplicate, or replacement-execution event must return before barrier mutation.

Do not clear the pending flag at the start of the claim.
The claim precedes workspace finalization, transcript flushing, and history publication.
An early clear lets a successor bypass the barrier while those operations still use shared state.

For an accepted numbered success, finish transcript processing and enqueue its completion signal first.
Then clear the pending flag while the existing `promptLifecycleMu` lease still protects the accepted generation.
Clear it before that lease ends and before `AgentReady` publication can admit a successor.
Retain signal-before-`AgentReady` ordering and generation checks in waiting consumers.

For an accepted numbered error, apply FAILED or shutdown STOPPED under the lifecycle lease before signaling completion or clearing pending.
Capture the terminal event payload, attempt identity, and failure evidence at that transition.
Then signal completion, clear pending, and release `promptLifecycleMu`.
Persist terminal state, run failure classification or shutdown teardown, and publish the captured `AgentFailed` or `AgentStopped` event only after unlocking.
A synchronous subscriber can wait on dispatch acknowledgement ownership without holding the lock acknowledgement needs to run its callback.
A waiting successor observes terminal status after its completion signal and cannot create another prompt generation for that execution.

A waiter that already observed a pending prompt still receives the matching signal.
A later caller can skip the completed barrier and drain the historical signal through the existing `sendPrompt` path.
Neither path clears a successor's barrier.

## Dispatch acknowledgement

Replace the unconditional flag store in `finishAcceptedPrompt` with generation-checked bookkeeping.
Serialize this operation with `promptLifecycleMu` and use the store's identity-aware snapshot when a store exists.
Retain the standalone `SessionManager` path for callers without an execution store.

Set pending only for the same execution and generation when completion has not been accepted.
If completion already owns the generation, keep pending false.
If a successor or replacement owns the state, do not mutate its barrier.
Release locks before `onDispatched`; preserve callback delivery and the dispatch result contract.

The completion mutex spans finalization, so a late acknowledgement cannot observe an unfinished claim as fully finalized.
Preserve lock order: prompt serialization, then prompt lifecycle, then short store access.
Completion handlers must not acquire `promptMu` because its owner can await their signal.

## Failure and compatibility

Generation-zero completion retains its existing autonomous-work path.
It remains rejected while a dispatch-only foreground prompt is pending.
This change does not broaden admission of unnumbered events during foreground work.

Preserve cancellation ownership, timeout behavior, reset cleanup, and blocking-prompt completion consumption.
Update test fixtures that currently require pending state after an already accepted completion.
Do not weaken their ownership, timeout, or next-prompt assertions.
Apply failed/stopped status before releasing the error completion fence, and never publish its synchronous terminal event while holding `promptLifecycleMu`.

The separate recursive startup-lock problem in issue #4149 remains outside this correction.
Tests for this package must not claim that generation-zero completion is safe against that unrelated pending-writer race.
Coordinate release with that repair because this correction allows more valid unnumbered completions through.

## Persistence and interfaces

All changed state is execution-local. There is no migration, new protocol field, API, configuration, or runtime flag.
Provider metadata and notification rendering remain unchanged.

## Verification

Use real lifecycle event handlers for both completion/acknowledgement orders.
Use the existing WebSocket mock for same-execution successor delivery.
Cover blocked finalization, waiting consumers, errors, stale generations, duplicate completions, and replacement executions.
Run cancellation, context-reset, wakeup, and steering compatibility tests under the race detector.

## Related decisions

- [ADR 0035: Prompt generation identity](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

## Implementation plans

- [Dispatch completion and wakeup](../../../plans/dispatch-completion-wakeup/plan.md)
