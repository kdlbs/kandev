---
status: current
system: tasks
requirements:
  - REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001
---

# Subtask re-parenting by drag and drop system design

## Purpose and boundaries

This design owns canonical parent admission and sidebar candidate discovery for `Nest under` and drag-to-re-parent. It makes the
rendered task group the shared candidate source and carries the existing Office discriminator into
that view model so the UI can apply the same depth boundary as the backend. The complete unfiltered
task collection supplies hierarchy metadata for cycle and depth validation without adding hidden
tasks to the candidate list.

The canonical parent mutation, workspace-mode normalization, optimistic update, rollback, and
WebSocket reconciliation continues to use the canonical task event. Office project reassignment
now invokes that publisher, and the event carries the authoritative Office identity explicitly.
The persistence boundaries are described by [Detached Workspace Continuity](detached-workspace-continuity.md).
No endpoint, database column, feature flag, or user-facing copy is added.

## Requirement mapping

| Criteria                                            | Design section                          |
| --------------------------------------------------- | --------------------------------------- |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1 and .2 | Candidate source and propagation        |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.3 and .4 | Eligibility algorithm                   |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.5        | Responsive interaction and verification |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.6 and .10 | Transaction-bound hierarchy admission |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.7 and .8 | Request presence and snapshot writers |
| AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.9 | Creation, deletion, and compensation |

## Candidate source and propagation

The task HTTP DTO already exposes `is_from_office`. `toKanbanTask` preserves it as
`isFromOffice`, the Kanban snapshot task type owns that property, and both desktop and phone
sidebar projections preserve it on `TaskSwitcherItem`. This is a data projection only; Office
identity continues to come from the backend.

The mapped property remains optional on partial inputs: candidate checks treat an absent value as
non-Office, and `mergeTaskUpdate` retains the cached value when a lightweight event omits
`is_from_office`. Canonical `task.updated` events always carry an explicit true or false value, so
project assignment and removal can establish or clear the cached identity. The Office project
mutation publishes that canonical event after its task-row write. Active-board and multi-workflow
reconciliation uses the same absent-value fallback so an unrelated partial event cannot erase
Office identity from a full snapshot.

`GroupSection` already flattens the roots and descendants that it renders into `groupTasks` for
drag candidate calculation. That collection is the candidate source for both drag and context-menu
composition. Desktop and phone also pass their complete pre-view task list as hierarchy metadata.
The menu must not reread `kanbanMulti.snapshots[workflowId].tasks`: a present placeholder or
partially refreshed snapshot can contain fewer tasks than the reconciled group visible beside the
menu.

Rows receive stable getter functions for both collections. The owning switcher and group update
their refs after each committed render, so opening a menu or starting a drag reads current data
without making every memoized sibling row rerender when one task changes.

Candidate order follows rendered group order. Candidates remain scoped to the subject's workflow,
so mixed-workflow sidebar groups cannot create an invalid target.

## Eligibility algorithm

`computeNestCandidates` remains the shared pure function. Its input includes `id`, `parentTaskId`,
and `isFromOffice`. It applies these rules in order:

1. Resolve the subject from the complete hierarchy, falling back to the rendered group. With no
   subject, return no candidates.
2. Exclude the subject and its current parent in every mode.
3. Exclude every rendered task whose ancestor chain reaches the subject. Follow parent links in the
   complete hierarchy, including ancestors hidden by filtering, and track visited identifiers so
   corrupt hierarchy data cannot loop in the browser.
4. When neither the subject nor candidate is an Office task, require the candidate to be a root and
   return no candidates if the subject has a child. This preserves the one-level Kanban boundary.
5. When either endpoint is an Office task, allow a candidate at any depth and allow a subject that
   has children. This mirrors `validateReparentDepth`, which exempts the mutation when either
   endpoint is Office.

The browser filter is an affordance, not an authorization boundary. `PATCH /api/v1/tasks/:id`
continues to reject stale, archived, missing, cross-workspace, self, and cyclic targets.

## Mutation and failure flow

The context-menu selection and the drag nest zone call the existing `useNestTask` path with the
subject workflow and target identifier. The request uses the canonical task PATCH and the existing
optimistic snapshot update. A rejected request restores the prior tree and shows the existing
request error. A successful `task.updated` event reconciles the sidebar, board, and task detail.

A temporary multi-workflow placeholder cannot suppress a target that is already present in the
rendered group. A filtered-out task never becomes a candidate, but it remains available as ancestry
metadata. If the rendered group itself contains no eligible task, the menu shows its existing
disabled `No other tasks` row and drag shows no nest zone.

## Responsive interaction

Desktop and phone continue to use the same task-row context-menu and drag components. Desktop keeps
the anchored submenu; the phone task switcher keeps its inset sheet, touch drag sensor, menu event
containment, internal scroll owner, safe-area spacing, and current touch targets. Candidate data
changes do not add controls or alter geometry.

## Verification

Pure tests cover Office and Kanban eligibility, mixed Office/Kanban endpoints, filtered intermediate
ancestors, descendant exclusion, input order, and missing subjects. Projection and event tests prove
`is_from_office` reaches `TaskSwitcherItem`, survives omitted fields, and changes explicitly in both
directions. Component tests prove the context menu uses current rendered-group candidates without
rerendering unaffected rows, and that menu and drag return the same candidates.

The existing Kanban sidebar E2E remains a shallow-hierarchy regression. Focused desktop and phone
Office sidebar E2Es create a parent with a child and another target, select that target from `Nest
under`, and assert the deeper persisted and rendered hierarchy. The existing mobile re-parenting
E2E continues to cover the shared touch drag path.

## Implementation Plans

- [Fix Nest under candidates](../../../plans/fix-nest-under-candidates/plan.md)
- [Serialize validated task re-parenting](../../../plans/serialize-validated-task-reparenting/plan.md)

## Transaction-bound hierarchy admission

The serialization correction uses the following shared admission boundary. Existing
candidate and responsive behavior above remains current. The owning requirement's concurrency
clauses clarify canonical admission; they do not establish a universal Office tree constraint.

### Database boundary

Canonical parent validation and its commit run under workspace admission. Different
task-row locks alone cannot order A-under-B against B-under-A. `db.LockTaskRowInTx` protects task-owned
state, while `common/taskdependencies.AcquireMutationLock` is a process-local dependency-edge
lock; neither supplies this hierarchy boundary. `Repository.GetTask` reads `r.ro`, so calling it
from a mutation callback would also escape the transaction.

The task-specific admission interface is defined in the `task/repository/hierarchy` leaf and
aliased by `task/repository/hierarchy.go` to avoid the provider/store import cycle. Its
transaction-bound hierarchy reader exposes task lookup and the existing active-child predicate.
The admission capability supplies a locked parent preflight and a full-row update with explicit
optional parent intent. The reader
uses the caller's transaction on the writer connection, including the existing Office projection.
The update invokes a task-owned validator with the freshly read subject and the requested
parent, then commits through `updateTaskTx`. It does not expose a generic transaction, patch
language, nested repository object, or arbitrary write callback. Parent policy stays in the task
domain; SQL admission/commit and locks stay in the repository. Repository implementations that
cannot provide admission must fail closed for parent requests rather than silently use an
unserialized fallback. Test doubles may implement the same interface, but regression decisions
must execute against real stores.

Both the early preflight and the final mutation reserve the shared hierarchy boundary **before
the first ancestry/depth read**. Early validation still precedes repository entity preparation,
preserving `validateTaskUpdateReferences` and prior23's reject-before-entity-creation contract.
Release that read-only admission transaction before `prepareRepositoryReplacement`: preparation
can use find-or-create writes and provider validation, which must not run inside a held SQLite
writer transaction. Reacquire admission in the final update, reread the subject, and validate again;
the first check is not authority for the final write. A race after preparation can leave an
idempotent workspace repository entity under the existing preparation contract, but cannot change
the rejected task or its association set. No compensating deletion of shared repository entities
is introduced. F19 and the prior23 replacement `finalize`/`ReplaceTaskRepositories` boundary remain
unchanged.

### Dialects and lock order

Share the dialect primitive between task and Office stores through a narrow `internal/db` task
hierarchy helper. PostgreSQL reuses the existing workspace-row lock used by creation and workspace
deletion. Transactions use explicit READ COMMITTED: after waiting for admission, every hierarchy
query sees the prior holder's commit. Resolve workspace identifiers only as lock locators before
admission, then confirm them against current rows inside the transaction; those initial reads must
not be used for parent validity. Workspace identity is not a new task-move capability.

Acquire workspace rows in sorted identifier order, then sorted workflow-step rows, then task rows
in the established order. Take existing in-process step-arrival locks before opening transactions.
Do not take a parent task-row lock before a workflow-step lock, and do not introduce a second
advisory lock after ancestry reads. Canonical same-workspace parent admission needs one workspace;
Office's more permissive scalar surface and historical creation shapes may need subject and
target workspaces. Missing nonempty workspace rows retain the existing workspace-not-found
behavior. Empty-workspace/config shapes use one explicit transaction advisory key for the empty
workspace only, since there is no workspace row to lock; they do not lock every workspace.

SQLite must reserve its database writer with a harmless write as the first transactional operation
(for example `UPDATE tasks SET id = id WHERE 0`) before reading any hierarchy row. `BeginTx` is
deferred and `MaxOpenConns(1)` only serializes one handle; neither is sufficient across independent
handles. Keep SQLite's current pool/DSN and bounded busy behavior. Context cancellation or failure
to acquire admission aborts without a provisional success; do not retry a stale read snapshot or
change global pool settings. PostgreSQL workspace contention and SQLite database-wide writer
contention are intentional costs of this bounded correction.

### Validation and error contract

Refactor the existing canonical helpers to consume the transaction reader. Preserve self,
target existence, same-workspace, archived-target, cycle-before-depth ordering, and error wrapping.
Depth rejection still wraps both `ErrInvalidParent` and `ErrSubtaskDepthExceeded`; HTTP maps parent
errors to 400, missing subjects to 404, and WS retains its existing error envelope. An empty parent
remains allowed. Same-parent requests retain the current no-new-edge semantics.

Keep `parentChainWalkLimit` (1000) and add visited-identifier detection for malformed cycles that
do not reach the subject. Fail with `ErrInvalidParent` on a repeated ancestor or exhausted bound.
Keep the historical positively missing-ancestor-as-root behavior; propagate genuine storage or
cancellation failures rather than treating them as a missing root. Do not migrate or repair old
relationships. Keep `ListChildren`'s active/non-ephemeral/non-automation depth population and the
update exemption when **either** endpoint is Office. Creation retains its existing distinct
`validateSubtaskDepth` predicate based on the parent, including its current error shape and
historical workspace/archive eligibility; do not unify those policies by accident.

## Request presence and snapshot writers

Canonical parent intent is a `*string`: nil preserves the committed relationship, empty requests
un-nesting, and a nonempty value requests that parent. Carry presence through both REST
`PATCH /api/v1/tasks/:id` and registered WS `task.update`, including explicit-position updates.
Use the freshly read subject to determine whether the intent is an effective change, even when
the request's first snapshot had that parent already. Normalize from current hierarchy-owned
workspace state when it changes. Preserve materialized workspace identity, group identifier and
membership; do not reattach to the destination parent's workspace or stop/restart sessions.

The common `updateTaskTx` family defaults to preserving current `parent_id` for snapshot writers;
only admitted explicit intent can replace it. The two separate capacity/promotion UPDATEs must
also preserve it. At the task-row write boundary preserve the hierarchy-owned workspace mode on
all snapshots, including parent ABA (A → B → A), so a stale `inherit_parent` block cannot undo
committed `shared_group` normalization even when the final parent identifier equals the copied
identifier. Derive preservation from the current hierarchy-owned workspace state, never solely
from parent inequality.
For a materialized workspace, retain the current group identity while applying normalization;
other metadata keys retain existing replacement/merge semantics, including deletion by omission.
Do not globally turn full-row updates into request patches or freeze unrelated metadata. If the
post-preservation snapshot cannot be encoded, abort the transaction; writing an empty object would
discard hierarchy-owned workspace state and cannot count as a successful update.

`UpdateTaskExact` has no parent field. Its version, operation receipt replay, management fence,
labels and state contracts stay intact; exact workflow moves also cannot gain parent authority
from a copied snapshot. Preserve explicit-position behavior and the current deferred-launch policy
of each branch (ordinary update protects it; the explicit-position branch currently does not).
The task hierarchy seam does not change repository associations, runner admission, launch policy,
workflow moves, priority-only writes, or project classification policy.

### Participating operation inventory

| Family / current entry points | Required participation |
| --- | --- |
| `Service.UpdateTask`, canonical REST PATCH and WS `task.update`; `UpdateTaskPreservingDeferredLaunch` and `UpdateTaskWithExplicitPosition` branches | Locked early parent preflight; final transaction reader, explicit intent, admission and commit. Nil parent uses preserving snapshot behavior. |
| Four ordinary full-row variants: `UpdateTask`, `UpdateTaskPreservingDeferredLaunch`, `UpdateTaskIfWorkflowMatches`, `UpdateTaskWithExplicitPosition` | Parent preservation unless invoked through admitted canonical intent; preserve normalization on stale hierarchy snapshots. |
| `UpdateTaskExactOperation` and seven admission wrappers (`UpdateTaskWithWorkflowStepAdmission`, `...AndState`, `...Exact`, `...WithWorkflowChangeAdmissionAndState`, `...IfAtStep`, `...ForDeferredMove`, plus the internal admission function) | Common `updateTaskTx` preservation. Receipts, fences, expected-workflow/version checks and WIP semantics remain. |
| `MarkDeferredMoveAppliedForSession` | The remaining common full-row writer reserves workspace and current step before existing session/queue guards and task reads. Guard identity, marker idempotence and pending-move consumption remain atomic; current parent/workspace state survives. |
| `UpdateTaskIfWorkflowStepHasCapacity`, `PromoteQueuedTaskIfWorkflowStepHasCapacity` | Both separate full-row SQL shapes preserve parent and live normalization. |
| `CreateTask`, `CreateTaskIfWorkflowStepHasCapacity`, `CreateTaskWithWorkflowStepAdmission`; canonical `Service.CreateTask` and `CreateChildTask` | All three insert paths reserve hierarchy before reads/insertion; final validated creation-depth check uses current parent in that transaction. Locked service preflight precedes preparation. The named creation-parent validator carries only the predicate to the existing insert variants; no transaction is carried in context. Trusted raw fixture/import insertion joins serialization without adopting canonical parent policy. |
| `DetachTask` | Hierarchy admission before `loadDetachmentState`, then current sorted task/group ownership checks and atomic transfer. Root no-op stays idempotent. |
| `ReparentDirectChildren`, `ReparentDirectChildrenInWorkspace` | Shared admission before changing edges. Production no-cascade usage remains clear-to-root, workspace-scoped. Legacy arbitrary-target bulk reparent remains trusted policy, not a new canonical API. |
| `RestoreTaskParentIfUnchanged` | Shared admission before expected-parent comparison, target/ancestry/depth validation before restoration, and conditional mode rollback. Never restore an invalid edge or overwrite a newer parent. |
| `ArchiveTask`, `ArchiveTaskExact`, `ArchiveTaskIfAutoArchiveEligible`, `ArchiveTaskIfActiveWithVacatedStep` (also used by `ArchiveTaskIfActive`); `UnarchiveTask` and `UnarchiveTaskByCascade` | Shared hierarchy admission before step/task locks so canonical archive eligibility and child-depth reads cannot straddle these writers. No new archive/unarchive depth policy. |
| `DeleteTaskWithVacatedStep` (also `DeleteTask`) | Shared admission before step locks, then check all structural children before deletion. Refuse a late child instead of deleting its parent. Existing service tree cleanup/retry retains cascade choice. |
| Workspace cascade in `workspace.go` | Reuse its workspace-first boundary; SQLite writer reservation must precede task inventory. No cascade redesign. |
| Office `DashboardService.UpdateTaskParentID` / Office `Repository.UpdateTaskParentID` | Acquire shared admission and recheck existence inside the scalar write transaction. On PostgreSQL lock the subject row before checking metadata with Go `json.Valid`, then bind that guard into the native normalization query; do not add a SQL/JSON version dependency or cast malformed historical metadata. Preserve direct-self/existence service checks and atomic normalization, without adding deeper cycle, archive, workspace, or depth rules. Empty dashboard parent still calls canonical detach. |

Inventory refers to mutation families rather than only task IDs: request handlers, workflow and
orchestrator callers that reuse these methods inherit preservation. During implementation audit
every `parent_id` assignment and all `updateTaskTx` callers against this table; newly discovered
unparticipating production writers must be recorded, not assumed covered.

## Creation, deletion, and compensation

Recheck canonical creation depth under the insertion transaction's hierarchy reservation;
preparation's original check cannot authorize insertion after the parent moved. Parent deletion
and insertion must share admission so the missing-parent branch cannot commit a dangling edge.
Do not add stricter creation workspace/archive/Office classification rules than its existing
predicate. Creation attachment and detachment retain
[Detached Workspace Continuity](detached-workspace-continuity.md), particularly its atomic
ownership and overlap contract.

No-cascade deletion still promotes children to roots and normalizes inherited workspace modes;
cascade still removes its chosen tree deepest-first. Existing cleanup runs outside these short
database transactions. At final row deletion, query **all** structural children, including archived,
ephemeral and automation rows, rather than the depth/candidate list. If a concurrent relationship
appeared after the service inventory or promotion pass, return a typed hierarchy conflict (HTTP
409, ordinary WS error) and retain the parent. A later retry uses the existing selected cascade
choice. Do not silently sweep/delete a late child, turn cascade into promotion, or publish a
parent-deleted success. Partial earlier deletions retain existing destructive retry semantics.

Compensation holds the same admission while comparing expected parent and validating the restored
edge. A parent different from the expected value, missing/archived parent, cycle, or depth violation blocks restoration; report
the compensation failure without changing parent/mode. Service compensation publication remains
after an actual successful repository restore. This is a structural rollback guard, not a new
task-retirement or cleanup ownership framework.

### Exact residual scope

Office scalar parent updates deliberately permit deeper cycles; a later Office update can create
a cycle after a valid canonical mutation. Arbitrary-target legacy bulk reparent and direct SQL,
historical imports/fixtures that bypass validated creation are also not cycle-free guarantees.
Project assignment/removal can change Office classification after a canonical commit; neither
that policy nor revival of archived children is newly constrained. Bounded canonical reads must
handle such data safely.

Cascade inventory deliberately excludes ephemeral and automation-origin children. Final ordinary
deletion still refuses their structural references, retaining the parent with a conflict until the
owning child lifecycle removes the relation. Refresh alone does not guarantee that a cascade retry
can finish. This package does not broaden ordinary cascade ownership to delete those excluded
children; no-cascade promotion retains its existing workspace-scoped structural update.

Quick-chat expiry (`DeleteExpiredQuickChatTask`) and profile-owned ephemeral deletion
(`DeleteEphemeralTasksByAgentProfile` in `session.go`) are legacy specialized deletion paths;
they do not receive a broad cascade redesign in this package. Their ability to remove a task with
unusual incoming parent references remains an explicit residual risk, outside the ordinary
parent-deletion guarantee. Guarded exact retirement already excludes relation-bearing candidates;
its separate association/F19 boundary stays unchanged. Workspace deletion covers same-workspace
trees; historical Office cross-workspace links can become dangling on a workspace cascade and
remain outside the canonical same-workspace contract. These exclusions must appear in delivery
evidence rather than being described as universally safe hierarchy storage.

## Commit evidence and verification for serialization

No task success response, `task.updated`/`task.state_changed`, or step dispatch is produced for a
failed parent mutation. Keep existing after-commit publication and projection reloads. A reload
may observe a later committed update; do not add event revision ordering or claim a globally
consistent snapshot. Repository-set replacement remains its existing subsequent transaction.

Permanent evidence uses real service and private SQLite data, two independent writer handles on
one database, and real PostgreSQL connections with worker-PID-scoped observed lock waits. Include
two/three-node competing moves (Office-shaped canonical tasks exercise cycle checks independently
of the depth guard), child-create versus move depth races, target archive/delete, detach and
rollback overlap, omitted-parent stale title/metadata/workflow snapshots, exact operation replay,
position and unchanged sessions/groups/fields. Register real REST routes and the WS dispatcher,
submit parent mutations through them, and assert committed rows, typed error responses and absence
of loser success events. Coordination may block a real SQL boundary or delegate actual store calls;
it must not fabricate validation decisions or race mutable service fields.

Mobile parity assessment: the correction is backend/shared data only. Existing desktop and phone
controls, optimistic rollback and events are reused. No UI/copy/layout change, browser/build or
full E2E is required. Public docs audit finds existing task actions and WS mutation documentation;
at implementation add at most a short existing-guide paragraph for concurrent-change rejection
and refresh/retry if the typed deletion conflict requires it.
