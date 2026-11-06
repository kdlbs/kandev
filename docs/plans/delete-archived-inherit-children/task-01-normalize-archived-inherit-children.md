---
id: "01-normalize-archived-inherit-children"
title: "Normalize archived inherited children"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001
acceptance_criteria:
  - AC-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001.1
  - AC-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001.2
  - AC-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001.3
system_design:
  - ../../specs/tasks/system-design/delete-archived-inherit-children.md
---

# Task 01: Normalize archived inherited children

## Summary

Normalize archived inherited children during a non-cascade delete and report a
lost guard as a typed conflict.

## In scope

- `normalizeNoCascadeChildren` guard without `RequireTaskNotArchived`.
- `TaskDeleteChildWorkspaceConflictError` and its reason classification.
- `handleNotFound` and `wsDeleteTask` conflict mapping.
- Error text without the repeated cause, and delete log fields.

## Out of scope

- Cascade delete, preflight, and stamping paths.

## Acceptance

- An archived inherited child, with or without a marker, ends up a root task in
  `shared_group` with no marker keys and stays archived; the delete succeeds.
- A lost guard returns the typed conflict naming the child and reason, and the
  error text names the cause once.
- REST maps the conflict to `409` with `error_code`, `child_task_id`, and
  `reason`.

## Verification

```bash
# From apps/backend:
go test -trimpath -tags fts5 -p=1 ./internal/task/service -run 'TestDeleteTaskTree_NoCascade' -count=1
go test -trimpath -tags fts5 -p=1 ./internal/task/handlers -run 'TestHandleNotFound_TaskDeleteChildConflictIsConflict|TestErrorsAreClassifiable' -count=1
# From the repository root:
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/task_delete_errors.go`
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`

## Dependencies

- None.

## Risks

- See the plan.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/delete-archived-inherit-children.md)
- [System design](../../specs/tasks/system-design/delete-archived-inherit-children.md)
- [Guarded write design](../../specs/tasks/system-design/orphaned-workspace-guarded-metadata-write.md)

## Original source results

The following results belong to the original integrated source and are retained
as history. They do not claim verification of this independent upstream port.


- RED at base `e29a5b364` with the fix reverted to
  `guard.RequireTaskNotArchived = true` in a scratch worktree (new tests copied
  in unchanged): both subtests of
  `TestDeleteTaskTree_NoCascadeNormalizesArchivedInheritParentChild` failed with
  `normalize workspace mode for child delete-archived-child before delete:
  workspace metadata changed concurrently (child_workspace_changed)`. The
  lost-guard and handler tests passed, as controls.
- GREEN from `apps/backend`:
  `go test -trimpath -tags fts5 -p=1 ./internal/task/service -run 'TestDeleteTaskTree_NoCascade' -count=1`
  passed (9 tests, including both archived-child subtests and the typed-conflict
  test, `ok ... 0.820s`).
  `go test -trimpath -tags fts5 -p=1 ./internal/task/handlers -run 'TestHandleNotFound_TaskDeleteChildConflictIsConflict|TestErrorsAreClassifiable' -count=1`
  passed (`ok ... 0.088s`), so the variadic `handleNotFound` signature keeps
  every existing call site and classification working.
- `gofmt -l` on the touched files: no output.
  `bash scripts/lint-go-changed <files>`: 0 issues.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.

## Upstream port validation

The independent port reproduced both archived-child failures by restoring only
the original unarchived guard in a disposable overlay (service 0.457s). The
candidate passed the same real SQLite cases with race detection, together with
the typed lost-guard case and existing no-cascade, compensation and reparenting
controls (service 1.718s). The HTTP joined-error conflict mapping and existing
error classification checks passed in the same selected run (handlers 1.176s).

The existing documentation coverage evaluator returned covered with no errors.
The WebSocket mapping was reviewed against the same typed error/details helper.
The port adds no permanent test or collection rule; its explicit temporary
overlays are removed after delivery. Original source results above remain history.

## Review follow-up: compensation error priority

A vanished child can add a not-found error during compensation to its typed
child-deleted conflict. HTTP and WebSocket classification must retain the typed
conflict and child details instead of reporting the existing parent missing.
The change moves that classification before generic not-found while preserving
authorization, consent, and plain missing-root behavior. Product comments state
the invariant; the detailed cause remains in this work order.

The disposable baseline reproduced HTTP 404 and WebSocket NOT_FOUND for the
joined typed conflict (0.085s); plain missing-root controls passed. The candidate
returned HTTP 409 and WebSocket CONFLICT with the child ID, reason and error
code. Those four cases and existing classification, delete-authorization and
client-disconnect controls passed together with race detection (1.142s).
No standing test is added; temporary checks are removed after delivery.
