---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
created: 2026-08-24
owners:
  - cfl12
---

# Task Launch Failure Recovery System Design

## Context and boundaries

The task system owns launch gating, typed launch-error projection, and recovery
actions for a task repository. GitHub remains the source of pull-request state;
the workspace system resolves repository branches; the UI renders the
task-owned projection.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001 | PR gate, error projection, recovery actions, responsive surface |

## PR gate and launch paths

The auto-start gate selects relevant PRs by explicit repository and PR identity,
then exact repository/branch identity. It skips only when at least one relevant
PR exists and every one is merged or closed. An open, empty, unknown, failed
lookup, or absent PR leaves the normal auto-start path available. Manual launch
always bypasses this gate.

The gate stores an informational task error when it suppresses an auto-start.
The error is stamped from the sorted relevant PR identities and states so the
same observation is idempotent.

## Error model and projection

Session-owned errors live in `task_sessions.metadata.last_agent_error`.
Pre-session gate errors live in `tasks.metadata.last_launch_error`. Both use a
safe message, timestamp, stable category, bounded details, ordered recovery
actions, exact task-repository identity when applicable, and an idempotency
stamp.

The supported categories are `base_branch_missing`, `pr_already_closed`,
`default_branch_unresolved`, and `generic_launch_failure`. The supported
actions are `retry_default`, `pick_base_branch`, and `mark_review_done`.

The `TaskStatusSummary.active_error` projection selects the newest active
record, limits strings and actions, removes duplicate actions without changing
their order, and ignores malformed optional metadata without invalidating the
full summary. Boot state, task reads, and `task.status_summary.updated` carry
the complete replacement projection.

## Recovery action contract

The `task.launch.recover` action authorizes the task first, then proves any
session and task-repository identities belong to that task. The request includes
the current error stamp; stale stamps fail without mutation.

- `retry_default` resolves the live remote default for one repository row and
  relaunches.
- `pick_base_branch` validates and persists one selected branch before
  relaunch.
- `mark_review_done` is allowed only for a valid terminal workflow step and
  when every relevant PR is terminal; it uses the normal task-move service.

The existing `session.recover` action is unchanged. A failed recovery
preserves the source error record, keeps it visible, and updates its typed
category, bounded details, and valid actions. A successful recovery clears the
source error only after its write and relaunch or move succeed.

## Branch resolution and persistence

Local default detection remains a pure helper and returns empty when only a
local HEAD branch exists. The worktree manager owns bounded remote-default
refresh. A resolved default may be cached in `repositories.default_branch`,
but `retry_default` and `pick_base_branch` must write the resolved base to
the exact `task_repositories` row and that write must succeed before relaunch.
No new table is required.

## Failure and security

PR lookup failures launch normally. Remote-default timeout, authentication,
network, missing branch, and unresolved default remain distinct diagnostics.
Ambiguous repository identity omits repository-scoped actions. Foreign session
or repository IDs and stale stamps fail without mutation.

## Responsive presentation

The task Chat surface renders one persistent error card from the shared
projection. Desktop actions are inline; mobile actions wrap and branch
selection uses the existing mobile picker. Both surfaces use the same action
authorization and remain free of horizontal overflow.

## Verification

- Test relevant-PR selection, terminal/open precedence, lookup failures, and
  manual bypass.
- Test error projection limits, stamps, persistence, and recovery authorization.
- Test branch self-healing and mark-review-done terminal-step checks.
- Cover desktop and mobile recovery actions with the existing task Chat tests.
