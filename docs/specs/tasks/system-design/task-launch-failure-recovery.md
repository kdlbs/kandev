---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
created: 2026-08-24
updated: 2026-09-06
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
| REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001 | PR gate, initial prompt admission, error projection, recovery actions, responsive surface |

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
`default_branch_unresolved`, `workspace_checkout_failed`, and
`generic_launch_failure`. The supported actions are `retry_launch`,
`retry_default`, `pick_base_branch`, and `mark_review_done`.

The failure category controls the available actions. A repository identity
alone does not make a base-branch action valid.

| Category | Valid actions |
| --- | --- |
| `base_branch_missing` | `retry_default`, `pick_base_branch` |
| `default_branch_unresolved` | `pick_base_branch` |
| `workspace_checkout_failed` | `retry_launch` |
| `pr_already_closed` | `mark_review_done` when the workflow permits it |
| `generic_launch_failure` | `retry_launch` |

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
- `retry_launch` keeps the task-repository settings unchanged and repeats the
  failed launch. It still requires the current error stamp.
- `mark_review_done` is allowed only for a valid terminal workflow step and
  when every relevant PR is terminal; it uses the normal task-move service.

The existing `session.recover` action is unchanged. A failed recovery
preserves the source error record, keeps it visible, and updates its typed
category, bounded details, and valid actions. A successful recovery clears the
source error only after its write and relaunch or move succeed.

## Initial prompt admission

The lifecycle manager owns initial prompt submission after an agent process
starts. Materialization and ACP submission errors occur inside that asynchronous
boundary.

The lifecycle manager sends each error to the existing terminal execution path.
That path publishes `agent.failed` with the execution and prompt evidence.

The orchestrator accepts the failure only for the current execution and prompt.
It completes the active turn and persists the safe session failure.

The task moves to `FAILED` only while the same session still owns its runtime
state. A successor execution or prompt remains unchanged.

Backend shutdown keeps its existing stopped-session behavior. A shutdown error
does not create a durable user-visible launch failure.

The task environment has `creating`, `ready`, `stopped`, and `failed` states.
A worktree path is optional when the state is `creating` or `failed`.
The `ready` and `stopped` states require a non-empty worktree path.

After a materialization error, the materialization owner clears its claim and
stores the `failed` state. This update remains valid when no worktree path exists.

## Branch resolution and persistence

Local default detection remains a pure helper and returns empty when only a
local HEAD branch exists. The worktree manager owns bounded remote-default
refresh. A resolved default may be cached in `repositories.default_branch`,
but `retry_default` and `pick_base_branch` must write the resolved base to
the exact `task_repositories` row and that write must succeed before relaunch.
No new table is required.

## Pull-request checkout isolation

A pull-request head uses a Kandev-owned ref that includes the pull-request
number, specifically `refs/kandev/pull/<N>/head`. The fetch can force-update
this internal ref because users do not own it. Ordinary remote refresh and
pruning do not manage this namespace. The fetch never writes directly to a
user-named local branch.

The worktree manager verifies the fetched ref before it selects a start point.
If the named local branch has unrelated history, the manager preserves that
branch. It creates a unique task branch from the verified pull-request ref.

This rule also applies when two pull requests reuse the same source-branch
name. A local branch from the first pull request cannot block the second pull
request. A fallback branch uses a task-owned deterministic suffix and a
bounded retry sequence, so an existing fallback branch cannot make launch fail
because of one random-name collision.

The manager sets `origin/<source-branch>` as upstream only when that ref points
to the verified pull-request start point. A remote branch with different
history is never attached as the worktree upstream.

If Kandev cannot fetch or verify the pull-request ref, preparation fails with a
typed `workspace_checkout_failed` error. The error retains the existing
credential and path redaction rules.

## Failure and security

PR lookup failures launch normally. Remote-default timeout, authentication,
network, missing branch, and unresolved default remain distinct diagnostics.
Ambiguous repository identity omits repository-scoped actions. Foreign session
or repository IDs and stale stamps fail without mutation.

Initial-prompt errors use a safe generic durable message and the same
stale-event checks as other agent failures. Raw attachment paths and provider
details remain in backend diagnostics and do not reach the durable projection.

## Launch-error presentation ownership

The active `TaskStatusSummary.active_error` record owns the primary launch
error for its matching session and stamp. A task-wide record remains visible
while a prior session is selected, but it owns session surfaces only when no
session is selected. A session-owned record owns surfaces only for its exact
session and stamp. The task Chat surface renders one persistent card from this
record.

The matching launch error suppresses these secondary presentations:

- The previous-agent-error notice.
- The standalone failed preparation row.
- The empty-turn warning.
- The generic failed-agent status row.
- The stopped-session recovery banner.
- A launch-error toast while the user views the affected task.

Only synthetic rows generated by the active launch failure are suppressed;
stale stamped or historical launch-only rows remain visible. An unrelated
historical error keeps its normal transcript presentation. A runtime error that
occurs after agent startup keeps the existing session
recovery surface.

The card shows a category title, a short cause, a no-change statement, and only
valid recovery actions. One disclosure shows bounded technical details. The
disclosure does not show raw Git output, credentials, or local file paths.

A failed recovery updates the same card. It does not add another banner or
toast. A successful recovery removes the card after launch succeeds.

## Responsive presentation

The task Chat surface is the entry point on desktop and phone. The card remains
inline because the error and its actions belong to the current task.

Desktop actions use one compact row. Phone actions use a vertical layout with
44-pixel touch targets. The existing mobile picker remains the branch-selection
surface for `base_branch_missing` and `default_branch_unresolved`.

The transcript owns vertical scrolling. Expanded technical details wrap inside
the card and do not create a second scroll region. Both layouts prevent
document-level horizontal overflow.

`useEnsureTaskSession` keeps one request latch after an error. Store updates and
loader identity changes do not start another request. The Retry control starts
the next request explicitly. A task change clears the prior task's error state.

## Verification

- Test relevant-PR selection, terminal/open precedence, lookup failures, and
  manual bypass.
- Test error projection limits, stamps, persistence, and recovery authorization.
- Test branch self-healing and mark-review-done terminal-step checks.
- Cover desktop and mobile recovery actions with the existing task Chat tests.

## Related decisions

- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)
