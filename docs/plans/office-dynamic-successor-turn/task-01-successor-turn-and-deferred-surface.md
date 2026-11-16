---
id: "01-successor-turn-and-deferred-surface"
title: "Keep the dynamic successor's completion applicable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-STALL-VISIBILITY-001
  - REQ-OFFICE-STALL-VISIBILITY-004
acceptance_criteria:
  - AC-OFFICE-STALL-VISIBILITY-001.6
  - AC-OFFICE-STALL-VISIBILITY-004.1
  - AC-OFFICE-STALL-VISIBILITY-004.2
  - AC-OFFICE-STALL-VISIBILITY-004.3
system_design:
  - ../../specs/office/system-design/stall-visibility.md
---

# Task 01: Keep the dynamic successor's completion applicable

## Summary

Start a turn for the Office prepared relaunch. Bind a turn-less delivery
terminal only when its prompt generation still owns the session. Persist the
terminal turn with its exact workflow receipt, then surface a completion that
is still deferred.

## In scope

- `officeDynamicRelaunchOptions` in `dynamic_launch.go` (AC-004.1).
- `workflowEffectForCompletion` and `completedEventClosingTurn` in
  `agent_delivery_effect.go`, used by both completion handlers (AC-004.2,
  AC-004.3).
- `handleAgentCompletedLocked` verifies prompt-generation ownership before
  closing a turn-less delivery completion. The SQLite repository binds the
  persisted terminal and workflow receipt in one guarded transaction
  (AC-004.2, AC-004.3).
- `surfaceDeferredOfficeCompletion` and the `deferred_completion` gate in
  `stuck_signal_watchdog.go` (AC-001.6).

## Verification

- `TestRelaunchDynamicOfficeTaskBindsSuccessorTurn` (AC-004.1).
- `TestWorkflowEffectForCompletionBindsClosingTurn`,
  `TestWorkflowEffectForCompletionKeepsOtherEvents`, and
  `TestCompletedEventClosingTurnUsesActiveTurn` (AC-004.2, AC-004.3).
- `TestDeferredOfficeCompletionSurfacesPendingSignal` (AC-001.6, AC-003.1
  through AC-003.4).
- `TestAgentCompletedTurnlessDeliveryIgnoresSupersededPrompt` and
  `TestDeliveryRetentionTurnlessTerminalBindsWorkflowTurn` cover stale-owner
  rejection and durable receipt plus settlement.

## Fixup verification

- `go test ./internal/orchestrator -run TestAgentCompletedTurnlessDeliveryIgnoresSupersededPrompt -count=1` passed.
- `go test ./internal/task/repository/sqlite -run TestDeliveryRetentionTurnlessTerminalBindsWorkflowTurn -count=1` passed.
