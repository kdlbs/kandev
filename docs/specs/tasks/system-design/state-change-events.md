---
status: draft
system: tasks
requirements:
  - REQ-TASKS-STATE-CHANGE-EVENTS-001
created: 2026-11-11
owners:
  - kandev
---

# Task State-Change Events System Design

## Purpose and boundaries

The task system owns the canonical state-change publication boundary. Office
owns status normalization, approval policy, comments, holds, and reactivity.
Office continues to apply those policies before it publishes the task change.

This design restores the existing task publisher rather than adding a bus
publisher to Office repositories. It adds no schema or event subject.
The repair sequence is in the
[Office task-state event plan](../../../plans/office-task-state-events/plan.md).

## Requirement mapping

| Criteria under REQ-TASKS-STATE-CHANGE-EVENTS-001 | Design section |
| --- | --- |
| .1, .2, .3, .9 | Commit evidence and canonical publication |
| .4, .7 | Dashboard flow and activity ownership |
| .5, .6 | Tree flow |
| .8 | Subscriber compatibility |
| .10 | Failure and recovery |

## Current boundaries

- `office/dashboard.UpdateTaskStatus` and `UpdateTaskStatusAsAgent` share
  `persistTaskStatus`. Their writer is the raw task repository, wired in
  `backendapp.buildOfficeDashboardService`.
- `shared.TaskStateWriter` exposes unguarded, step-guarded, and
  step-and-state-guarded writes. The repository keeps completion checks inside
  those transactions.
- `task/service.PublishTaskStateChanged` creates the canonical rich event.
  `service_events.go` snapshots requests and uses a reentrant per-task FIFO.
- `task/service.TaskStateActivityLogger` calls
  `dashboard.LogTaskStateChange` before canonical publication. The dashboard
  separately records activity for its own status route.
- `orchestrator.markOfficeTaskCompletedForTerminalStep` calls the dashboard,
  then publishes another task update and state event. It also directly invokes
  parent completion. This path currently compensates for the dashboard gap.
- `office/service.CancelTaskTree` bypasses the task writer with
  `office/repository/sqlite.BulkUpdateTaskState`. Restoration uses the raw task
  writer. Neither path publishes canonical state events.

## Commit evidence and canonical publication

Add a receipt-returning task repository entry point in
`internal/task/repository/sqlite/task_state_commit.go`. Keep existing repository
signatures available for unrelated callers.

The proposed task-owned models in `internal/task/models/task_state_change.go`
are `TaskStateWrite` and `TaskStateChange`:

- A write carries the target state and optional expected workflow step and
  expected state. A missing step guard differs from an explicit empty step ID.
- A change carries the authoritative previous state and the resulting task
  snapshot, including the committed state, placement, and update timestamp.
- The result distinguishes a failed guard, a matched unchanged state, and a
  changed state. Guard matching alone does not mean a state transition occurred.

Select the previous state under the writer transaction and existing row lock.
Apply `guardTaskCompletionTransitionTx` in the same transaction. Preserve
SQLite writer admission and PostgreSQL row locking. Reuse `taskSelectColumns`
and `scanSingleTask` through the transaction for the result snapshot.
Release database resources before publication. Never derive `old_state` from
the dashboard's earlier best-effort read or reload `new_state` after another
writer can replace it.

Migrate the Office `shared.TaskStateWriter` seam to consume these receipts.
Automatic completion retains both caller-observed guards. Manual completion
retains its approval-step guard. The change does not add an archive restriction
to paths that lack one today.

Add `task/service.PublishCommittedTaskStateChange` to reuse the existing
canonical serializer and FIFO. It accepts the immutable receipt and an internal
activity-handled option. That option suppresses only the duplicate activity
write. It never suppresses the canonical event and never enters its payload.

Require the canonical publisher before admitting an Office state mutation.
Wire it in `backendapp/main.go` for both dashboard and tree services. Narrow
test constructions must provide the same seam or a recording test publisher.
There is no raw-write fallback after a missing production dependency.

## Dashboard flow and activity ownership

1. Normalize status and apply the existing approval gate.
2. Commit with the correct guards and obtain the change receipt.
3. Create an attached comment through the existing path.
4. For a changed state, record dashboard activity once with its existing actor
   and run attribution.
5. Publish the canonical change with activity already handled. Retain the
   existing Office status event and canonical `task.updated` refresh.
6. Apply existing reactivity and decision-supersession behavior. Use the receipt's
   previous state for transition-based side effects too.
7. Return the existing gate result, including approvals-pending after a
   committed `REVIEW` redirect.

The activity row precedes every refresh event that can trigger a detail read.
`SuppressStatusActivity` becomes obsolete when the orchestrator stops publishing
the same dashboard transition. Remove the field and its callers together.
Status requests that add a comment without changing state retain the comment
and explicit resume behavior. They do not fabricate a state transition.

Automatic Office completion relies on the existing transactional state-and-step
guard for concurrent request admission. Remove the compensating publications
and direct parent invocation from `markOfficeTaskCompletedForTerminalStep`.
Remove its Office completion mutex once the guarded receipt owns admission.
Keep the initial terminal/blocked/step checks and existing typed errors.
No completion mutex or database lock can span a synchronous subscriber call.
Unrelated `childCompletionOperationLock` behavior remains.

## Tree flow

Use a task-owned batch cancellation writer returning one receipt per changed
member. It commits all selected state changes and checkout releases in one
transaction. Reuse the task row projection instead of adding another serializer
to the Office repository. Skip unchanged `CANCELLED` states, while retaining
checkout cleanup for every selected member.

Keep Office hold creation, captured membership, skip markers, execution
cancellation, and run cancellation under their existing owners. Replace the
sole raw bulk-state call and retire its unused Office repository helper.
This repair does not redesign graph enumeration or make the whole hold
lifecycle transactional.

After the cancellation batch commits, retain its receipts through any later
run-cleanup error. Publish them before the tree-hold notification. Every
subscriber must see all members already cancelled, so an in-tree dependent
cannot look like live work held by an earlier member notification.

Restoration uses the receipt-returning single-task writer for each eligible
hold member. Publish each committed transition even if a later restore fails.
Release the hold only after all required restores succeed. Keep the
`already_cancelled` exclusion. Pause and resume never use the state publisher.

## Subscriber compatibility

Keep existing subscriber policies and event payloads:

| Consumer | Required evidence |
| --- | --- |
| Orchestrator dependency handling | Office success starts a Kanban dependent only after all blockers succeed and only through its existing deferred-launch claim. |
| Orchestrator parent handling | Terminal child events retain the existing operation key and all-children predicate. |
| Scheduler committed completion plus status reactivity | Keep the shared blocker digest and parent-wave identities. Both delivery orders produce one run per existing occurrence. |
| Office handoff completion | One completion of the matching handoff, including canonical duplicate delivery. |
| Coordinator wake recorder | Keep the rich payload's `state` field, which `onTaskStateChanged` reads. One completion episode is recorded. |
| Plugin event bridge | Forward the same canonical subject and payload once per producer transition. |

PR [#5277](https://github.com/kdlbs/kandev/pull/5277) adds the blocker-failure
policy for issue #4548. At the planning snapshot, it is open and absent from
`main`. Refresh its status and head before implementation and final validation.

After both changes coexist, use its canonical state subscriber as the sole
blocker-failure trigger. Remove its direct status-reactivity and tree-hold
fallback triggers together. Retain its occurrence key, recipient policy, and
success-only classifier. Do not remove fallback coverage before every relevant
Office writer publishes canonical changes.

Test `FAILED -> CANCELLED`, repeated cancellation, retry then re-failure,
distinct blockers, and mixed in-tree/outside dependents. Checkout cleanup and
activity must not create another failure occurrence. This task does not choose
or change #4548's product policy.

## Failure and recovery

Use the existing bounded publication context detached from caller cancellation.
Publication errors remain logged through the task publisher. A committed write
must not become an apparent rollback merely because a later notification fails.

No-op and rejected writes produce no canonical change. A failed batch
transaction produces no receipts or member events. A later cleanup error does
not discard successful receipts. Test these boundaries with controlled errors
and channels instead of timing sleeps.

This design adds no outbox, restart replay guarantee, or global ordering across
different tasks. Existing reconciliation and transport behavior remain.

## Verification and related contracts

Use repository tests for authoritative receipts and guard races. Use real
task/Office services with a shared event bus for publication and consumer tests.
Add an isolated UI-to-scheduler E2E case for the reported dependency chain.
The [work orders](../../../plans/office-task-state-events/plan.md#work-orders)
name test files and exact commands.

The frontend already consumes the canonical event. There are no rendered UI,
localization, mobile composition, or touch changes. The same event serves both
desktop and mobile clients. An additional mobile layout scenario is unnecessary
for this backend event repair.

Related contracts:

- [Task dependencies](task-dependencies.md).
- [Office scheduler](../../office/system-design/scheduler-01.md).
- [Office task policy](../../office/system-design/tasks-01.md).
- [Task-before-session publication decision](../../../decisions/2026-07-30-runtime-task-state-before-running-event.md).

The task publisher already owns this boundary. The requirement and this design
preserve the repair rationale without a new architecture decision.
