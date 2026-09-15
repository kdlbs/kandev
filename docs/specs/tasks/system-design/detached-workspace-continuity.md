---
status: current
system: tasks
requirements:
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
---

# Detached Workspace Continuity System Design

## Purpose and boundaries

Task lifecycle owns `task_environments` and task-owned worktree lifetime. This
design makes hierarchy detachment, shared-workspace stewardship, creation-time
attachment, and destructive cleanup use one durable ownership contract. The
workspace system continues to own workspace and repository configuration; it
does not own task-environment cleanup authority.

The design extends
[ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
and applies the fail-closed rule from
[ADR-0009](../../../decisions/0009-fail-closed-gc-semantics.md). The ownership
generation and transaction rule is recorded in
[ADR-2026-09-04-generation-fenced-task-environment-ownership](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001` | [Ownership model](#ownership-model), [Atomic detachment](#atomic-detachment), [Cleanup fencing](#cleanup-fencing), [Restart and recovery](#restart-and-recovery) |
| `REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002` | [Canonical creation attachment](#canonical-creation-attachment), [Concurrency and failure behavior](#concurrency-and-failure-behavior) |

| Acceptance criteria | Design section/test |
| --- | --- |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.1` | Atomic detachment; mode/owner tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.2` | Atomic detachment transaction; rollback tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.3` | Cleanup fencing; parent archive/delete races |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.4` | Admission matrix; stale-generation tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.5` | Restart and recovery; replay tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.6` | Command predicates; mixed-owner conflict tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.1` | Canonical creation attachment; route matrix tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.2` | CreationPlan attachment step; Abort/recovery tests |
| `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.3` | Lock order and admission; dialect race tests |

## Ownership model

`task_workspace_groups.owner_task_id` identifies the current workspace steward.
Exactly one active membership row for that group has role `owner`; all other
active memberships have role `member`. The group gains an
`ownership_generation` integer that starts at `1` and increments whenever the
steward changes.

`task_environments.task_id` remains the only physical environment owner. The
environment gains its own `ownership_generation`, also starting at `1` and
incremented by every successful owner transfer. A cleanup snapshot identifies
an environment by ID, owner task ID, and ownership generation. An environment
ID alone is never sufficient authority for a destructive operation.

The workspace group can remain shared after detachment. Stewardship may move to
another active member later; this does not copy the workspace or invalidate
sessions that already reference the canonical environment.

## Components and responsibilities

- `internal/task/archivecascade` owns `Store.SetTaskParent` and
  `Store.DetachTask`, the sole cross-table transaction owners for hierarchy,
  group stewardship, environment ownership generations, cleanup barriers,
  lifecycle revisions, and `task.updated` outboxes. Its shared `*sqlx.DB` and
  dialect are the only persistence boundary.
- `internal/task/repository/sqlite` supplies private dialect transaction helpers;
  no service or Office caller can invoke them or bypass aggregate admission.
- `internal/task/service.Service.DetachTask` invokes only the aggregate and
  returns its committed `DetachTaskResult.Task`; it never reloads a mutable DTO
  after the transaction. The lifecycle outbox dispatcher publishes the result.
- `internal/task/service.Service.CreateTask` owns required creation-time
  workspace attachment before `task.created` is published or a launchable
  result is returned. A narrow injected coordinator reuses
  `HandoffService` policy resolution and membership behavior without making API
  handlers responsible for correctness.
- `internal/task/service` task-resource cleanup validates environment ownership
  and generation before teardown. Ownership transfers reject while the source
  task has an active cleanup barrier.
- `internal/task/service.HandoffService` group cleanup claims the current group
  generation before external cleanup and completes the status transition only
  for that same generation.

## Atomic detachment

The hierarchy mutation APIs are closed. `Store.SetTaskParent` handles a
non-empty target parent; `Store.DetachTask` handles an empty target parent.
`SetTaskParent` accepts
`SetTaskParentCommand{Actor,OperationID,RequestSHA256,ChildTaskID,ExpectedParentID,ExpectedParentRevision,ExpectedLifecycleEpoch,TargetParentID,ExpectedTargetParentRevision,ExpectedGroupID,ExpectedGroupGeneration,ExpectedEnvironmentID,ExpectedEnvironmentGeneration}`.
It returns
`SetTaskParentResult{Task,Changed,OperationID,RequestSHA256,LifecycleEpoch,TaskRevision,QueueSequence,EventID?}`.
It rejects an empty target, missing child/target parent, self-parent, cycle,
cross-workspace target, stale task/target revision, or stale group/environment
generation with a typed conflict and zero task/domain writes. It locks workspace
tier 1, all relevant barriers tier 4, child and target-parent tasks in sorted
task-ID order tier 5, retained revision rows tier 6, and selected group/member
rows tier 12; it reloads every predicate after the full lock prefix and restarts
at tier 1 when the scope digest changes.

For a changed parent, the branch contract is:

| Existing mode | Task writes | Forbidden writes |
| --- | --- | --- |
| `inherit_parent` | target `parent_id`; normalize metadata mode to `shared_group` | no membership, owner/steward, group, environment, repository, session, or descendant writes |
| `shared_group` | target `parent_id`; preserve mode | no membership, owner/steward, group, environment, repository, session, or descendant writes |
| `new_workspace` | target `parent_id`; preserve mode | no membership, owner/steward, group, environment, repository, session, or descendant writes |

The task row, parent relation, next revision, and one canonical `task.updated`
outbox envelope commit in one transaction. If the child already has
`TargetParentID`, the command is a `Changed:false` no-op after validating every
expected identity; it writes no task row, mode, revision, or outbox event,
returns `EventID:null`, and retains only its result-ledger row. Its result replays
byte-for-byte. A different request hash for an existing operation is
`task_parent_request_conflict`.

`task_parent_mutation_results` is the retained result ledger for both changed
and no-op SetTaskParent commands. Its unique key is `OperationID`; it stores
request hash, actor binding hash, child/target IDs, lifecycle tuple, result
payload, and nullable event identity. It is inserted atomically with the task
outbox for a change or with the validated no-op result, so response loss cannot
observe a later parent. Missing selected group/member rows lock their canonical
absent keys and conflict; optional empty sets acquire no row. No caller composes
SetTaskParent with stewardship or environment mutation.
`task_detach_mutation_results` is the retained result ledger for changed and
already-root DetachTask commands. Its unique key is `OperationID`; it stores
request hash, actor binding hash, child and expected-parent IDs, lifecycle tuple,
result payload, nullable event identity, and the validated expected group and
environment generations. The command inserts this row in the same transaction
as a changed task/revision/outbox mutation, or in a read-only no-op transaction
after validating the expected identity. A same operation/hash replays the stored
result byte-for-byte; a different hash conflicts; a missing result with an
existing operation cannot inspect a later task holder.


### Retained mutation-result tables

Both result ledgers are retained historical-identity tables:

```text
task_parent_mutation_results /
task_detach_mutation_results
  operation_id UUID PRIMARY KEY
  workspace_id UUID NOT NULL
  child_task_id UUID NOT NULL
  expected_parent_task_id UUID NULL
  target_parent_task_id UUID NULL
  request_sha256 BLOB/BYTEA NOT NULL CHECK(length=32)
  actor_binding_sha256 BLOB/BYTEA NOT NULL CHECK(length=32)
  lifecycle_epoch BIGINT NOT NULL
  task_revision BIGINT NOT NULL
  queue_sequence BIGINT NOT NULL
  changed BOOLEAN NOT NULL
  event_id UUID NULL
  result_json BLOB/BYTEA NOT NULL
  result_sha256 BLOB/BYTEA NOT NULL CHECK(length=32)
  created_at TIMESTAMP NOT NULL
```

The two tables use identical DDL and have unique `operation_id`, canonical
RFC8785 `result_json`, and `result_sha256 =
SHA-256(ASCII("kandev/task-parent-mutation-result/v1") || 0x00 || result_json)`
or the corresponding `task-detach-mutation-result/v1` domain. Their only
historical identity foreign key is
`(child_task_id,lifecycle_epoch) -> task_lifecycle_revisions` with
`ON DELETE RESTRICT`; they never reference mutable `tasks` rows or a deletable
workspace row. Dialect-specific BLOB/BYTEA, boolean, UUID, timestamp, checks,
and `DialectDDLHash` entries are generated from these exact columns.

`Store.DetachTask` accepts
`DetachTaskCommand{Actor,OperationID,RequestSHA256,ChildTaskID,ExpectedParentID,ExpectedGroupID,ExpectedGroupGeneration,ExpectedEnvironmentID,ExpectedEnvironmentGeneration}`
and returns `DetachTaskResult{Task,Changed,OperationID,RequestSHA256,LifecycleEpoch,TaskRevision,QueueSequence,EventID?}`.

1. Lock workspace identity at tier 1. Discover every active archive, creation,
   external-ID release, workspace-deletion operation and cleanup barrier whose
   locked scope includes either task, the group, or any selected environment,
   repository, or worktree. Sort operation IDs and barrier keys; lock tier 4,
   re-read the complete set and digest, and restart from tier 1 if either
   changes.
2. Lock child and former-parent task rows in deterministic task-ID order (tier
   5), then the child's retained lifecycle revision/watermark rows and both
   `task_parent_mutation_results` and `task_detach_mutation_results` ledgers
   (tier 6).
3. Lock selected creation/effect rows (tier 7), cleanup, materialization, and
   launch-dispatch claims/resource snapshots (tier 8), sessions (tier 9), and
   runtime rows (tier 10), each by canonical primary key. Then lock every
   selected environment, repository, and worktree row in canonical order (tier
   11), followed by every selected workspace-group and member row (tier 12).
   Lock group snapshots/cleanup rows (tier 13) and resource markers (tier 14)
   before any state change.

Every selected row is identified by its canonical primary key; a required
missing row locks its canonical absent identity key and conflicts, while an empty
optional set acquires no row. No absent key may be treated as ownership proof.

Detachment intentionally acquires no tier 2, 3, 15, or 17 rows; this empty-tier
set is part of the registry contract and no later-tier acquisition may precede
an omitted tier.

4. Before clearing `tasks.parent_id`, for `inherit_parent` require
   `group.owner_task_id == ExpectedParentID`, exactly one active owner-role
   membership for that parent, `environment.task_id == ExpectedParentID`, and
   the environment's group linkage plus both expected generations to match.
   Then normalize the child to `shared_group`, make it steward, demote the
   former owner, and increment group/environment generations. For `shared_group`
   and `new_workspace`, preserve mode, membership, owner, and generations.
   Missing or inconsistent state returns typed `detachment_state_conflict` with
   zero writes.
5. Allocate the next lifecycle revision and insert the immutable `task.updated`
   outbox row, including cleared `parent_id` and changed metadata, in the same
   transaction. Commit once. Any failure rolls back every change.

The outbox envelope is the canonical
`{task_id,lifecycle_epoch,task_revision,queue_sequence,event_id,tombstone,payload}`
shape; payload is the complete updated task DTO with explicit empty `parent_id`.
The outbox row also binds `OperationID` and `RequestSHA256`; its unique event
identity is `(task_id,lifecycle_epoch,task_revision,queue_sequence,event_id)`.
Response loss replays the retained operation result; same key/hash is idempotent,
and a different hash conflicts. Delayed delivery follows the lifecycle watermark
and cannot resurrect a tombstone.

Detaching an already-root task validates the expected identity and is an
idempotent read of the current state. It writes no task row, generation,
lifecycle revision, or outbox event, returns `Changed:false` and `EventID:null`,
and retains the result for byte-for-byte replay. A different request hash
conflicts.

## Detachment admission matrix

The lock-time admission matrix is closed:
| Selected authority | Active, uncertain, or claimed states | Terminal/allowed result |
| --- | --- | --- |
| archive operation | `preparing|archiving|unarchiving|restore_pending|foreign_scope_blocked|retry_wait|archived` | `completed|aborted|foreign_scope_resolved|workspace_deleted` only if no selected claim remains |
| creation operation | `preparing|preparing-release|completing` | `completed|aborted` only if no selected claim remains |
| external-ID release | every nonterminal, leased, retry, or unknown state | terminal replay only when it does not target the selected task |
| workspace-deletion step | `pending|running|retry_wait|unknown` plus `rehome_pending` disposition | `succeeded` only when it targets no selected resource |
| task/group cleanup and restore | `prepared|pending|running|retry_wait|unknown|failed`; target `cleanup|restore`; group status `cleanup_pending|cleanup_failed`; restore `restore_pending|restore_retry_wait|restoring`; bounded exhaustion is `policy=bounded_terminal, attempts>=8` | `cancelled|restored|succeeded` only with no active claim and unchanged owner/generation |
| materialization/launch claim | `planned|running|unknown|retry_wait|blocked|transferred`, or any unfinished dispatch claim | `succeeded|cancelled|transferred` only with unchanged owner/generation |
| resource marker | `cleanup_claimed|cleanup_unknown|restore_claimed|restore_unknown` | `cleaned|cleanup_cancelled|restored` only with unchanged owner/generation |

Every active, uncertain, failed-with-claim, transferred, or exhausted row returns
typed `detachment_conflict` with zero writes. Detachment never waits for a guard
or transfers a claim. Terminal states and dispositions still undergo exact owner,
membership, linkage, generation, and materialization checks; missing or mixed
rows return `detachment_state_conflict`. With no admitted row, cleanup wins before
admission, while a committed detachment is in the next inventory.


SQLite's writer transaction provides serialization. PostgreSQL uses explicit
row locks in the order above. Archive and delete preparation already reserve a
task cleanup barrier; detachment uses the same barrier states so either the
transfer commits before cleanup inventory is captured or detachment is rejected
after cleanup wins.

## Canonical creation attachment

Workspace policy resolution and attachment become part of the task service's
required synchronous create sequence. REST, WebSocket, MCP, plugin Host API,
workflow child creation, and other internal callers pass parent and explicit
workspace-policy inputs through `CreateTask`; no caller performs an independent
post-create membership write.

| Route owner/symbol | Policy input | Planned step and bypass | Route test owner |
| --- | --- | --- | --- |
| `office/runtime.RegisterRoutes -> Handler.createTask` (`POST /runtime/tasks`) | authenticated run context and Office workspace defaults | delegates `Actions.CreateTask` -> canonical service; selected/no-policy branches | Office runtime root route test |
| `office/runtime.RegisterRoutes -> Handler.createSubtask` (`POST /runtime/tasks/:id/subtasks`) | authenticated run context and parent ID | delegates `Actions.CreateSubtask` -> canonical child service; identity-found bypass | Office runtime subtask route test |
| `office/runtime.Actions.CreateTask` | `RunContext` workspace and parent policy | root/child branch uses the same resolver and coordinator | Office runtime action test |
| `office/runtime.Actions.CreateSubtask` | `RunContext` task and parent policy | delegates unchanged; no independent attachment write | Office runtime subtask action test |
| `cmd/agentctl.taskCreate` (`POST /api/v1/office/runtime/tasks`) | authenticated CLI run context | same root route and canonical coordinator | authenticated CLI task-create test |
| `task/service.Service.CreateTask` | complete request policy plus admission inputs | canonical resolver and `CreationPlan`; selected/no-policy/identity-found branches | service aggregate contract test |
| `task/repository/sqlite.Repository.CreateTaskWithWorkflowStepAdmission` | workflow-step admission derived from the aggregate request | internal primitive invoked only inside `Service.CreateTask`; no direct public route | repository admission ownership test |
| `task/repository/sqlite.Repository.CreateTaskIfWorkflowStepHasCapacity` | capacity admission derived from the aggregate request | internal primitive invoked only inside `Service.CreateTask`; no direct public route | repository capacity ownership test |
| `task/handlers.TaskHandlers.httpCreateTask` | `body.WorkspaceMode|WorkspaceGroupID|EnvironmentID`, resolved by `resolveWorkspacePolicy` | selected policy plans `workspace_attachment`; no-policy has no step; `FoundSettled|FoundUnsettled` bypasses all post-create work | HTTP create handler contract test |
| `task/handlers.TaskHandlers.wsCreateTask` | WS request policy fields and parent | same selected/no-policy/identity-found branches | WS create handler contract test |
| `mcp/handlers.Handlers.handleCreateTask` | MCP policy fields and source/parent task | same selected/no-policy/identity-found branches | MCP creation ledger test |
| `backendapp.pluginsTaskWriterAdapter.CreateTask` | plugin `TaskCreateInput` plus parent/default policy | same selected/no-policy/identity-found branches | plugin task writer contract test |
| `task/service.Service.CreateChildTask` | parent task's group, member, and environment binding | inherited policy plans the step; attachment failure aborts or recovers; identity-found bypass | `service_child_task_test.go` |
| `office/engine_adapters.TaskCreatorAdapter.CreateChildTask` | parent ID resolved through `ParentTaskRepo` | delegates unchanged to `Service.CreateChildTask`; no independent step | task creator adapter test |
| `backendapp.childTaskCreatorAdapter.CreateChildTask` | Office engine parent and typed child spec | delegates unchanged to `Service.CreateChildTask` | Office adapter test |
| `backendapp.taskCreatorAdapter.CreateOfficeTask` and `CreateOfficeTaskAsAgent` | workspace office-workflow default | selected policy step before return; no-policy has no step | Office adapter test |
| `backendapp.taskCreatorAdapter.CreateOfficeTaskInWorkflow` | explicit workflow plus workspace policy default | same selected/no-policy branches | routine task creator test |
| `backendapp.taskCreatorAdapter.CreateOfficeSubtask` | parent task ID and inherited policy | same selected/no-policy/identity-found branches | Office subtask test |
| `backendapp.reviewTaskCreatorAdapter.CreateReviewTask` and `issueTaskCreatorAdapter.CreateIssueTask` | watcher request workspace and repository inputs | same selected/no-policy branches; external-ID identity-found bypass | watcher composition test |
| `workflow/engine.CreateChildTaskCallback.Execute` | trigger task ID and engine child spec | delegates through `TaskCreatorAdapter`; never writes task/workspace rows | workflow callback test |

Public route owners (HTTP, WS, MCP, plugin, Office runtime, and authenticated
agentctl) build the canonical request/hash and delegate to `CreateTask` or its
child-service path. Service and adapter rows are delegates; repository admission
primitives are internal-only and receive an already planned request, so they do
not call `CreateTask` or create an independent route. Route tests cover
selected-policy, no-policy, attachment failure, and identity-found bypass.

The service resolves parent defaults before building task metadata. After the
task row is inserted, it records attachment as a typed `CreationPlan` step and
records the workspace-group membership and sequential blocker attachment before
publishing `task.created` or returning a Created outcome. If attachment fails,
the coordinator invokes handle-bound `AbortTaskCreation` only when its closed
predicate proves every effect absent, compensated, or transferred. Otherwise it
persists pending recovery and Runtime reconciles it; request cancellation or
error never directly deletes or settles the child. Deduplicated external-ID
outcomes do not reapply workspace policy.

## Cleanup fencing

Task-resource cleanup snapshots persist the environment owner task ID and
ownership generation captured after the cleanup barrier is reserved. Before
environment or associated worktree teardown, the repository compares the
snapshot with the live environment. A missing row is idempotent success only
when the resource is already positively known absent. An owner or generation
mismatch makes the destructive portion a safe no-op and records that the
snapshot was superseded.

Every environment-owner transfer uses a guarded repository method. It checks
the expected owner and generation, verifies that the source owner has no active
cleanup barrier, and increments the generation in the same transaction as the
owner change. The existing delete and cascade ownership-transfer paths use this
method; no unguarded `UPDATE task_environments SET task_id = ...` remains.

Workspace deletion performs the same guarded transfer before selecting owned
environment or repository rows: a shared group with a surviving foreign member
moves group stewardship, environment owner, repository ownership, and each
ownership generation atomically under the full resource lock set. Those rows are
excluded from the deleted-workspace selectors; stale cleanup claims then fail
the new generation and cannot remove the rehomed resources.

Workspace-group cleanup changes from check-then-delete to a generation claim.
The claim atomically verifies the group generation, ownership flags, cleanup
policy, and absence of active members before setting `cleanup_pending` for that
generation. Stewardship transfer is allowed only from `active` or retryable
`cleanup_failed` state with no active cleanup claim. Completion and retry writes
use the claimed generation; stale completions cannot mark a replacement group
cleaned or failed.

Filesystem and provider cleaners retain their existing managed-root and exact
resource-identity guards. Generation fencing is an additional authorization
condition, not a replacement for those checks.

## Concurrency and failure behavior

- Detach loses to an already-admitted parent or child cleanup and returns a
  conflict without clearing `parent_id`.
- Cleanup inventory captured after detachment cannot select the former parent's
  transferred environment because the environment owner has already changed.
- A delayed cleanup or ownership retry with an old generation is treated as
  superseded and performs no destructive work.
- Concurrent sibling detaches serialize on the shared group. Each committed
  transfer leaves one active steward and increments generations once.
- Creation returns success only after membership attachment. A concurrent
  parent lifecycle barrier causes creation to roll back instead of publishing a
  child that can launch without its workspace relationship.
- Repository or event-publication failures never compensate by deleting a
  workspace whose current ownership cannot be proven.

## Persistence and migration

Add `ownership_generation INTEGER NOT NULL DEFAULT 1` to
`task_workspace_groups` and `task_environments`. Fresh schemas include the
columns and replayable migrations add them to existing SQLite and PostgreSQL
databases. Existing rows retain their current owner at generation `1`; no
filesystem mutation occurs during migration.

All selects, inserts, model projections, test fixtures, and table-rebuild copy
lists that cover these tables include the new columns. Schema tests use the real
task/Office repository initialization order and cover fresh and replayed startup
for both dialects, following ADR-0027.

## Restart and recovery

Ownership generations live in the database and are included in durable cleanup
snapshots. On restart, cleanup workers re-read the current owner and generation
before acting. A snapshot prepared before a stewardship transfer cannot regain
authority after restart. The detached task's sessions continue resolving the
same materialized environment ID from the workspace group and their persisted
`task_environment_id` values.

## Observability

Structured logs identify the task, group, environment, expected owner and
generation, current owner and generation, and the resulting disposition for
ownership transfers and rejected cleanup. Logs do not include workspace file
contents, credentials, or restore secrets.
