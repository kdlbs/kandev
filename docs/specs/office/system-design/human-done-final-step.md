---
status: current
system: office
requirements:
  - REQ-OFFICE-HUMAN-DONE-001
  - REQ-OFFICE-HUMAN-DONE-002
---

# Office Human Done System Design

## Purpose and boundaries

Two existing paths meet here:

- `PATCH /api/v1/office/tasks/:id` with `status: "done"`:
  `dashboard.Handler.applyTaskMutations` -> `DashboardService.UpdateTaskStatus`
  -> `applyApprovalGate` (`office/dashboard/service_tasks.go`). On a step that
  is not last by position, the gate rewrites the state to `REVIEW`, and the rest
  of `UpdateTaskStatus` persists it, publishes `office.task.status_changed` and
  `task.updated`, and runs reactivity. Reactivity's `in_review` branch
  (`office/scheduler/reactivity.go` `cascadeReviewRequested`) wakes every
  workflow-scoped reviewer and approver. The handler then returns 409
  `reason: "workflow_step"`.
- `POST /api/v1/tasks/:id/move`: `task/handlers.httpMoveTask` ->
  `task/service.MoveTaskWithOptions`. `syncTaskStateForWorkflowMove` sets
  `COMPLETED` when the target step is terminal (`wfmodels.IsTerminalStep`: no
  next step and `CompleteTaskOnEnter`). The state is written in the move's
  admission transaction, where the completion gate
  (`guardTaskCompletionTransitionTx`) also runs. The orchestrator's Office seam
  (`markOfficeTaskCompletedForTerminalStep`) later skips the task because it is
  already complete, so Office approvers are never consulted.

The design adds a pre-write approver guard to the human move, then routes a
human Office Done request on a non-final step through that move. The agent
and engine paths remain unchanged. The selected atomic amendment below also
protects human final-step Done and same-step completion; it is not implemented
yet.

## Current preflight and transport

The current `OfficeApprovalGuard` in task/service reads pending names through
backendapp before repository admission. `MoveTaskWithOptions` checks human
actor, pre-read Office membership and speculative completed state. This leaves
a read/write window and does not yet know actual WIP admission. It is not an
implementation of the concurrent guarantee. The selected amendment below
replaces production eligibility with a transaction-local check.

The existing canonical slate includes per-task seats across the destination
workflow and templates at the destination step, including tasks with no current
step. Existing per-task precedence and active approval semantics remain.
`ApprovalsPendingMoveError` maps through `handleSelectedMoveError` and shared
`handleNotFound` to409 `task_approvals_pending` and named pending details. WS
task.move uses CONFLICT with the same details. These transport contracts and
the genuine human actor remain; names move outside the transaction.

## Human Done on a non-final step (REQ-OFFICE-HUMAN-DONE-001)

`dashboard.Handler.applyTaskMutations` routes a status change with no agent
caller (`agentIDFromCtx` empty) to `DashboardService.UpdateTaskStatusAsHuman`.
Agent requests, the runtime agent action and the orchestrator seam keep calling
`UpdateTaskStatus`, so they keep today's gate.

`dashboard` gains one seam, set from `backendapp`:

```go
type TaskFinalStepMover interface {
    MoveTaskToFinalStep(ctx context.Context, taskID string) (*FinalStepMoveResult, error)
}

type FinalStepMoveResult struct {
    State    string // persisted task state after the move (set by the mover)
    Status   string // Office status, filled by UpdateTaskStatusAsHuman
    StepID   string
    StepName string
}
```

`UpdateTaskStatusAsHuman` calls the mover only when the requested state is
`COMPLETED`, the mover is wired, and `IsTaskWorkflowStepTerminal` reports
`hasStep && !terminal`. The current implementation otherwise delegates to `UpdateTaskStatus`; the
selected amendment replaces that human Done delegation with the transaction
completer. On the
move path it writes no state, activity or event of its own: the move publishes
`task.updated`, `task.state_changed` and `task.moved`, and the orchestrator
owns completion side effects. A step lookup error aborts the request.

The `backendapp` adapter `officeFinalStepMoverAdapter` reads the task, takes
the step with the highest position from `ListStepsByWorkflow` (a position tie
goes to the larger step ID), and calls
`MoveTaskWithOptions` with `StepHistoryActor: human` and
`AllowActivePrimarySession: true`, matching the human actor and session options
of `httpMoveTask`. It also carries `ExpectedWorkflowID` from the task read:
the existing write-time workflow guard refuses a concurrent reassignment
instead of moving the task back to its stale workflow. That refusal uses
`reason: "workflow_move"` and `code: "workflow_change_conflict"`.
It classifies refusals into `*dashboard.FinalStepMoveRefusedError{Reason, Code, Pending}`
with the task handlers' exported `CompletionGateCode` and `MoveConflictCode`:

| Move error | `reason` | Extra fields |
| --- | --- | --- |
| `*ApprovalsPendingMoveError` | `approvals` | `pending_approvers` |
| completion-gate errors | `completion_gate` | `code` |
| move conflicts | `workflow_move` | `code` |

Any other error passes through unchanged and keeps the existing 400 response.

`respondStatusUpdateError` returns 409 with `reason`, `error` and the extra
fields, and **no `status` field**: the absence of `status` tells the client
that nothing changed.

On success the PATCH returns 200
`{ok: true, status, queued, workflow_step: {id, name}}`. `queued` is true when
the final step's WIP limit queued the move (`QueuedForStepID` is the final
step). The task is placed in the final step's queued band
(`WIPAdmitted=false`) while keeping its prior state until admission. Other
PATCH requests keep returning `{ok: true}`.

The step read and the move are not one transaction. If another actor moves the
task in between, `MoveTaskWithOptions` revalidates the task and either applies
the move to the final step or refuses it like any other move.

## Frontend

`lib/api/domains/office-status-gate.ts`:

- `updateTaskStatusOrTranslateGate` returns `{status, workflowStepName?}` from
  the PATCH body, defaulting `status` to the requested value.
- A 409 with a `status` field keeps throwing `ApprovalGateError` (agent-style
  redirect for agent requests; the human final-step refusal has no
  redirect status).
- A 409 without `status` throws `StatusChangeRefusedError` (no redirect) with
  a translated message: pending approvers through
  `formatPendingApproversMessage`, `completion_gate` through
  `task:cannotMarkDoneCompletionCriteria`, `workflow_move` through
  `getTaskMoveErrorMessage` with `task:cannotMarkDoneMoveRefused` as the
  fallback.

`app/office/tasks/use-board-drag.ts` `applyStatusDrop` settles the card on the
returned status. `hooks/use-optimistic-task-mutation.ts` settles a status
mutation on the status its API call returns, which covers the status picker
(`components/task/simple/components/status-picker.tsx`). A
`StatusChangeRefusedError` restores the pre-mutation snapshot like any failed
write. A response only corrects the displayed status while its write is the
latest one for the task (`shouldRestoreAfterFailedWrite`); otherwise the
reconciliation of the last write to settle decides. When the returned status
is not the requested `done`, both show an info toast with the step name
(`finalStepMoveNotice`): `task:movedToFinalStepQueued` for a queued move,
otherwise `task:movedToFinalStepNotCompleted`. The queued notice reports waiting
for room without promising completion: a final step can admit the task without
completing it when `CompleteTaskOnEnter` is disabled.

`components/task/task-move-error-message.ts` maps `task_approvals_pending` to
`task:taskMoveErrorApprovalsPending` for Kanban and task-page moves.

All new copy is added in every catalog (`en`, `pseudo`, `pt-pt`, `ja`, `ko`, `zh-cn`, `zh-hk`, `zh-tw`).

## Observability

No new metric. Refusals are visible as 409 responses with a stable `reason`
or `code`. The move path logs as today.

## Compatibility

- `TestUpdateTaskEndpoint_409OnWorkflowStepGate` keeps covering the redirect
  when no mover is wired. The agent cases in `approval_gate_step_test.go` stay.
- `apps/web/e2e/tests/office/approval-flow.spec.ts` "gated done transition"
  still passes: the refused move leaves the task in `in_review`, where the test
  put it, and the toast still names the approver.
- A direct human move of an Office task with pending approvers into a
  completing step now fails with 409 instead of completing.

## Selected atomic completion design, reviewed for implementation

This amendment was independently accepted in V6 and implemented locally after
the later explicit request. Delivery still requires independent actual-diff
review. The delivered preflight window violates the before-write guarantee;
the candidate closes it for human moves, same-step state completion, session
approval and Office final-step PATCH. Engine/agent/nonOffice behavior and
actual queued noncompletion remain unchanged.

### Boundary and transaction authority

Keep eligibility inside the task repository's existing mutation transaction;
compose the canonical Office and workflow readers through `backendapp`. Ordinary
human completion does not acquire relation-table locks, a new advisory namespace,
a schema revision, or a general transaction framework.

Domain-specific APIs in the existing owning packages:

```go
// task/models: internal provenance, following TaskCompletionMoveOverride's pattern.
// Values are never decoded from JSON. Expected scope came from existing authorization.
type OfficeHumanCompletionIntent struct {
    TaskID, ExpectedWorkspaceID, ExpectedWorkflowID, ExpectedStepID string
    ExpectedTaskState string // source state used to prepare automatic move metadata
    Kind string // move, set_state, office_done
}
func WithOfficeHumanCompletionIntent(context.Context, OfficeHumanCompletionIntent) context.Context
func OfficeHumanCompletionIntentFromContext(context.Context) (OfficeHumanCompletionIntent, bool)

// task/repository/sqlite: configured once in backendapp, before serving requests.
func (r *Repository) SetOfficeHumanCompletionPendingReader(
    func(context.Context, *sql.Tx, string, string) ([]string, error))
func (r *Repository) OfficeHumanCompletionGuardConfigured() bool

// Canonical implementations share private query helpers between reader pool and tx.
// office/repository/sqlite
func (r *Repository) ListTaskParticipantsAtStepTx(
    context.Context, *sql.Tx, string, string, string) ([]Participant, error)
// workflow/repository
func (r *Repository) ListActiveTaskDecisionsTx(
    context.Context, *sql.Tx, string) ([]*wfmodels.WorkflowStepDecision, error)

// office/dashboard; configured by backendapp, like TaskFinalStepMover.
type TaskHumanDoneCompleter interface {
    CompleteOfficeTaskAsHuman(context.Context, string, string) (*FinalStepMoveResult, error)
}
func (s *DashboardService) SetTaskHumanDoneCompleter(TaskHumanDoneCompleter)
func (s *DashboardService) ResolvePendingApproverNames(context.Context, []string) []PendingApproverDTO

// task/service and task/repository/sqlite; result is the persisted task snapshot.
func (s *Service) CompleteOfficeTaskAsHuman(ctx context.Context, taskID, expectedStepID string) (*Task, error)
func (r *Repository) CompleteOfficeTaskAsHuman(ctx context.Context, taskID, expectedWorkspaceID, expectedWorkflowID, expectedStepID string) (*Task, error)
// Generic HTTP/WS state edits use this wrapper; agent/internal UpdateTask stays unchanged.
func (s *Service) UpdateTaskAsHuman(ctx context.Context, taskID string, req *UpdateTaskRequest) (*Task, error)
func (s *Service) UpdateTaskStateAsHuman(context.Context, string, v1.TaskState) (*Task, error)
func (r *Repository) UpdateTaskStateAsHuman(context.Context, string, v1.TaskState) (*Task, error)
```

The canonical pending-reader callback receives the *same* `*sql.Tx` as the task
write. `backendapp` composes the Office destination slate and workflow active
decisions, preserving `pendingApproversAtStep` semantics: destination workflow
per-task seats, target templates, existing per-task precedence/canonicalization,
and approved active decisions by decider ID. Extract the pure approved-by
comparison once; do not fork or narrow the current predicate. SQL helpers take a
small private `QueryContext`/`QueryRowContext` interface; `sqlx.Tx.Tx` joins the
standard transaction when needed. No pool read escapes the transaction.

The repository helper is
`guardOfficeHumanCompletionTx(ctx context.Context, tx *sql.Tx, candidate *models.Task) error`;
it reads the fresh locked snapshot itself. It runs before task, completion-history,
transition, marker or durable-effect writes, including replay paths that would
actually mutate. Existing exact-operation replays return their stored result
without redoing effects; they are not a new completion. The helper is called
from `updateTaskTx` (manual/same-step/exact move),
`UpdateTaskFieldsWithParentAdmission` (same-step HTTP/WS state edits), and the
new scalar Office human completer. The existing generic completion-criteria
check remains in its current task mutation paths. The Office direct Done path
keeps its separate existing criteria exclusion; this amendment closes its
Office-approver window, not a new generic criteria policy.

`MoveTaskWithOptions` attaches intent whenever its *server-supplied*
`StepHistoryActor` is Human, regardless of the pre-read `IsFromOffice` flag.
ApproveSession keeps Human. HTTP/WS generic task-state handlers call
`UpdateTaskAsHuman`; direct WS task.state uses `UpdateTaskStateAsHuman`.
HTTP bulk moves pass trusted Human provenance into the existing per-item move
path. Existing MCP/internal agent calls keep their original methods. The
Office handler's already-established empty `agentIDFromCtx` classification
selects the human path. Do not infer humanity merely from an owner identity:
agent operations also carry their owner's identity. Reject inherited intent
for a different task; agent/engine entry points must not inherit a human
intent into nested work. One observed task snapshot receives the existing workspace-write authorization;
that same workspace/workflow/step is carried to the transaction fence.
The transaction never silently follows a different scope.

The early service guard is not authoritative when the repository reports its
transaction reader configured; defer eligibility until actual admission is
known. This also avoids refusing a WIP-queued noncompletion from a speculative
completed candidate. Legacy narrow test stores with no transaction capability
retain the current optional preflight behavior. Both nil guard and nil reader
fixtures preserve existing compatibility. Production `backendapp` must assert
that the concrete repository reader is configured before exposing Office
mutations; it cannot silently take the legacy path. Names may fall back to IDs
when the optional resolver is absent; eligibility never does.

### Fresh state and policy at the boundary

Participating PostgreSQL transactions explicitly use READ COMMITTED, as task
admission already does. A session default of REPEATABLE READ would retain an
old locator after waiting; the post-lock re-read must see committed scope.
SQLite retains its existing single-writer reservation.

After workspace, step and task locks, re-read actual task scope, state, project,
queue and entry identity. Compute Office membership from project or workspace
Office workflow. Validate named workflow/step membership; dangling steps fail
closed. Preserve exact/version/claim fences and the Office scalar criteria
exclusion only while actual Office membership holds.

Ordinary field edits fence the authorized workspace but apply supplied fields
to fresh placement. Explicit state/step edits retain source placement fences.
Human admission intent ends at commit: entry dispatch, published callbacks and
queue reconciliation must not inherit it. An Office participant add observing
no task returns the existing unchanged result before role locking; it cannot
follow a task created later without workspace exclusion.

Read destination `complete_task_on_enter`, WIP limit and workflow positions in
this transaction. Every step create/update/reorder/delete now takes the same
workspace exclusion first, so a sibling insertion or reorder cannot change
terminality behind a locked target step. The task repository decides actual
capacity using the fresh target limit before the Office check.

- A step-changing human move derives terminal completion with the existing
  `IsTerminalStep` semantics (last-by-position plus complete-on-entry), after
  admission. If queued, preserve current state and normal queued placement;
  do not treat the speculative completed state as a completion.
- A same-step reorder stays a reorder; it does not become an arrival or start
  entry actions. If a same-step mutation explicitly writes completed, it must
  pass the same Office gate. Generic explicit state edits preserve their
  existing step-position policy; do not add an implicit move.
- Explicit Office Done at the current final step may complete even when
  `CompleteTaskOnEnter` is false: explicit status and automatic entry are
  different contracts. A non-final Office Done move must still target the
  actual final step; fence the pre-resolved highest-position/ID selection.
  A stale target or changed completing/admission policy returns existing
  `workflow_change_conflict` before writes, instead of trusting a stale
  service-derived completed candidate. The no-step legacy explicit-status
  behavior remains, but a dangling nonempty step fails closed.
- Prefer recomputing transaction-owned state from the fresh row; where the
  caller prepared state-dependent metadata, compare against the fresh policy
  and refuse with the existing conflict before writing rather than merge a
  stale state into it. Existing authorized scope and source-entry retries rebuild the entire
  candidate; they do not retain stale completion or entry options.

The final-step PATCH no longer delegates a human completed write to the Office
repository's unguarded `UpdateTaskState[IfWorkflowStep]`. Add a narrow
`TaskHumanDoneCompleter` seam in `human_done_final_step.go`, implemented in
backendapp over the new service method. It handles final/no-step status writes
with a transaction-local scope/policy fence. Non-final Done keeps the existing
mover, with `office_done` intent, the actual-final-step fence and normal WIP
admission. No branch falls back to the agent redirect after a human guard
refusal. Only after successful scalar persistence does the dashboard run its
existing status comment/activity/event/reactivity sequence once. On refusal it
runs none. The move path retains existing task-service/orchestrator ownership;
no duplicate status/event pipeline is added.

Return a typed repository refusal carrying only the stable pending agent IDs.
After rollback, task service converts it to existing
`ApprovalsPendingMoveError`, resolving display names through a new
`ResolvePendingApproverNames(ctx, ids)` adapter over the existing dashboard
resolver. Never re-read eligibility or accept an approval after rollback to
turn a refusal into success. ID fallback is deterministic. HTTP moves,
session approval and task-field updates use existing409
`task_approvals_pending` plus pending details. WS task.move and task.update use
existing CONFLICT/error_code/pending details. Office PATCH uses existing
`FinalStepMoveRefusedError{Reason: approvals, Pending}` without `status`, so the
board/picker rolls back. Read errors abort without writes or wakes. Agent
redirects retain their existing `status: in_review` response.

### One acquisition graph for runtime writers

The order below is normative for the reviewed implementation. No participating
writer may acquire a workspace lock after a role, decision, table or row lock.
Ordinary admission's local sorted step-arrival mutexes remain an outside-SQL
prefix; no caller may enter them while holding SQL locks, and no transaction
callback invokes a nested mutation.

1. Begin the writer transaction (PostgreSQL READ COMMITTED; SQLite immediate
   writer reservation). Resolve non-locking locator snapshots only.
2. Acquire the complete source/destination workspace set in sorted ID order
   using existing `db.LockTaskHierarchy(ctx, tx, driver, bind, workspaceIDs,
   nil)`. The existing empty-workspace/config advisory key remains unchanged.
3. Re-read every locator under those locks. If task workspace/workflow/step or
   participant-by-ID source scope changed, rollback the entire transaction.
   Preserve only existing bounded retries that already repeat authorization
   and rebuild the complete lock set. Low-level locator races without such an
   existing seam roll back and return the existing stable conflict; introduce
   no retry loop, authorization callback or foreign-scope following. An authoritative ExpectedWorkflowID,
   exact generation or source predicate mismatch returns its existing
   conflict immediately. Never append a newly discovered workspace to held
   locks or reuse authorization from the previous scope.
4. Transfer alone performs the complete nonblocking relation reservation
   described below, before any role/decision/step/task locks. Ordinary
   completion and eligibility writers do not take table locks.
5. Existing decision-identity advisory keys, sorted if multiple, then existing
   `ParticipantRoleSeatLockKey(taskID, role)` keys, sorted if multiple. The key
   is exactly `workflow-participant-role-seat:|<task>|<role>`; the decision key
   remains `workflow-step-decision:|<task>|<step>|<identity>`.
6. Sorted workflow-step row locks, then sorted task rows, then participant,
   decision and marker writes; perform fresh completion evaluation before
   any mutation/history/effect. Commit before any event, name lookup, run
   dispatch or other external callback.

Immutable workflow workspace IDs are read from `workflows`; generic workflow
update does not reassign `workspace_id`. Template scope comes from step to
workflow to workspace, never an empty task ID. A by-ID participant upsert locks
both the old row's scope and requested new task/template scope. Revalidate its
old locator after locking. For an initially absent ID, use insert-on-ID-conflict
DO NOTHING and rollback with existing conflict on a lost insertion; a blind ON CONFLICT update
must not move a concurrently created row from an unheld workspace. Existing
natural-key/seat and decision uniqueness semantics remain.

Required participation is concrete:

| Writer | Workspace-first change and revalidation |
| --- | --- |
| Office AddTaskParticipant, claim/promotion, RemoveTaskParticipant | Task locator first, workspace lock, re-read, then existing role lock and canonical writes. |
| Workflow UpsertStepParticipant, UpsertTaskParticipant, DeleteTaskParticipant, DeleteStepParticipant, EnsureRoleSeat | Resolve task/template and old/new by-ID scope, lock complete set, revalidate, then existing role keys where present. |
| RecordStepDecision, including roleless | Task/workflow scope first; workspace before decision identity and optional role key; existing participant-seat validation remains. |
| SupersedeTaskDecisions and ClearStepDecisions | Replace autocommit with one short writer transaction; workspace first, then unchanged canonical UPDATE/DELETE. |
| ClearStepDecisionsAndCompleteMarker | Same workspace-first prefix inside its existing transaction; deletion and marker completion still commit together. No call to a separate workflow-store transaction. |
| CreateStep, UpdateStep variants, ReorderStepsIfUnchanged, ClearStartStepFlag, DeleteStep and relevant workflow deletion | Resolve workflow workspace first; lock it before demotion, cleanup, policy or step writes. Revalidate IDs/expected versions. Step deletion cascades participants, not decisions; preserve existing orphan/active-decision semantics. |
| Existing task hierarchy/field/move writers | Keep workspace-before-step/task order; add locator revalidation and intent evaluation at the shared write sites. No new ordering for scalar engine state writers is required for Office eligibility, because they do not mutate the slate; transfer's reservation must also tolerate their row-first shape. |
| TransferTask | Lock explicit source/destination workspaces first, revalidate exact source predicates, then nonblocking complete relation reservation, then existing mapping/census/mutation. |

Runner-only helpers do not introduce approvers and need no new Office gate.
Workspace row updates to Office workflow membership serialize with its FOR
UPDATE lock; project/placement changes serialize with the locked task row.
Startup migration/backfill is outside runtime request admission; preserve the
required-store initialization barrier. The external full SQL writer/consumer
inventory remains in the V6 evidence packet; this table selects the changes,
not a claim that the current code already obeys them.

### Transfer obstruction, probe and smallest necessary change

Keeping transfer's current *blocking* SHARE ROW EXCLUSIVE reservation is not
compatible with a bounded workspace-first design. Real PostgreSQL17 minimal
SQL-shape probes observed both wait-graph cycles using `pg_blocking_pids`:
relation-lock-before-hierarchy (178↔171) and workspace-first SRE NOWAIT
(227↔220). SHARE ROW EXCLUSIVE permits an existing SELECT FOR UPDATE holder's
ROW SHARE table lock. Transfer can therefore reserve the table and wait on its
row, while that row owner waits to upgrade to ROW EXCLUSIVE for its UPDATE.
Merely adding NOWAIT to the SRE acquisition does not close this cycle.

Selected necessary transfer-only change: after both workspace locks and locator
revalidation, acquire the *complete existing discovered relation set* in sorted
order with `LOCK TABLE ... IN EXCLUSIVE MODE NOWAIT`. PostgreSQL EXCLUSIVE also
conflicts with ROW SHARE, so a prior row-lock owner causes55P03 before transfer
continues. Roll back the entire transaction, including workspace and any
partially acquired relation reservations, then audit and return the existing
stable task-transfer conflict. Record result `conflict`, not `failed`: existing
`validateTaskTransferRejectedKey` poisons denied/failed keys, while conflicts
permit the same caller/key to retry. No lock upgrade is awaited, no table reservation
is followed by a late workspace acquisition, and no hidden retry/backoff is
introduced. Callers may retry the same idempotent command through the normal
authorization path. Once the full reservation succeeds, a new row-lock/DML
owner blocks at the table boundary until commit; it cannot hold a target row
that transfer will later need. Plain readers remain allowed.

V6 probe receipts remain SQL-shape evidence only. Product tests separately
exercise prior/new row owners, full census and rollback. The exact reservation
set is the sorted union of the present projection and
preservation-count tables already discovered, plus core `tasks`, `workspaces`,
`workflows`, `workflow_steps`, `task_step_transitions`,
`task_transfer_operations`, `task_transfer_audit`, `task_transfer_serialization`,
and the existing conditional `office_labels`, `office_task_tree_holds`,
`agent_profiles` entries. Two concrete core gaps must be corrected: current
discovery omits `tasks` (its key is id, not task_id) and explicitly skips
`task_step_transitions` from before/after preservation comparison and digest.
The receipt separately reports its post-write transition count.
Reserve both without changing the count/digest semantics. Task, session and
workspace write-time FK targets are covered by core and existing counts;
participant step mapping references core workflow_steps. Generic projection
updates change only workspace_id, whose target is core workspaces. Transfer
neither mutates nor row-locks step-entry markers. Its discovered entry/count
reads retain the existing reservation. Runtime schema migrations are outside
request admission. Tests must assert this exact union and all transfer DML/FK
consumers, rather than leave census selection to implementation.
Unknown/unmapped relation preservation refusals remain unchanged.

This is the exact smallest additional boundary change needed beyond Office:
`task_transfer.go` acquires workspaces/revalidates before calling
`lockTaskTransferRelations`; that helper adds the two core tables, changes mode and uses
NOWAIT;55P03 maps after rollback to existing ErrTaskTransferConflict/audit.
SQLite keeps its existing single writer and serialization-row reservation,
after logical workspace/source validation. No schema or new lock namespace.
The tradeoff is explicit: transfer already freezes a broad relation census;
its reservation is stronger and contention now promptly conflicts
instead of waiting. Other-workspace row writers may cause that conflict.
This task-system contract amendment is reviewed and accepted in the linked
transfer design and requirements; it must not be hidden in an
Office-only implementation.

Reject relation-table exclusion *for human completion* as the bounded
comparison: it would globally serialize unrelated completions/writers and
requires the same complete table order as transfer. A partial participant/
decision table list or SRE lock before hierarchy admits the observed row/table
cycle; an awaited upgrade under held rows is unsafe. Revision/schema and a new
global mutex are larger alternatives. The selected boundary remains workspace
exclusion for Office eligibility, with the necessary correction to transfer's
existing exceptional reservation. If parent declines that transfer conflict
tradeoff, retaining current blocking SRE is not a safe fallback: the concrete
alternative is a separately designed per-task transfer preservation protocol
covering every discovered relation writer, a substantially broader change.

### Deterministic implementation matrix and limits

Before atomic production edits, add causal RED using real repositories/services:
pause after the current preflight, commit a new task approver, destination
template, nonapproved decision or bulk supersession, then release. Old code
completes or redirects/writes; the new transaction must refuse without task,
review, decision, history, marker, event or on_enter/run effects. A same-fixture
no-pending control must reach completion. Production callback wiring is used;
no fake guard substitutes for database serialization.

Run each ordering on SQLite and two independent PostgreSQL connections:

- Each human route: step-changing Done, same-step explicit completion,
  ApproveSession and final-step PATCH. Include generic HTTP/WS task-state
  completion. Add actual existing terminal/no-step/dangling-step cases.
- Add/claim/Ensure seat; by-ID task/template insert/rekey/delete/role change;
  roleful and roleless decision writes; supersede; clear; marker-atomic clear.
  Show writer-before-admission refusal and admission-before-writer blocking,
  with actual PostgreSQL lock observations and channels rather than sleeps.
- Transfer A→B and B→A with sorted workspaces; prior scalar task row owner;
  unrelated-workspace relation writer; new row owner after reservation;
  cancellation/55P03 cleanup; source move between locator and lock; unknown
  relation census. Verify existing stable conflict/audit/idempotent receipt,
  unchanged source on refusal and preservation digests on success.
- Flip Office membership, target CompleteTaskOnEnter/WIP, insert/reorder a later
  step, delete/move a target reference, and reassign workflow while completion
  is paused. Prove fresh policy or existing conflict, not stale completion.
- WIP-full completing target plus pending approver still queues without
  completing; noncompleting moves, no-op reorders, nonOffice, engine and agent
  APIs keep behavior. Names resolve only after rollback with ID fallback;
  reader failure is closed; nil legacy tests stay explicit.
- Foreign/owner authorization, same-step ExpectedWorkflowID and exact-operation
  replay fences, generic completion criteria and its Office scalar exclusion,
  no duplicate events, unchanged session review on refusal, desktop and native
  phone success/refusal/queued flows. Final-step human refusal now has no
  redirect status; frontend rollback uses the already-established shape.

The local implementation is uncommitted for independent actual-diff review.
SQLite/PostgreSQL17 product RED/GREEN, writer/transfer controls and race checks
passed; containers were removed. V6 probes remain historical SQL-shape evidence.
Go lint, docs and fresh managed desktop/Pixel5 completion, refusal and queued
checks passed on the recorded actual-main combined tree. No schema, generic retry
policy or provider action was added.
