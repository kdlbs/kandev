---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
  - REQ-OFFICE-AUTOMATION-TARGETS-002
  - REQ-OFFICE-AUTOMATION-TARGETS-003
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

## Components and responsibilities

- `internal/automation` owns `TaskMode`, `RepositoryMode`, validation,
  persistence, portable export, admission, and `AutomationRun` accounting.
- `internal/orchestrator` resolves the target before task creation. It creates
  hidden tasks with `TaskOriginAutomationRun` and visible tasks with a distinct
  visible automation origin. It records the run ID in task metadata and binds
  the exact session and turn after launch.
- `internal/task/service` receives an empty repository list for a valid scratch
  task. It requires a workflow for a visible automation task and retains the
  normal workflow-step resolver when the step is omitted.
- `internal/agent/runtime/lifecycle` continues to create a task-scoped scratch
  workspace when `RepositoryPath` is empty. It does not create a worktree or
  run repository preparation for a repository-free launch.
- `internal/mcp/profile`, `internal/mcp/scope`, and the MCP handlers apply
  `SurfaceAutomation` only to hidden automation tasks. Visible automation
  tasks use their ordinary task profile and ordinary caller boundaries.
- `apps/web/components/automations` renders the target and repository-mode
  controls, normalizes payloads, includes Local profiles, and keeps the
  desktop and mobile editor behavior aligned.

## Frontend boundary

The editor keeps target mode separate from continuity. Hidden mode explains
that its per-run tasks stay out of the Kanban and sidebar. Normal-task mode
explains that each firing is ordinary workflow work. Switching the target
clears or revalidates fields that are not valid for the new mode, and the save
payload always carries explicit target and repository modes.

On phones the existing automation editor remains the primary scroll surface.
Target and continuity choices use the existing stacked control pattern with
44-pixel touch targets. No second drawer or horizontal editor surface is
introduced. The settings toolbar gives Export and New Automation the same
height through one shared button sizing rule.

## Data and contracts

The automation model and API add two persisted user choices:

```text
task_mode: automation_run | normal_task
repository_mode: workspace_default | selected | none
```

`automation_run` and `workspace_default` are the compatibility defaults when
older clients omit the fields. New editor saves always send an explicit
repository mode. `repository_ids` is meaningful only for `selected`.

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
   normal-task mode requires a workflow. The step picker remains optional.
2. The editor presents repository mode explicitly. `none` selects Local and
   disables Worktree. `selected` resolves repository IDs. `workspace_default`
   is retained for legacy rows and is visible as a distinct fallback choice.
3. The backend validates target, workflow, repository mode, executor type, and
   continuation policy together. Invalid combinations fail before a firing is
   admitted.

### Admission and dispatch

1. `FireTrigger` renders the run title, atomically checks the shared open-run
   predicate, stores a `triggered` `AutomationRun`, and publishes its run ID.
2. The orchestrator resolves repositories from `repository_mode`. `none`
   produces an empty list for repository-neutral triggers. A provider trigger
   that requires a repository rejects `none` before admission; existing
   provider event-repository override behavior remains for the other modes.
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
profile, and repository mode/IDs. A missing or incompatible continuation gets a
replacement task and an explicit `created` or `replaced` action. The configured
workflow and step are used for a replacement. A resumed task keeps its existing
title.

## Failure and recovery

- An empty repository list is valid only for a Local-compatible executor. The
  orchestrator returns a validation error before task creation for Worktree.
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

Add `task_mode` and `repository_mode` columns to `automations`. Fresh schemas
use the compatibility defaults. Existing rows migrate to
`task_mode=automation_run` and `repository_mode=workspace_default`, preserving
the prior empty-repository fallback. The current automation editor writes
`repository_mode=none` when the person explicitly chooses no repository.

Portable YAML includes both user choices and repository IDs, but excludes
`continuation_task_id`, run bindings, rendered titles, session pointers, and
cleanup jobs. Import normalizes omitted target fields to the compatibility
defaults and revalidates executor/workflow combinations.

Changing target mode, repository mode, repository IDs, workflow, workflow step,
agent profile, or executor profile rotates the continuation on the next firing.
Name, description, prompt, title template, enabled state, and trigger edits do
not change the existing task until a firing needs a new or replacement task.

## Security

The task mode is server-owned configuration, not an MCP argument. Hidden
automation identity is derived only from a task whose origin and metadata bind
it to the automation. Visible automation tasks are not accepted as hidden
coordinator callers. Workspace authorization and exact run identity remain
required for all automation mutations.

## Observability

Run records expose target mode through the owning automation and preserve exact
task/session/turn IDs. Dispatch logs include target mode, repository mode,
thread action, and failure reason without logging prompt contents or secrets.
The editor and run history expose whether a run produced a hidden task or a
visible normal task, while the task list remains the source of truth for
visible task state.

## Related decisions

- [User-configured automation continuity](../../../decisions/2026-08-22-user-configured-automation-continuity.md)
- [Automation target modes](../../../decisions/2026-08-23-automation-target-modes.md)
