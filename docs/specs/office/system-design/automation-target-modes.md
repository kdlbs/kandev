---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
  - REQ-OFFICE-AUTOMATION-TARGETS-002
  - REQ-OFFICE-AUTOMATION-TARGETS-003
  - REQ-OFFICE-AUTOMATION-TARGETS-004
---

# Automation Target Modes System Design

## Purpose and boundaries

This design adds an explicit target to automation dispatch. The Office system
owns the saved target and repository choices, admission, exact run bindings,
continuation ownership, and deletion behavior. The task system owns normal
task visibility and workflow placement. The agent runtime owns scratch
workspace creation and provider context. The frontend owns the target choice,
conditional validation, and accessible explanations.

The existing hidden automation target remains the default. A normal-task
target is visible work with normal task semantics, not a second coordinator
MCP surface.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATION-TARGETS-001` | Data and contracts, control flow, persistence |
| `REQ-OFFICE-AUTOMATION-TARGETS-002` | Components and responsibilities, control flow, failure and recovery |
| `REQ-OFFICE-AUTOMATION-TARGETS-003` | Frontend boundary and observability |
| `REQ-OFFICE-AUTOMATION-TARGETS-004` | Data and contracts, control flow, persistence |

## Components and responsibilities

- `internal/automation` owns `TaskMode`, ordered repository/base-branch
  selections, validation, persistence, portable export, admission, and
  `AutomationRun` accounting.
- `internal/orchestrator` resolves the target before task creation. It creates
  hidden tasks with `TaskOriginAutomationRun` and visible tasks with a distinct
  visible automation origin. It records the run ID in task metadata and binds
  the exact session and turn after launch. It carries the automation's optional
  `parent_task_id` through `ReviewTaskRequest.ParentID` into the task service,
  and preserves it in the retry launch snapshot.
- `internal/task/service` receives an empty repository list for a valid scratch
  task. It requires a workflow for a visible automation task and retains the
  normal workflow-step resolver when the step is omitted.
- `internal/agent/runtime/lifecycle` continues to create a task-scoped scratch
  workspace when `RepositoryPath` is empty. It does not create a worktree or
  run repository preparation for a repository-free launch.
- `internal/mcp/profile`, `internal/mcp/scope`, and the MCP handlers apply
  `SurfaceAutomation` only to hidden automation tasks. Visible automation
  tasks use their ordinary task profile and ordinary caller boundaries.
- `apps/web/components/automations` composes the shared task-creation workflow,
  repository, branch, agent-profile, and executor-profile selectors. It adds
  only automation-specific validation and keeps desktop and mobile behavior
  aligned.

## Frontend boundary

The editor keeps target mode separate from continuity. Hidden mode explains
that its per-run tasks stay out of the Kanban and sidebar. Normal-task mode
explains that each firing is ordinary workflow work. Switching the target
clears or revalidates fields that are not valid for the new mode, and the save
payload always carries the explicit target and ordered repository/base-branch
pairs. An empty list is the only no-repository representation. The editor does
not offer a workspace-default repository choice.

The workflow, repository, branch, agent-profile, and executor-profile controls
are shared with task creation. The workflow menu shows ordered steps and the
configured start marker. Repository rows use the same paired chips and branch
search. Profile menus use the same search, logos, labels, and availability
rules. The optional parent control reuses the shared searchable task picker and
is scoped to the automation's workspace. Typed input reaches the existing search
endpoint with its bounded limit. Responses are scoped to workspace and query;
late responses cannot replace the current result. The selected task label stays
available when another query omits that task. Other Combobox consumers retain
their existing local filtering. Automation code owns no parallel option catalog.

On phones the existing automation editor remains the primary scroll surface.
Target and continuity choices use the existing stacked control pattern with
44-pixel touch targets. No second drawer or horizontal editor surface is
introduced. The settings toolbar gives Export and New Automation the same
height through one shared button sizing rule.

## Data and contracts

The automation model and API add a persisted task choice and ordered repository
bindings:

```text
task_mode: automation_run | normal_task
repositories: [{repository_id, base_branch}, ...]
parent_task_id: <task id, optional>
```

`automation_run` is the compatibility default when older clients omit the
field. An empty `repositories` list means no repository. The compatibility
`repository_ids` field is accepted from older clients and resolves each ID to
its configured default branch, but no empty request falls back to the first
workspace repository.

An optional `parent_task_id` names an existing task in the same workspace. It is
carried through dispatch to `CreateTaskRequest.ParentID`; the task service's
existing hierarchy admission resolves and validates it, so the feature adds no
second parent rule. An empty value keeps the firing's task top-level. Because it
names a task row in this install, `parent_task_id` is excluded from portable
export like `continuation_task_id`.

An update that disables the automation without changing its parent can proceed
when that saved parent is stale. A changed parent or an enabling update still
uses canonical admission.

Dispatch checks the parent when preparation is about to create a new task.
Committed retry adoption first resolves the recorded task/operation identity; it
does not require that task's original parent to survive. A no-cascade deletion
may already have reparented the retained child to root. Adoption binds that same
child and never reattaches the deleted parent or creates a successor. Future
continuation selection still uses the existing parent/revision compare-and-set,
and ordinary continuation reuse still compares its current parent.

The existing parent relation read includes hidden automation-run children whose
stored `parent_id` names that parent in the same workspace. This does not change
board visibility or the ordinary child lists used for readiness and hierarchy
admission.

The task model adds a visible automation provenance value, for example
`automation_task`. Board, sidebar, and normal task queries continue to exclude
only `automation_run`, so the new origin remains visible without pretending
that it was manually authored. The run ID, automation ID, target mode, and
trigger identity are carried in server-owned task metadata; callers cannot
choose a different run binding.

`ReviewTaskRequest` and the automation dispatch contract carry the target and
run identity to the task service. The exact run remains the authority for
automation status; task ID alone is only a compatibility lookup for legacy
unbound rows.

## Control flow

### Save and validation

1. The editor presents the target choice. Hidden mode allows an empty workflow;
   normal-task mode requires a workflow. Selecting a workflow shows its step
   preview; there is no independent step picker.
2. The editor presents zero or more repository/base-branch chip pairs. Removing
   every pair selects scratch execution. Every non-empty row requires a
   repository and base branch.
3. The backend validates target, workflow, repository selection, and
   continuation policy together. Invalid combinations fail before a firing is
   admitted.

### Admission and dispatch

1. `FireTrigger` renders the run title, atomically checks the shared open-run
   predicate, stores a `triggered` `AutomationRun`, and publishes its run ID.
2. The orchestrator resolves the saved repository/base-branch pairs. An empty
   list remains repository-free. A provider event can supply its own exact
   repository only where its established event contract already does so.
3. A hidden target creates or resumes an automation-origin task. A normal-task
   target creates or resumes a visible automation-origin task. Both paths use
   the same exact task/session/turn binding and continuation slot.
4. A new task starts through the normal task launch path. With no repository,
   the lifecycle creates a scratch workspace. With a repository, the selected
   executor performs its normal repository preparation.
5. Hidden tasks receive `SurfaceAutomation`; visible tasks receive the normal
   task MCP profile. A visible task never inherits the hidden profile merely
   because its run was scheduled.

### Continuation

`new_task` creates a new task for either target. `reuse_thread` continues the
target's current task and primary session, and remains limited to one open run.
Compatibility checks include workspace, target mode, agent profile, executor
profile, and ordered repository/base-branch pairs. A missing or incompatible continuation gets a
replacement task and an explicit `created` or `replaced` action. The configured
workflow is used for a replacement, and normal task creation resolves its
configured start step. A resumed task keeps its existing title.

## Failure and recovery

- An empty repository list is valid for Worktree and Local-compatible executor
  profiles. The lifecycle creates a scratch directory and skips Git worktree
  preparation because there is no repository checkout.
- A task creation or exact launch failure marks the admitted run failed. The
  concurrency slot is released without deleting a visible task that was
  already created.
- Normal task completion and stop handling resolve the run by exact task,
  session, and turn metadata, then update only that run. They do not invoke the
  hidden automation agent stop or coordinator self-target rules.
- Hidden task cleanup remains reference-aware and deletes only unreferenced
  hidden tasks/worktrees. Visible tasks are left in the normal task lifecycle
  when an automation is disabled or deleted. Any open scheduled run is
  terminalized, but the visible task remains available to its owner.
- Startup reconciliation handles stale hidden and visible bindings with the
  same exact-turn liveness checks. A live, open, or blocked turn remains open.

## Persistence

Keep `task_mode` on `automations` and store ordered repository bindings in
`automation_repositories`, including `base_branch`. Fresh rows default to no
repositories. Existing non-empty repository bindings use the repository's
configured default branch when their legacy branch is empty. Existing empty
and `workspace_default` rows migrate to no repositories; the first-repository
fallback is removed. `parent_task_id` is a nullable-by-default `TEXT NOT NULL
DEFAULT ''` column added by an idempotent `ADD COLUMN` migration; empty means
top-level.

Portable YAML includes both user choices and repository IDs, but excludes
`continuation_task_id`, `parent_task_id`, run bindings, rendered titles, session
pointers, and cleanup jobs. Import normalizes omitted target fields to the
compatibility defaults and revalidates executor/workflow combinations.

Changing target mode, repository/base-branch pairs, workflow, agent profile, or
executor profile rotates the continuation on the next firing. Changing or clearing
the parent task also rotates the continuation; the previous task and run history
are retained.
Continuation reuse compares the task's parent. A firing writes or clears the
future continuation only while the existing automation revision, parent and
previous continuation still match its read. Retry dispatch obtains that revision
from its existing durable run. A stale conditional write changes no binding and
does not delete the older firing's task or history. No new revision or schema is
introduced. Legacy setter calls without a binding keep their existing behavior.
Name, description, prompt, title template, enabled state, and trigger edits do
not change the existing task until a firing needs a new or replacement task.

## Security

The task mode is server-owned configuration, not an MCP argument. Hidden
automation identity is derived only from a task whose origin and metadata bind
it to the automation. Visible automation tasks are not accepted as hidden
coordinator callers. Workspace authorization and exact run identity remain
required for all automation mutations. The one self-target exception is
`archive_task_kandev` on the calling hidden run task as terminal completion;
all other self-target mutations and foreign-workspace targets remain denied.

## Observability

Run records expose target mode through the owning automation and preserve exact
task/session/turn IDs. Dispatch logs include target mode, repository mode,
thread action, and failure reason without logging prompt contents or secrets.
The editor and run history expose whether a run produced a hidden task or a
visible normal task, while the task list remains the source of truth for
visible task state.

## Related decisions

- [User-configured automation continuity](../../../decisions/2026-08-22-user-configured-automation-continuity.md)
- [Automation self-archive](../../../decisions/2026-09-02-automation-self-archive.md)
- [Automation target modes](../../../decisions/2026-08-23-automation-target-modes.md)

## Proposed coordination extension

A proposed managed-conversation destination stores a reference to shared conversation input. It does not acquire ownership of conversation cleanup.
See the [managed-conversation destination design](plugin-conversation-targets.md).
