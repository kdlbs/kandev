---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
created: 2026-08-24
updated: 2026-09-14
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
| REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001 | PR gate, initial prompt admission, error projection, recovery actions |
| REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002 | Error scope, durable session history, shared task surface |

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
  relaunches. The [qualified PR base amendment](../../workspaces/system-design/worktree-base-refresh.md#recovery-and-persistence)
  rejects this action for an explicit cross-repository binding before writes.
  The action does not authorize replacing that binding with the fork default.
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

### Asynchronous startup amendment

Implemented by `Executor.handleAgentProcessStartFailure` and
`Service.handleAgentStartFailed` must preserve the launch phase through the
existing terminal path. After provider-specific auth/runtime handling and
current-execution guards, persist one typed `last_agent_error` using the
existing launch classification and stamp model. Do not first publish a raw
`startErr.Error()` failure and then race to replace it with a safe record.

Keep provider-specific recovery routes authoritative. Generic bootstrap errors
use `generic_launch_failure` with safe structured operation/reason details.
Known contribution access, transport, and destination reasons must remain
distinguishable in the safe projection. Unknown errors use a neutral summary;
do not guess their cause from an English substring in the frontend.

Add bounded optional operation/attempt correlation fields to the existing
error projection and recovery error envelope where required. The current
execution guard and compare-and-set persistence must reject a successor
execution race. Initial failure, session state, transcript marker, HTTP/boot
projection, and live status summary carry the same stamp. A failed resume and
its fallback restore retain separate sanitized causes for that attempt.
Old records without the new fields remain readable with safe generic copy.

The optional fields are `phase` (`bootstrap`), `execution_id`, `attempt_id`,
and `causes`. An attempt ID identifies one resume and its optional fallback;
each explicit retry gets a new ID. The server returns the attempt ID and error
stamp in request errors and synthetic messages. IDs are bounded to 256 bytes.
`causes` has at most two entries with `operation` (`resume` or
`restore_workspace`), an allowlisted `code`, and a sanitized `detail` limited
to 1024 UTF-8 bytes. Cause details and legacy details together stay within the
existing 4096-byte details budget. Malformed optional fields are ignored.

Safe contribution reason codes are `authentication_required`,
`permission_denied`, `destination_invalid`, `source_branch_missing`,
`transport_unavailable`, `timeout`, and `unknown`. Assign specific codes only
from typed evidence at the operation boundary. The preflight's history-only
reason is an admission result, not a durable agent error. Raw Git output and
nested transport strings are not persisted as user-facing details.

The [agent recovery design](../../agents/system-design/session-recovery-failures.md)
owns the single recovery card and request-state composition. This extends the
existing launch-card ownership to asynchronous bootstrap failures; it does not
turn post-start provider errors into launch errors.

### Existing prompt contract

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

## Error scope (September 14 amendment)

Implementation is complete in the [error scope package](../../../plans/error-scope-and-history/plan.md).
This section supersedes the earlier Chat-only task error surface.
Tasks own durable error records and task-shell projections. Agents own session recovery eligibility and entry presentation.
Workspace services retain authority over resource validity. This change does not add a workspace-wide alert bus.

Add an optional `scope` discriminator, `session` or `task`, to existing normalized error metadata and wire types.
The producer selects scope from operation ownership, never from English error text or a count of failed sessions.
Session bootstrap, provider, turn, and resume failures default to session scope.
Task admission and preparation failures before session creation remain task scope.
A shared workspace or repository preparation failure uses task scope only when its owner establishes shared impact.
Preserve `task_repository_id` and the originating session correlation when available.
A repository ID alone does not prove shared impact. Unknown legacy session records remain session-scoped.
Legacy task metadata without scope remains task-scoped. Reject or safely ignore malformed optional scope values.

Continue storing current session errors in `task_sessions.metadata.last_agent_error`.
Store the current shared failure in `tasks.metadata.last_launch_error`, with the existing stamp fencing and recovery validation.
Do not create the same active failure in both stores. An origin session can retain a non-actionable historical reference.
Recovery lookup uses scope to choose the source record. The originating session ID is correlation, not authority to select session metadata.
Legacy requests retain their existing lookup rules. Scoped requests must pass the same task, repository, and stamp guards.
The existing single current task error contract remains. This package does not add a multi-incident queue.

Expose optional `TaskStatusSummary.task_error` independently of `active_error`.
The projector already tracks `taskError` separately from its session error map.
Use that source for the shared field, while preserving `active_error` for existing aggregate consumers.
Boot payloads, HTTP reads, summary persistence, rebuilds, equality checks, and WebSocket replacements must carry the same field.
A newer session failure cannot displace `task_error`. Session success cannot clear task metadata.
Clear the shared record only through its matching successful recovery or an authoritative resource-resolution event.
For old payloads, clients can use an explicitly task-scoped or sessionless `active_error` as a compatibility fallback.
An old aggregate cannot reconstruct a shared error that it never carried.

## Durable session history

Use existing persisted session messages as the history ledger. No new history table or frontend error store is required.
Persist one sanitized error marker for each accepted session failure stamp before publishing the terminal state.
The marker carries session, execution, attempt, stamp, occurrence time, phase, and bounded cause data when available.
Reuse the current failure path and message storage rather than writing a second copy from the browser.
Extend the fenced persistence boundary so marker creation and active error admission cannot diverge under duplicate or late delivery.
An operation that loses its execution or stamp fence must not insert a failure for the successor.
Use deterministic message identity derived from the owning session and failure stamp for idempotent insertion.

Successful recovery clears matching active metadata and retains `recovery_resolved_at` under existing rules.
It must not delete the error marker. Client presentation reads resolution separately from historical content.
Workspace-only restoration does not resolve the stopped agent. New agent output does not resolve shared workspace state.
A failed retry has a new attempt/stamp and a new entry. Request feedback for the same stamp updates only that entry.
Old records with markers remain readable. Metadata-only legacy failures can use the agent design's provisional entry while unresolved.
No migration invents historical errors after metadata has already been cleared.

## Shared task surface

Mount one shared alert in the task shell below the task title/header and above session and content tabs.
It is outside Dockview and mobile tab content, including maximized panels and tasks without sessions.
Audit `task-page-content.tsx`, `task-layout.tsx`, preview composition, and dedicated phone/tablet layouts to choose the common host.
Extract a proposed `TaskSharedError` view instead of mounting `TaskChatLaunchError` inside every chat.
The compact strip shows the safe cause, affected resource label, and a Recovery details control.
A desktop dialog exposes existing valid task recovery actions. The phone equivalent uses an inset Drawer.
Use the existing branch picker and confirmation controls inside their established flows.
The alert has no dismiss action that clears unresolved state. Details can close without resolving the error.
Preserve focus return and use an accessible status announcement once for a new stamp.

A shared alert and an unrelated session error can coexist. They represent separate failures.
The same stamp cannot mount recovery controls in both places.
Session error filtering must not hide the durable historical marker merely because a shared alert is present.
Session switching, Plan/PR navigation, and message pagination do not affect the shared alert.

## Responsive behavior

Phone entry: the task header above the selected session/content view.
Exemplars: `mobile/session-mobile-layout.tsx` for task chrome and
`mobile/mobile-picker-sheet.tsx` for the inset drawer interaction.
The strip provides summary and details entry. The drawer provides cause, resource, then recovery actions.
This occasional multi-action recovery fits a temporary drawer, while the transcript remains the main reading surface.
Use one internal drawer body scroller for long shared details, dynamic viewport bounds, and bottom safe-area clearance.
Phone targets measure at least 44 pixels. Fine-pointer controls retain 28-pixel sizing.
Session entry details remain inline in the transcript and never gain a nested details scroller.
Shared view models, action guards, stamps, and request state serve both presentations.

## Verification

- Test relevant-PR selection, terminal/open precedence, lookup failures, and
  manual bypass.
- Test error projection limits, stamps, persistence, and recovery authorization.
- Test branch self-healing and mark-review-done terminal-step checks.
- Cover desktop and mobile recovery actions with the existing task Chat tests.

## Related decisions

- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)

- [Error scope and history decision](../../../decisions/2026-09-14-error-scope-and-history.md)
