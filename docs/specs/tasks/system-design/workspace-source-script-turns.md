---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
created: 2026-11-07
updated: 2026-11-08
owners:
  - kandev
---

# Workspace source script turns

## Purpose and boundaries

The task system owns attachment serialization and conversation turns. Worktree setup and cleanup messages cross both boundaries.
This design completes the [attachment design](attach-workspace-sources.md) for script-message creation during materialization.

The existing [attachment requirement](../requirements/attach-workspace-sources.md) remains authoritative. This correction restores its existing behavior.
Executor capabilities, workspace placement, script execution, and public request schemas retain their existing contracts.

## Requirement mapping

| Requirement | Acceptance criteria | Design sections |
| --- | --- | --- |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001 | AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4, AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5, AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6 | Turn binding, mutation serialization, and failure handling |

## Components and responsibilities

- `task/service.Service` owns the task mutation gate and message persistence.
- `backendapp.taskServiceAdapter` translates trusted worktree messages into task-service requests.
- `worktree.DefaultScriptMessageHandler` creates a message before script execution and updates its output and status.
- `worktree.Manager` retains its existing setup warnings and cleanup behavior.

`backendapp.bootMsgAdapter` already uses completed turns for idle lifecycle messages.
The [todo recovery design](queued-session-ownership.md#recovery-metadata-and-prompt-turns) uses the same distinction between conversation and lifecycle bookkeeping.
This correction applies that existing distinction at the worktree adapter.

## Turn binding

The trusted adapter resolves one turn before it calls `Service.CreateMessage`:

1. If the request supplies `TurnID`, preserve that ID without an active-turn lookup.
2. Otherwise, call the non-creating `Service.GetActiveTurn` for the requested session.
3. If the lookup returns an active turn, pass its explicit ID to message creation.
4. If the lookup confirms no active turn, set the internal `CompletedTurn` field.
5. If the lookup fails, return the error. A lookup error does not prove that the session is idle.

An observed active turn can complete before message persistence. Its captured ID remains the message owner, even if a successor starts.
The adapter must not repeat turn selection or leave an empty ID with the ordinary open-turn fallback enabled.

`CompletedTurn` uses `Service.createCompletedTurn`, which persists `CompletedAt` and `metadata.lifecycle_only` at creation.
This path does not acquire the workspace mutation gate or publish `turn.started`.
The completed turn represents bookkeeping. The script message retains its own starting, running, exited, or failed status while execution proceeds.

The internal field remains unavailable to JSON clients. No new worktree DTO flag, public parameter, or message type is necessary.
The policy covers setup and cleanup requests through the same adapter.

## Mutation serialization

`AttachWorkspaceSources` and `AddBranchToTask` retain their task mutex through validation, persistence, materialization, compensation, and publication.
Real conversational `StartTurn` and `ReserveTurn` calls continue to acquire that mutex.
This preserves the existing regression `TestAttachWorkspaceSources_HoldsTaskMutationGateThroughMaterialization`.

During attachment, script-message creation uses an explicit existing turn or the completed lifecycle path.
It must never call the creating fallback in `getOrStartTurn` while the attachment owns the mutex.
No reentrant mutex, context bypass token, early unlock, or asynchronous setup worker is introduced.

Batch attachment still requires every session to have no active turn. One active session rejects the entire batch.
Legacy add-branch still permits its invoking turn and retains sibling placement and unchanged agent CWD.

## Failure handling and execution lifetime

The script handler retains its detached execution context and existing timeout.
Caller cancellation does not create a second script execution or change the existing attachment commit/compensation boundary.
There is no new shared worker or waiter lifecycle.

Turn lookup and message persistence errors retain the existing script-handler error path.
The manager records setup errors as warnings and keeps the worktree. Cleanup remains best effort.
Nonzero setup exits still produce failed script messages without making worktree creation fatal.

Failures after materialization retain the attachment service's compensation behavior.
Any cleanup message created during compensation follows the same turn-binding policy.
An exact attachment retry retains its existing no-op behavior and does not run setup again.

## Persistence, security, and observability

Messages and completed lifecycle turns use existing repositories and events. No migration or stored-data rewrite is necessary.
Existing task/session authorization remains in `CreateMessage`. Turn resolution does not grant message-write authority.
Script output and repository credentials retain their current handling.

Existing script messages, final statuses, setup warnings, and attachment events provide execution evidence.
The correction adds no metric, log stream, or public recovery control.
Public documentation already describes the restored attachment behavior, so it needs no new procedure.

## Verification

The backend regression uses real SQLite, local Git worktrees, the production adapter, and a script subprocess.
It covers idle batch attachment, idle legacy add-branch, and legacy add-branch during an existing turn.
Assertions include script output, final status, persisted sources, turn ownership, and admission of the next user turn.

Additional coverage includes explicit turn IDs, lookup errors, a completed observed turn, nonzero setup exits, compensation, and prelaunch deferral.
Existing mutation-gate and lifecycle-shadow regressions remain mandatory.
Desktop and phone E2E checks submit through the existing Files flow and verify script output and a subsequent conversational response.

## Related decisions and delivery

- [Runtime-mutable task workspace sources](../../../decisions/2026-07-22-runtime-mutable-task-workspace-sources.md).
- [Legacy add-branch live rescan](../../../decisions/2026-07-27-legacy-add-branch-live-rescan.md).
- [Fix package](../../../plans/workspace-source-script-deadlock/plan.md).

The correction reuses existing turn and serialization rules. Its local adapter choice does not require a new ADR.
