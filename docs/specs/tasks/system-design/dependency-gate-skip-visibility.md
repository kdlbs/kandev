---
status: current
system: tasks
requirements:
  - REQ-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001
created: 2026-09-16
owners:
  - kandev
---
# Dependency Gate Skip Visibility System Design

## Purpose and boundaries

This design owns the backend log contract for automated launches that the
dependency gate deliberately skips. A skipped auto-start is exceptional and
operator-visible by definition: a task moved onto an auto-start step and did
not launch. The skip is therefore logged at WARN, matching the gate's adjacent
lookup-failure path.

Gate semantics, the fail-closed read behavior, and launch-token handling are
owned by [Task Dependencies and Auto-Start Chains](task-dependencies.md). This
design changes only the observability of the genuine blocked verdict; it adds
no endpoint, feature flag, database column, or user-facing copy.

## Requirement mapping

| Criterion | Design section |
| --- | --- |
| AC-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.1 | Blocked-skip log contract |
| AC-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.2 | Non-blocking paths |

## Blocked-skip log contract

`dependencyBlocksAutoStart` in `internal/orchestrator` is the single gate
consulted by every automated launch path: step entry
(`task.workflow_step_entered`), WIP queue promotion (`task.queue_promoted`),
and automated session launch (`session.launch`). The caller's event name is
passed in and prefixes the log message, so one log site identifies which path
skipped the launch.

When the gate's read succeeds and reports blocked, the service logs one WARN
entry through its structured zap logger:

- Message: `<eventName>: task has unresolved dependencies; skipping auto-start`.
- Fields: `task_id` (string) and `blocked_reason` (string, the gate's reason
  such as `pending` or the failed-predecessor reason).

A read error keeps the existing separate WARN entry (`dependency lookup
failed; skipping auto-start`, with the error field) and the fail-closed
return; it is not conflated with the genuine-block entry because the blocked
reason differs from a read error. No launch, error surface, or task-state
change accompanies the blocked-skip entry: the task simply stays where it is
until its dependencies resolve.

## Non-blocking paths

A task that passes the gate logs nothing at this site. The WARN level is
reserved for the exceptional skip so ordinary launches do not train operators
to ignore warnings. Debug-level detail for unrelated gate decisions stays at
Debug.

## Verification

`TestDependencyBlocksAutoStartWarnsOnBlockedSkip` in
`event_handlers_dependencies_test.go` drives the gate with a blocked reader,
asserts exactly one WARN entry with the event-prefixed message, and checks the
`task_id` and `blocked_reason` fields. This test covers the blocked path.
The unblocked and lookup-error paths are not asserted in this file.

## Implementation Plans

- [Dependency gate skip visibility](../../../plans/dependency-gate-skip-visibility/plan.md)
