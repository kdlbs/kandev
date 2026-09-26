---
created: 2026-09-27
status: planned
requirements:
  - REQ-OFFICE-AGENT-COMMENT-READS-001
  - REQ-OFFICE-AGENT-COMMENT-READS-009
system_design:
  - ../../specs/office/system-design/agent-comment-reads-01.md
legacy_specs: []
---

# Implementation Plan: Taskless coordinator comment reads

## Overview

Issue [#3976](https://github.com/kdlbs/kandev/issues/3976): the Office
coordinator heartbeat runs every five minutes, every
`GET /api/v1/office/tasks/:id/comments` it makes returns `403`, and the run
still completes cleanly.

**Root cause.** The heartbeat is a lightweight routine, so every fire is a
taskless run (`office_run_sessions`, no task session). Its runtime JWT is
minted with `TaskID == ""` (`SchedulerIntegration.mintRuntimeToken`). The agent
comment read goes through `HandoffService.ListCommentsForCaller`, whose
`canReadDocuments` → `loadAccessPair` denies any empty caller task. That is
what AC-OFFICE-AGENT-COMMENT-READS-001.13 specified on 2026-08-25, before
taskless runs existed. The later taskless coordinator authority contract gave
the same run a workspace-wide board read and workspace-wide annotation, but
never covered reads. So the coordinator can post on a task's thread but cannot
read it.

**Silent success.** Refusals on the runtime action surface already append
`runtime.denied` to the run. The comment read lives on the dashboard route and
records nothing, so a run in which every read was refused looks clean in run
history.

**Reproduction.** A temporary HTTP test minted a runtime JWT with an empty task
claim, a run id and the workspace claim, then read a same-workspace task:
`403 {"error":"document access denied"}`. That body is 34 bytes, matching the
`"bytes": 34` in the issue's log line. The existing
`TestListCommentsForCallerDeniesEveryTargetWhenCallerTaskEmpty` pins the
service-level denial.

**Not defects.** The issue's other refusals match current contracts. A `403`
on `POST /runtime/tasks` is a parented create outside the task scope
(AC-OFFICE-COORDINATOR-AUTHORITY-004.10). A `409` on `.../status` is a status
conflict. The `Agent stream event missing task_id` warnings are expected for a
run-owned execution, which has no task conversation
(`taskless-run-sessions.md`, "Events and observation"). The single `500` has no
evidence to diagnose and is excluded.

## Specifications

- Amended: [agent-comment-reads.md](../../specs/office/requirements/agent-comment-reads.md) (AC-OFFICE-AGENT-COMMENT-READS-001.13).
- Added: [taskless-comment-reads.md](../../specs/office/requirements/taskless-comment-reads.md) (REQ-OFFICE-AGENT-COMMENT-READS-009).
- Design: [agent-comment-reads-01.md](../../specs/office/system-design/agent-comment-reads-01.md), sections "Taskless run callers", "Security", "Observability".

## Decisions (user, 2026-09-27)

- A taskless run reads comments on any task in its own workspace, the same reach
  it has for annotation. Task-bound runs are unchanged.
- A refused agent read with a run id appends a `runtime.denied` run event. The
  run's outcome is not changed.

## Scope

- Taskless branch in the agent comment read (task service plus dashboard
  handler).
- `runtime.denied` event on refused agent reads.
- Coordinator heartbeat instruction to read comments.

## Exclusions

- Failing a run because it hit a denial.
- Persisting a taskless run's transcript, or quieting the watcher warning.
- The unguarded task document routes (already out of scope in the design).
- Any change to runtime task creation, status updates, or task-bound reach.

## Work orders

Sequential; 02 depends on 01.

- [Task 01: Taskless run comment reads](task-01-taskless-run-comment-reads.md)
- [Task 02: Record refused agent comment reads](task-02-record-refused-comment-reads.md)

## Verification

```bash
cd apps/backend
go test ./internal/task/service/ -run 'ListComments' -count=1
go test ./internal/office/dashboard/ -run 'Comment' -count=1
go test ./internal/backendapp/ -run 'Office' -count=1
golangci-lint run ./internal/task/service/... ./internal/office/dashboard/... ./internal/backendapp/... --timeout=5m
cd ../..
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```
