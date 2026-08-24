# 0051: PR Agent Notifications Extend Task PR Automation

**Status:** accepted
**Date:** 2026-07-23
**Area:** backend, frontend, workflow, GitHub, MCP

## Context

A PR Review task goes idle after its agent submits a review. A later review
request, merge, or close must be able to wake that same task. Kandev already
has task-level PR automation options, per-PR checkpoints, linked-PR records,
the one-minute PR watch poller, agent prompt queueing, and desktop/mobile
controls.

## Decision

Extend the existing GitHub task PR automation subsystem:

- add task-level `prompt_on_review_requested`, `prompt_on_merged`, and
  `prompt_on_closed` options beside auto-fix and auto-merge;
- keep transition and dedupe checkpoints per linked PR in
  `github_task_ci_pr_state`;
- continue using `github_pr_watches` and its one-minute batched poller as the
  source of PR facts;
- resolve the task workspace's authenticated GitHub login to classify review
  requests, and quietly rebind/reset baselines when that workspace identity
  changes;
- deliver prompts through the orchestrator's existing single CI automation pass
  (same goroutine and in-flight map as auto-fix and auto-merge);
- expose current-task get/update tools to task-mode MCP agents;
- render the same switches in the existing desktop popover and mobile drawer.

Schema evolution is additive. The existing table and API names retain `ci` for
compatibility even though their product meaning broadens to task PR automation.

The PR Review workflow enables these options through MCP after its initial
review. Review-requested is silently baselined and fires on a later
false-to-true request edge. Merged and closed fire once per observed terminal
entry. Stable states remain quiet, and an observed open state rearms a later
close. Lifecycle templates are visible server-owned factual notifications. They
report only the observed event and canonical PR URL; the task's workflow and
agent context decide what, if anything, to do next.

Lifecycle delivery reuses the existing task session selection and durable
message queue. It does not interrupt a busy turn, create a replacement session,
move the task to another workflow step, or reactivate an archived/deleted task.
Queue ownership remains explicit: agent-, workflow-, and server-owned entries
are reserved for backend dispatch paths, while clients may create, edit, and
remove only user-owned entries. Ordinary queued messages retain the existing
destructive dequeue behavior. Lifecycle entries use the same queue table and
FIFO ordering, but `ReserveHead` leaves their row durable until
`AcknowledgeByID` removes it after the executor accepts the prompt. A
per-session in-process dispatch guard prevents concurrent drains from
delivering the same reservation.

A passthrough session reserves a lifecycle head while its ready handler holds
the per-session guard, then defers dispatch until that guard has been released.
Before releasing that guard it claims the reservation's in-flight token; the
deferred lifecycle dispatcher consumes that preclaimed token rather than
claiming a second token later. This preserves the guard's serialization across
the deferral boundary, so a concurrent queue drain cannot duplicate the same
durable reservation.
It enters the ordinary lifecycle dispatcher rather than writing directly to
PTY stdin, so final active-task/session claiming, visible-message ordering,
retry, and acknowledgement semantics are identical to ACP delivery. Ordinary
passthrough queue entries retain their direct historical delivery path.

It runs turn-start/runtime/model preparation before a final active-task claim
under the session cancel guard and current queue token. That claim reports
claimed, busy, or inactive: `IDLE` and `WAITING_FOR_INPUT` primary sessions are
eligible for immediate delivery; busy and transient delivery failures retain a
durable coalesced retry. An active task with no currently promptable session
also retains the original event; only a missing or inactive task discards it.
If the guarded claim updates no row, Kandev re-reads the task/session in the
same transaction: an active session that became non-promptable is busy and
retained for retry, while a missing or archived task/session is inactive and
discarded. A successful claim reconciles the task to `IN_PROGRESS` and
publishes the session's `RUNNING` transition before creating visible work or
calling the executor.
If final task-level resolution selects another session, the event is requeued
to that newly selected session before the source reservation is acknowledged.
Active-task failures retain or coalesce the durable reservation; an inactive
task acknowledges and discards it. A reset or superseded token after claim, or
a failed dispatch, restores the claimed session's pre-claim state before
retrying. The visible message is recorded only after the final claim succeeds.
If that message cannot be persisted, Kandev restores the session, closes a turn
only when that dispatch created it, requeues the event, and does not call the
executor for that attempt. Task rollback is compare-and-set from
`IN_PROGRESS`, so it cannot overwrite a concurrent terminal transition or
archive. A pre-existing turn remains owned by its original dispatch.

This is at-least-once delivery without queue loss: a restart or crash before
acknowledgement redelivers the durable lifecycle row. A crash after the
external executor accepted the prompt but before acknowledgement can therefore
produce a duplicate. This trade-off uses the existing queue and schema rather
than adding a transactional outbox or provider acknowledgement protocol.
Reservation copies carry process-local delivery evidence, while the persisted
in-flight marker is stripped from every returned or requeued copy so a failed
post-restart delivery becomes a visible retry.
Delivery failures use the existing per-PR `last_error` and remain eligible for
retry. Review-watch `cleanup_policy=auto` retains tasks with enabled lifecycle
prompts, while `always` remains an explicit deletion override.

Lifecycle prompts are immutable, versioned server-owned templates. Their only
dynamic value is a validated canonical GitHub PR URL; they never interpolate PR
titles, branches, comments, review text, or caller-supplied prompt content.
They do not prescribe fetching, bookkeeping, archival, or another response.
The three lifecycle controls are boolean-only through HTTP and current-task MCP.
Legacy lifecycle-override columns remain additive migration compatibility data:
startup clears existing values and runtime ignores them.

Lifecycle queue acceptance is linearized with task archival. Both initial queue
acceptance and every retry insertion, plus the eventual prompt claim, require
an active task. If archival wins, no lifecycle checkpoint, queued message, or
prompt is created; a retry that encounters an inactive task is discarded even
if the task is later unarchived. If queue acceptance wins, normal archival
cancellation semantics cancel the accepted work; archival never leaves a
lifecycle prompt that can reactivate the task.

Lifecycle dispatch captures its rollback state before handing a prompt to the
PTY/executor. A failed executor handoff uses that captured state to restore and
retry the lifecycle row; acknowledgement happens only after the PTY accepts the
prompt. Archive and delete use privileged backend queue cleanup that also
purges reserved rows, rather than client ownership APIs. Each task queue has a
generation that advances when archival/deletion invalidates accepted work.
Accepted, reserved, or in-flight lifecycle work may finish its cancellation
path, but a stale generation cannot reinsert a retry or reappear after a later
unarchive.

When the task repository is supplied the production SQLite queue, its queue
purge and generation advance run inside the same archive/delete transaction as
the task mutation. The post-commit callback therefore does not re-purge that
same persistent queue. It mirrors cleanup only to a registered or fallback
ephemeral queue, and only after commit. Workspace cascade deletion captures
its tasks, purges each task queue and advances each generation in its single
transaction, then notifies that ephemeral mirror exactly once for every
captured task after commit.

Lifecycle and CI automation share one per-PR error field. The CI pass evaluates
lifecycle delivery before persisting any auto-fix or auto-merge error it
produced, so a successful lifecycle delivery cannot erase that same-pass CI
error.

## Consequences

- ~~PR automation has one task-level control plane for users and agents.~~
  **Superseded (2026-08-10):** the five automation switches
  (`auto_fix_enabled`, `auto_merge_enabled`, `prompt_on_review_requested`,
  `prompt_on_merged`, `prompt_on_closed`) are now scoped per linked PR —
  `github_task_pr_automation_options`, keyed `(task_id, repository_id,
  pr_number)` — because a task with multiple linked PRs (e.g. one repo's
  feature branch and its backport) needs independent auto-fix/auto-merge
  configuration per PR, and users editing one PR's tab in the multi-PR
  popover were confused to see the same values change on every other tab
  for the task. `auto_fix_prompt_override` and the resolved reviewer login
  remain the one task-level control plane described below; only the five
  boolean switches moved. MCP callers that omit PR identity keep today's
  fan-out-to-every-linked-PR behavior. See `docs/specs/ui/requirements/ci-pr-automation.md`
  and the GitLab MR automation follow-up for the mirrored change.
- Lifecycle evaluation runs inside the single CI automation pass — one goroutine
  and one in-flight map handle auto-fix, auto-merge, and lifecycle for a PR.
- Lifecycle rows remain durable until executor acceptance, so restart recovery
  favors at-least-once delivery over silent loss and may duplicate only across
  the external-acceptance/queue-acknowledgement crash window.
- Task archive/delete is a durable delivery boundary: privileged cleanup also
  purges reserved rows, and queue generations prevent stale accepted or
  in-flight lifecycle work from reappearing after unarchive.
- Passthrough lifecycle delivery follows the same guarded lifecycle dispatcher
  as ACP after ready-handler guard release; it cannot bypass final claim,
  durable retry, or acknowledgement rules.
- SQLite task mutations do persistent queue cleanup transactionally. Ephemeral
  queue mirrors are post-commit only, and workspace cascades notify each
  captured task once.
- Existing auto-fix, auto-merge, PR watch, and linked-PR behavior remains
  compatible.
- Review tasks remain retained under `cleanup_policy=auto` while PR-agent
  prompt options express ongoing intent.
- Reviewer-specific detection depends on the task workspace's authenticated
  user-level review request. Workspace identity changes require an atomic quiet
  rebaseline; ambient credentials must not influence this path, and team-level
  matching remains future work.
- Lifecycle prompt text is not configurable through the UI, HTTP, MCP, or
  storage; only its three task-level booleans are configurable. Merged and
  closed remain separate controls because users may subscribe to either terminal
  outcome even though both indicate that review work has ended.
- Event-specific workflow-step destinations and GitLab lifecycle parity remain
  independent follow-up features.

## Alternatives Considered

1. **A dedicated task cron or poller.** Rejected because the existing
   `github_pr_watches` loop already observes linked PRs and owns retry,
   cancellation, and shutdown.
2. **Generic task-bound external-event automations.** Rejected for this feature
   because task PR options, linked-PR identity, per-PR checkpoints, session
   delivery, and desktop/mobile controls already have one owner.
3. **Event-specific workflow-step transitions in the same change.** Deferred
   because today's task PR automation contract is boolean task-level options.
   Step selection needs its own workflow interaction and UI design.

## Amendment: message-queue-merge agent carve-out

**Date:** 2026-08-01

The `message.queue.merge` action (fold one queued message into the message above
it) is the single controlled exception to this ADR's rule that agent-,
workflow-, and server-owned entries are reserved for backend dispatch paths and
clients may only create, edit, and remove user-owned entries.

MergeIntoAbove permits folding an agent-owned source into the agent-owned target
above it only when both rows carry the same non-empty `sender_task_id` metadata,
and the caller must have session access. This preserves the reserved row's
provenance — the merged entry keeps the target's identity, sender kind, and
queued-by of `agent` — so the operation consolidates additive one-way prompts
from a single inter-task agent without letting clients move, delete, or reorder
agent-owned rows outside that exact fold. The caller identity is not the gate
for agent merges (matching how the entry's ownership belongs to the sending
task, not the UI user); the identical-sender-task gate is. All other merge paths
remain strictly user-owned: a user-kind source merges only into a user-kind
target whose `queued_by` equals the caller's identity.

Workflow- and server-owned rows remain entirely non-mergeable, and the reference-
overflow guard (a merge is rejected atomically when the combined entity
references exceed the per-message cap) applies to every sender kind.

## Supersession: session-owner queue cancellation

**Date:** 2026-08-03

[ADR-2026-08-03-separate-message-queue-provenance-cancellation-and-capacity](2026-08-03-separate-message-queue-provenance-cancellation-and-capacity.md)
supersedes this ADR only where it prohibited authorized clients from deleting
agent-, workflow-, or server-owned **pending** queue rows. Provenance still
governs editing and merging. Durable lifecycle rows already reserved in flight
remain hidden and non-cancellable, and all acknowledgement, retry, purge, and
generation guarantees in this ADR remain unchanged.
