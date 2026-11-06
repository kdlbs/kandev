---
id: "01-implementation"
title: "feat(mcp): let direct parents reorder child tasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001
acceptance_criteria:
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.1
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.2
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.3
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.4
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.5
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.6
  - AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.7
system_design:
  - ../../specs/tasks/system-design/coordinator-child-task-ordering.md
---

# feat(mcp): let direct parents reorder child tasks

## Outcome and scope

Allow the trusted direct parent to reorder selected child tasks using existing task order and all-or-nothing membership/version checks.

Out of scope: Task hierarchy mutation, title-derived authorization, and reordering unrelated tasks.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4719 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/tasks/system-design/coordinator-child-task-ordering.md)
- [Plan](plan.md)

## Verification and results

Validation: existing MCP, service and repository reorder selections passed. Disposable checks exercised the registered MCP → dispatcher → real SQLite → event/queued-admission path, authorization/membership rejection and the reproduced parent/workspace race before and after the correction. All normal commit hooks passed. Temporary checks were removed; no standing tests, new timer or CI were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.


## Scope and prompt follow-up

The step transaction now uses the workflow owner's workspace and workflow,
matching the service's filtered band snapshot. Historical foreign rows remain
untouched and excluded from membership, results and renumbering. All four
prompt producer paths omit the task-control extension on persisted automation
run tasks while normal tasks and visible automation-created tasks keep it.
The scheduler DTO is not used as origin evidence, and fresh launch reuses its
existing canonical task read. No schema, dependency or task hierarchy changes.

Disposable real SQLite fixtures reproduced persistent conflicts plus foreign-row
exposure for mismatched workspace and workflow rows. The actual created-message
path reproduced the unavailable reorder tool in an automation-run prompt, with
ordinary and automation-created-task controls passing. Follow-up verification
results are retained with the exact PR revision; no permanent cases are added.

Focused candidate race checks passed: real mismatched-row repository cases
(1.728s), created-message origin controls (1.125s), and the four orchestrator
prompt outputs across three origins (1.549s). Existing reorder, message and
launch/target controls passed under race (6.851s / 1.213s / 1.476s). SQL guard
reported no issues. The required broader conformance run passed the SQLite task
store but failed four storage temporary-artifact scenarios: their fixture path
is rejected as non-absolute on this Windows host. The same four failures were
reproduced on unchanged ebf55 source (0.115s); they are not reported as passing
or modified by this task. PostgreSQL conformance was skipped without a DSN.
