---
status: current
system: tasks
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-003
---

# Successful child completion system design

## Purpose and boundaries

`on_children_completed` requires a nonempty set of successful direct children.
Every counted child must have persisted state `COMPLETED`. The rule applies
without a setting, including to existing and imported workflows.

The task system owns trigger readiness. Global terminal states, session
recovery, dependency resolution, and independent Office scheduler wakes retain
their existing contracts. The trigger does not prove that child branches are
merged.

## Requirement mapping

| Criteria under requirement 003 | Design sections |
| --- | --- |
| `.1`, `.2`, `.4`, `.9` | Readiness before dispatch |
| `.3`, `.5`, `.6` | Recovery and idempotency |
| `.7`, `.10` | Compatibility and boundaries |
| `.8` | Existing workflow help |

## Readiness before dispatch

Keep eligibility in the existing producers. Do not emit a workflow trigger
until its counted children all succeeded. This prevents every configured
reaction, including transitions and side-effect-only declarations, before the
engine or operation ledger can act.

### Orchestrator

`readyChildCompletionRows` already loads the full counted child set once through
`ListChildCompletionRows`. Replace its `allChildrenTerminal` predicate with an
`allChildrenCompleted` predicate that checks `row.State == TaskStateCompleted`.
Keep the existing nonempty check.

Preserve the repository membership query: direct children with no archive
marker, `is_ephemeral = 0`, and origin other than `automation_run`. Unknown,
failed, cancelled, and active states fail the equality check. A read error
retains the existing warning and early return.

This check runs before `parentCompletionContext`, operation locking, engine
callbacks, and the orchestrator's external operation mark. No new deferred
result, payload field, or engine interface is needed.

Keep the global terminal-event listener and dependency handler intact. Update
its comment that currently contrasts failure-ready children with successful
dependencies. A failed candidate event can still reach the readiness check and
return without effects.

### Office engine producers

`queueChildrenCompletedRun` and `ParentWakeReconciler.reconcileOne` both call
`office/service/resolveWaveIdentity` before dispatch. That helper already reads
`ListWaveMembers`, rejects an empty set, and returns `ok=false` for unready
members. Require `COMPLETED` there instead of accepting `CANCELLED`.

Use the same read for readiness and identity. Keep the helper signature,
identity encodings, payloads, source-specific admission, and dispatcher API.
A rejected snapshot returns before dispatch, so the recovery path writes no
wake receipt. Existing no-action dispatch handling stays unchanged.

The similarly named helper in `office/scheduler` belongs to independent
scheduler cascades. It is outside this workflow-trigger change. Preserve its
membership and terminal rules.

## Recovery and idempotency

A rejected child set never reaches the engine or an operation mark. It can
therefore become eligible when a failed or cancelled child later becomes
`COMPLETED`. Use existing task events and completion-step paths for that retry.
No polling worker or synthetic completion is required.

Keep `childCompletionOperationID` and Office wave/operation identity unchanged.
Previously accepted operations remain accepted. Duplicate successful delivery
and reopen/complete cycles retain existing behavior.

The [parent commit design](subtask-completion-parent-commit.md) and
[busy parent design](subtask-completion-busy-parent-wake.md) remain authoritative
for committed transitions and queued prompts. Readiness does not add locks
across runtime I/O or serialize child mutation with the parent lifecycle.

## Existing workflow help

Update `workflows:childrenCompletedHelp` in all supported locales. Explain that
every active direct child must be `COMPLETED`, and failed or cancelled children
keep the parent waiting. Explain that at least one counted active direct child
is required, archived, ephemeral, and automation-run children are ignored, and
grandchildren do not count. Preserve the explanation of membership and empty
sets.

The existing **When Child Tasks Complete** selector remains the only control.
Reuse `EventTransitionSelect` and its touch-aware `HelpTip`: desktop hover or
keyboard help, and the existing help drawer on phones. No control, route,
layout, saved draft, or touch geometry is added.

The [plan preview](../../../plans/children-completed-success-policy/plan.md#ascii-ui-preview)
shows the unchanged control and revised help. Public coordination, workflow
reference, and workflow tips pages explain the changed behavior.

## Compatibility and boundaries

This intentionally changes readiness for all workflows. A parent previously
advanced by failed or cancelled children will now wait. A user can recover or
complete the child through existing controls. This change does not rewind a
parent already advanced or replay a consumed operation after upgrade.

No migration, stored flag, new API field, SDK change, portable version, feature
flag, or notification contract is introduced. Existing workflow-edit and task
permissions remain authoritative.

`FAILED` and `CANCELLED` remain terminal states elsewhere. Office wave identity
is still a function of membership IDs, separate from a producer's readiness
rule. Independent Office scheduler wakes and session-recovery issue #5256
remain separate work.

## Verification boundary

Use the real orchestrator handler for failed batches, mixed outcomes, cancelled
children, and later successful completion. Assert no parent move, callback, or
operation mark while any counted child is not successful.

Exercise both Office engine producers and their recovery receipt boundary.
Preserve the existing all-success, busy-parent, parent-commit, duplicate, and
wave-identity tests. Update successful-delivery fixtures that currently use a
cancelled child, and retain explicit cancellation-blocking regressions.

Desktop and phone Playwright scenarios prove the existing selector, revised
help, and resulting parent progression through an isolated backend.

## Decision

The user explicitly requested a simple fix without an additional setting.
Successful completion becomes the single readiness rule. Producer checks fit
the current design and avoid expanding configuration or the engine contract.
