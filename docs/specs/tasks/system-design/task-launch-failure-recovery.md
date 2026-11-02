---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
created: 2026-08-24
updated: 2026-10-26
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
| REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003 | Quick Chat retention amendment, retained response contract, recovery and navigation |
| REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004 | Abandoned materialization recovery amendment |

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
The [abandoned materialization recovery amendment](#abandoned-materialization-recovery-amendment-2026-10-12-draft)
extends this rule to every early exit of the owner and defines when a later
start can elect a new owner.

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

## Quick Chat retention amendment (2026-10-02, draft)

REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003 maps to this section.

`httpStartQuickChat` currently rolls back its ephemeral task on any synchronous
`LaunchSession` error. Retain the task when a session has already been persisted.
Resolve that session by the newly created task's authoritative primary/session
records, never by another workspace's most recent session. Persist the safe
launch error through the existing task/session error machinery before returning
the failed response. Extend this endpoint's error envelope additively with
`task_id` and `session_id` when a retained session exists; retain non-2xx status
and a sanitized error. Extend the typed workspace API error handling so the
caller can reconcile these identities without parsing English error text.
No global HTTP-client success semantics change is needed.

Before session allocation, keep the existing cleanup of an unstarted task and
return an ordinary safe error. The local setup form retains its values and
inline error, permitting a new request. This distinguishes pre-session form
failure from an inspectable persisted conversation. Asynchronous failures already
have an accepted identity and must use existing durable error projection.

`useAgentSelection` must separate whether a response may activate a tab from
whether the persisted conversation exists. A stale request result upserts its
returned task/session without deleting it, selecting it, or reopening the
modal. The current request replaces the setup tab and opens the retained session
on either success or a typed retained-session error. WS-first and HTTP-first
orders converge on one identity. Explicit user deletion wins over late responses
through existing tombstones; a late upsert must not resurrect a deleted tab.
Unrelated WS updates never supersede request ownership. Review configuration-chat
start callers for shared reset behavior without extending ordinary-chat changes
to unrelated runtime contracts.

Render pre-session errors in `QuickChatSetup`, owned above the setup component so
rerenders do not discard the error or form choices. Persisted failures use
`QuickChatSessionView`, `TaskLaunchErrorProvider`, preparation progress, and the
existing error history/recovery surface. Prevent automatic resumption while an
active launch error exists, scoped to Quick Chat so ordinary stopped-session
resumption retains its current policy. Retry invokes the existing guarded
recovery action rather than POSTing a new quick chat. Boot/list/reconnect must
include failed ephemeral sessions; retain normal authorization and expiration.

Desktop retains the existing dialog and tab strip. Phone retains the shipped
full-height Quick Chat surface with safe-area padding, fixed navigation and
composer/footer, and the existing content scroll region. Place pre-session error
copy above the setup footer; reuse session error cards in persisted conversations.
This gives error inspection the space of the existing conversation rather than
opening a nested overlay. All new copy goes through all seven locale catalogs.

Test synchronous post-allocation failures, asynchronous failures, pre-allocation
errors, tab navigation during a delayed response, explicit deletion racing a late
response, reload/reconnect, and retry identity. Include mixed healthy/failed tabs.
Public recovery guidance belongs in `docs/public/tasks-and-workflows.md` when
implementation lands. See [delivery package](../../../plans/setup-recovery-ux/plan.md).

The [startup amendment](quick-chat-opening-composer.md#startup-acceptance-amendment)
moves ordinary creation acceptance before preparation. Its background path
preserves this retention contract.

### Quick Chat launch admission coverage

For a newly allocated ephemeral session, the orchestrator retains the allocated
identity through initial-prompt persistence and later launch admission failures.
The prepared-launch failure helper invokes the executor's existing conditional typed
failure transition, including sanitization, recovery actions and primary-aware
task reconciliation. Already-settled sessions retain their original error history;
intentional capacity deferral is not a launch failure. The HTTP handler returns
the retained identity only after the launch path has attempted this bookkeeping.

Automatic recovery considers task-owned errors and this session's active error.
A sibling session's error does not block this conversation. Rejecting a late
response for a tombstoned conversation preserves any pending selection request.

## Abandoned materialization recovery amendment (2026-10-12, draft)

REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004 maps to this section. Delivery:
[environment failure recovery package](../../../plans/upstream-environment-failure-recovery/plan.md).

### Problem

The first session of a task wins the workspace election in
`createTaskSessionWithWorkspaceBinding` and inserts a `creating` environment that
it owns through `materialization_session_id`. Only two exits of
`LaunchPreparedSession` release that claim: a `LaunchAgent` error and a
persistence error. Every earlier return, and every `startTask` return between
session preparation and launch, leaves the claim in place. A Kanban task keeps
its `CREATED` session, so the election returns `workspace_preparing` forever. A
session that reaches `FAILED` lets the next election mark the row `failed`, and
from then on every binding and launch returns `workspace_reuse_unsafe` until a
manual environment reset. Neither state has a physical workspace to protect.

### Claim release by the owner

The owner releases its own claim when it gives up. Release uses one conditional
repository write, `FailTaskEnvironmentMaterialization(sessionID)`,
that changes the row to `failed` and clears the claim only while the row is
still `creating` and still owned by that session. The existing
`markTaskEnvironmentMaterializationFailed` path and the inline `LaunchAgent`
error branch use the same write, because a full-row update could otherwise
overwrite a claim that another session re-elected meanwhile.

Release points:

- `LaunchPreparedSession` releases the session's claim on error only after the
  session snapshot read under its launch lock proves membership in the supplied
  task. A failed initial lookup or foreign-task request has no release authority.
  Release remains inside that lock. `ErrExecutionAlreadyRunning` and a
  superseded cancellable resume preserve the current claim. A committed
  re-election followed by a failed environment reload still releases the claim
  after session ownership has been established.
- The `startTask` early-return helper releases the claim of a session that this
  start created, for ordinary and ephemeral tasks alike, unless the error is
  `ErrCeilingLaunchDeferred`. A session reused by the start keeps its claim.

Release never changes session state. Kanban sessions keep their current
`CREATED` behavior, and Quick Chat keeps its existing typed failure transition.

### Abandoned claims

The election treats a `creating` claim as abandoned when the owner session is
missing or terminal, which is the current rule, or when the environment row was
last updated before the launch budget and the owner is not live. Lifecycle
preparation and each launch phase have separate bounds, so the budget is
`constants.SetupScriptTimeout` plus `constants.AgentLaunchTimeout` plus the
five-minute allowance that the orphaned-session reconciliation already uses
(30 minutes with default settings). The sum saturates at the maximum duration
for accepted extreme preparation settings; arithmetic overflow must never
shorten the owner's budget. A live owner is a session in `STARTING` or
`RUNNING` with an `executors_running` row. A launch that is still preparing after
the budget is treated as abandoned; the race rules below keep it from
publishing over the new owner.

The predicate runs only inside a binding transaction. A process restart, a
start path that never reached a release point, and any other cause that leaves
the owner silent are all detected the next time a start needs the environment.
No background sweep or configuration setting is added.

### Re-election

`reelectTaskEnvironmentMaterializationTx` hands an environment to a new owner in
the caller's transaction. Its conditional update accepts exactly two rows:

- a `failed` row with an empty claim and no physical evidence; or
- a `creating` row whose claim is still the abandoned owner read by the same
  transaction.

Physical evidence is a workspace path, container ID, sandbox ID, a
non-deleted `task_environment_repos` row, or retained Kubernetes metadata even
when control credentials were never recorded. Only a bare Kubernetes row with
empty metadata is eligible. The update sets `creating`, stores the
new owner, clears container and sandbox handles and their secret references, and
updates `updated_at`. It keeps the environment ID, so the shared group pointer,
sessions that reference the row, and recovery claims keep their identity.

Callers:

- The task-local binding re-elects instead of returning `workspace_reuse_unsafe`
  for an abandoned `creating` row and for an eligible `failed` row. An abandoned
  row with physical evidence keeps the current behavior: it becomes `failed` and
  the binding returns `workspace_reuse_unsafe`.
- The shared-group binding applies the same rule to the group's environment.
  The group pointer keeps naming the row, so other members observe
  `workspace_preparing` while the new owner prepares it.
- `LaunchPreparedSession` re-elects an eligible `failed` environment of the same
  task for the session that it launches. It uses a standalone transaction that
  takes the task cleanup barrier and the recovery-claim admission. An
  environment owned by another task, which is the `inherit_parent` case, stays
  fail-closed.

Every caller runs after `taskCleanupBarrierLocked` and
`recoveryclaim.EnsureAvailableTx`, so a deleting task or a held recovery claim
still fails the binding.

### Races

PostgreSQL release, binding, launch re-election, and finalization share the
transaction advisory namespace `task-environment-materialization:<environmentID>`.
Identity-only queries discover the lock target; they do not authorize a mutable
state transition. Acquire the owning task row before the environment lock, then
read current status and materializer in a separate READ COMMITTED statement.
Shared bindings acquire requester, group-owner, and environment-owner task rows
in sorted order before any environment lock. Recheck identity after waiting and
fail closed if it changed rather than extending the task lock set out of order.
SQLite retains its existing single-writer serialization. Cleanup and recovery
admission remain mandatory before a binding, re-election, or publication write.

Exactly one conditional update wins. A binding that loses returns the current
state error: `workspace_preparing` while the winner prepares. A launch that loses
reloads the row: a `creating` row owned by another session is awaited with
`waitForTaskEnvironmentReady`, a `ready` or `stopped` row is adopted, and any
other state fails closed.

A former owner that was re-elected away cannot change the row. Its failure write
matches no row. Before publishing readiness, `persistTaskEnvironment` refuses a
launch that started as the initial materializer when the fresh row is no longer
claimed by that session. `FinalizeTaskEnvironmentMaterialization` already
requires the current claim. The refused launch fails through the normal launch
failure path, and its partial runtime is stopped by the existing cleanup.

### Boundary with explicit retry

The failed state stays visible, and the `session.ensure` latch is unchanged.
Re-election happens only when a binding, start, or launch request arrives:
the user's Retry, a new session, `spawn_session_kandev`, or a workflow or
automation start. A `failed` environment with physical evidence still needs the
environment reset action.

### Reset guidance

The worktree store writes `task_environment_repos` rows when it creates a
worktree, before the launch finalizes. A launch that fails after that point, for
example in a setup script, leaves a `failed` row with physical evidence. That
row is not re-elected. The binding and the launch refuse it with
`models.ErrTaskEnvironmentResetRequired`, which wraps `ErrWorkspaceReuseUnsafe`
so existing `errors.Is` callers and the `workspace_reuse_unsafe` reason are
unchanged. Its message names the task environment reset action.
`spawn_session_kandev` keeps the reason and `recoverable: true` and changes its
retry guidance for this error to resetting the task environment.

Retaining such an environment while a replacement becomes canonical is not part
of this amendment. It needs a schema decision on how a task keeps an inactive
environment next to its active one, and on how that fits the existing
recovery-claim consumer admission and preserved-checkout inventory recovery.

### Interaction with workspace election waits

A binding that loses the initial election can wait for the elected environment
before retrying. When that environment becomes `failed` through a release point,
the wait ends with the existing refusal, and the next start re-elects it. A
second retry inside the same request is not part of this amendment.

### Verification

- Repository tests cover release by the owner only, abandonment by terminal,
  missing, and stale owners, protection of live and young owners, re-election
  of eligible `failed` rows, fail-closed rows with physical evidence, shared
  groups, the cleanup barrier and recovery claims, and single-winner races.
- Executor tests cover release on early launch errors, preservation on
  `ErrExecutionAlreadyRunning`, launch-time re-election, the lost-race outcomes,
  the `inherit_parent` refusal, and the refused publication by a former owner.
- Orchestrator tests cover release by `startTask` for created sessions and
  preservation on `ErrCeilingLaunchDeferred` and for reused sessions.
- MCP handler tests cover the reset guidance for a `failed` environment with
  physical evidence.
- PostgreSQL 16 and 18 run actual multi-connection lock-wait tests for release,
  local/shared binding, launch re-election, and finalization. State and owner
  replacements while blocked must be observed after admission. A refused former
  owner's finalization must roll back its inventory deletion and replacement.
  Shared-group ownership round trips through the real INTEGER schema on both
  versions. CI guards against skipped or unmatched PostgreSQL 18 tests.
