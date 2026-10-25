---
status: draft
system: tasks
requirements:
  - REQ-TASKS-DEFERRED-START-STEP-ENTRY-001
  - REQ-TASKS-TASK-DEPENDENCIES-001
---

# Deferred Start on Step Entry System Design

## Purpose and boundaries

The orchestrator's automatic start path consumes a task's `deferred_launch`
record. This design limits when a `task.moved` entry may consume it.
[Workflow step agent start ownership](workflow-step-agent-start-ownership.md)
owns step prompts and start races. The session ceiling and WIP queue own their
own deferred records' replay.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-DEFERRED-START-STEP-ENTRY-001` | [Control flow](#control-flow) |
| `REQ-TASKS-TASK-DEPENDENCIES-001` | [Prepared primary on deferred start](#prepared-primary-on-deferred-start) |

## Components and responsibilities

- `handleTaskMoved` in `orchestrator/event_handlers_workflow.go` starts a moved
  task without a session through `autoStartTaskForLoadedStep` with the
  destination step it already loaded.
- `autoStartTaskForStep` handles automatic starts that only know the step ID,
  including `task.moved` fallbacks, dependency resolution, and the watcher.
- `autoStartTaskForLoadedStep` settles auto-start-on-create tokens, consumes the
  deferred record through `launchDeferredTask`, and checks the step's
  `auto_start_agent` action.

## Data and contracts

The step-entry decision depends on the event name and loaded step. Prepared
primary reuse adds only an internal launch option; no wire or record format
changes.

## Control flow

`autoStartTaskForStep` called `launchDeferredTask` before reading the
destination step, and `autoStartTaskForLoadedStep` called it before its own
`auto_start_agent` check. A move into a hold step therefore launched the
record and removed it, on both paths.

`autoStartTaskForLoadedStep` now skips `launchDeferredTask` for `task.moved`
into a step without `auto_start_agent`. Tokens are still settled, the skipped
start is logged, and the record stays (`001.1`). An entry into an agent step
consumes it as before (`001.2`).

`autoStartTaskForStep` no longer calls `launchDeferredTask` before the step
read for `task.moved`; `autoStartTaskForLoadedStep` decides instead. When the
step read fails, it still calls `launchDeferredTask` before it returns,
because a failed read is no evidence that the step has no agent (`001.3`).
Other events keep the early call, and queue promotion, dependency resolution,
and creation keep their behavior (`001.3`).

### Prepared primary on deferred start

A start-when-unblocked intent may encounter the workspace-only `CREATED`
primary prepared when the blocked task was opened. After resolving the workflow
recipient and effective profile, the deferred start reuses that primary only
when its profile and executor settings match and it has no messages, turns,
initial-prompt submission, provider conversation, resume token, or committed
harness generation. An omitted executor ID retains the prepared selection, as
`LaunchPreparedSession` already does; an explicit different executor is not
eligible. The existing lifecycle lock protects the eligibility read and launch.
A workspace
execution alone does not make the conversation used. Its runtime may still
report startup liveness; the executor serializes workspace preparation and the
first agent launch with its existing session lock.

Explicit workflow targets and Office sessions retain their established
selectors. Any starting/running session or live sibling agent prevents this
prepared primary claim, and launch failure restores the existing deferred
intent. A non-primary, failed, terminal, or otherwise ineligible session is not revived
or promoted. Ordinary starts retain their creation behavior. The dependency consumer currently leaves automatic activation unset and retains
existing manual admission at a full session ceiling. It does not create a
ceiling-deferred start; this repair adds no ceiling payload/replay field or
admission-policy change. The selected
session retains its queue incarnation and goes through normal launch admission;
MCP moves continue to select the resulting active primary and defer until its
turn ends. This adds no caller-supplied move authority or checkout refresh.

## Failure and recovery

A kept record is consumed by the next eligible entry or by an explicit start.
No retry or sweep is added.

## Persistence

No schema change. The record is left untouched in task metadata.

## Security

No permission or trust boundary changes.

## Observability

A debug log names the task and step whose deferred start was kept.
