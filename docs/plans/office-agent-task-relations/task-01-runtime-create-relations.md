---
id: "01-runtime-create-relations"
title: "Accept relations on runtime task create"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-TASK-RELATIONS-001
  - REQ-OFFICE-AGENT-TASK-RELATIONS-003
  - REQ-OFFICE-AGENT-TASK-RELATIONS-004
acceptance_criteria:
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.1
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.2
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.3
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.4
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.5
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.6
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.7
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.8
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.1
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.2
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.5
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.6
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.7
  - AC-OFFICE-AGENT-TASK-RELATIONS-004.3
system_design:
  - ../../specs/office/system-design/agent-task-relations.md
---

# Task 01: Accept Relations on Runtime Task Create

## Summary

Make `POST /api/v1/office/runtime/tasks` accept `blocked_by` and
`workspace_mode`. Validate and scope them in `internal/office/runtime`, and
carry them through the Office service and `backendapp` adapter. `CreateChildTask`
honors an explicit mode; an omitted mode keeps today's `inherit_parent`.

## In scope

- Shrink `CreateTaskInput.unsupportedField` to `priority`,
  `workspace_group_id`, `default_child_workspace`, and `default_child_ordering`.
- Normalize and validate the two fields in `Actions.CreateTask`, using the new
  `ErrInvalidTaskRelations` mapped to `400` with `runtime.denied`.
- Blocker scope: `CanMutateTask` plus a same-workspace check, otherwise the new
  `TaskReadRelation.CanReadTask`. Every denial returns `ErrBlockerOutOfScope`
  (`403`, `runtime.denied`, one message).
- Add exported `HandoffService.CanReadTask` delegating to `canReadDocuments`,
  and wire it into `runtime.ActionDependencies` in `backendapp`.
- Add `shared.AgentTaskRelations{BlockedBy, WorkspaceMode}`. Extend the runtime
  `TaskCreator`, the Office service create methods, the Office `TaskCreator` /
  `SubtaskCreator`, and the `backendapp` adapter to carry it next to
  `causingRunID`.
- Root create: set `CreateTaskRequest.BlockedBy`, and `WorkspacePolicy{Mode}`
  only when a mode is given. Subtask create: extend `ChildTaskSpec` with
  `BlockedBy` and `WorkspaceMode`; `CreateChildTask` uses the mode when set and
  `inherit_parent` otherwise.
- In the adapter, map `taskservice.ErrTaskReferenceNotFound` to
  `ErrBlockerOutOfScope`.

## Out of scope

- When the created task first wakes (`REQ-OFFICE-SCHEDULER-002`).
- CLI flags, skills, prompts, and docs (work order 02).
- `default_child_workspace`, `shared_group`, priority, ordering policy, edge
  changes on existing tasks, and any change to omitted-mode resolution.

## Acceptance

- Runtime tests show that root and child creates with permitted blockers reach
  the creator with deduplicated IDs, and that malformed relations return `400`
  without any lookup or creator call. Missing, foreign-workspace, and unrelated
  blockers return the same `403` message, record `runtime.denied`, and make no
  creator call. A parent named as a blocker of its new child is accepted.
- Task-service tests show `CreateChildTask` recording `new_workspace` with no
  group attachment, explicit `inherit_parent`, an omitted mode still producing
  `inherit_parent` even when the parent records a `default_child_workspace`
  (unchanged behavior), workflow-engine children unchanged, and blocker edges
  whose post-insert failure rolls the task back.
- Adapter tests show root and subtask relations reaching
  `CreateTaskRequest` / `ChildTaskSpec`, and a deleted-blocker race surfacing
  as `ErrBlockerOutOfScope`.

## Verification

Write each test first and confirm it fails for the expected reason (for
example, `blocked_by is not supported` or the child recorded as
`inherit_parent`). Then run, from `apps/backend`:

```bash
rtk go test ./internal/office/runtime/... -run 'TaskRelations|CreateTask' -race -count=1
rtk go test ./internal/task/service/... -run 'CreateChildTask|CanReadTask|Blocker' -race -count=1
rtk go test ./internal/backendapp/... -run 'TaskCreatorAdapter|OfficeTask' -race -count=1
rtk go test ./internal/office/service/... -run 'CreateOffice' -race -count=1
rtk go test ./internal/office/runtime/... ./internal/office/service/... ./internal/task/service/... ./internal/backendapp/... -race -count=1
rtk go run ./cmd/sqlguard ./internal
```

## Files likely touched

- `apps/backend/internal/office/runtime/actions.go`
- `apps/backend/internal/office/runtime/errors.go`
- `apps/backend/internal/office/runtime/handler.go`
- `apps/backend/internal/office/runtime/actions_task_relations_test.go` (new)
- `apps/backend/internal/office/runtime/handler_task_relations_test.go` (new)
- `apps/backend/internal/office/shared/` (new `AgentTaskRelations` type)
- `apps/backend/internal/office/service/service.go`
- `apps/backend/internal/office/service/task_creator_test.go`
- `apps/backend/internal/task/service/service_child_task.go`
- `apps/backend/internal/task/service/service_child_task_test.go`
- `apps/backend/internal/task/service/handoff_service.go` (`CanReadTask`)
- `apps/backend/internal/task/service/handoff_access_test.go`
- `apps/backend/internal/backendapp/adapters_office.go`
- `apps/backend/internal/backendapp/adapters_office_test.go`
- Runtime dependency wiring in `apps/backend/internal/backendapp/`

## Dependencies

None.

## Risks

- `actions_test.go` (1,302 lines) and `handler_test.go` (976 lines) exceed the
  revive file limit for additions. Put new tests in new files.
- `CreateChildTask` is shared with workflow-engine children. The new fields
  must be inert when empty; pin that with a test.
- Check blocker scope after the parent check, and never echo a blocker ID in an
  error. Existence must not leak.
- Keep runtime functions within Go lint limits (80 lines, complexity 15).
  Extract `normalizeTaskRelations` and `validateBlockerScope` helpers.

## Parallelism

Sequential, first.

## Inputs

- `docs/specs/office/requirements/agent-task-relations.md`
- `docs/specs/office/system-design/agent-task-relations.md`: sections Runtime
  input and validation, Blocker scope, Creation path.
- Patterns: the existing parent/assignee checks in `validateTaskRelations`, the
  MCP `WorkspacePolicy` handling in `internal/mcp/handlers/handlers.go`, and
  `canReadDocuments` in `handoff_access.go`.

## Results

- Runtime create accepts `blocked_by` and `workspace_mode`; `default_child_workspace`
  joins `priority`, `workspace_group_id`, and `default_child_ordering` as rejected
  fields. Validation lives in `internal/office/runtime/task_relations.go`
  (`normalizeTaskRelations`, `validateBlockerScope`), with `ErrInvalidTaskRelations`
  (400) and `ErrBlockerOutOfScope` (403).
- Blocker scope uses `CanMutateTask` plus a same-workspace check, otherwise
  `HandoffService.CanReadTask`, wired through `ActionDependencies.TaskReads` in
  `internal/office/routes.go` with a typed-nil guard.
- `shared.AgentTaskRelations` is carried next to `causingRunID` through the runtime,
  Office service, and `backendapp` adapter. `ChildTaskSpec` gained `BlockedBy` and
  `WorkspaceMode`; an empty mode keeps `inherit_parent`, and a test pins that a
  parent's `default_child_workspace` is still ignored on this path.
- The adapter maps a vanished blocker (`ErrTaskReferenceNotFound`) to
  `ErrBlockerOutOfScope`.
- Tests: `actions_task_relations_test.go`, `handler_task_relations_test.go`,
  `service_child_task_relations_test.go`, `handoff_can_read_task_test.go`,
  `task_creator_relations_test.go`, `adapters_office_relations_test.go`.
- Verification: `go test -race` passed for `internal/office/runtime`,
  `internal/office/service`, `internal/task/service`, and `cmd/agentctl`;
  targeted `internal/backendapp` adapter/scope tests passed under `-race`.
  `golangci-lint --new-from-rev=origin/main` reported 0 issues. No SQL changed, so
  `sqlguard` was not needed.
- TDD note: the handler-level tests were written after the action layer was
  implemented, so they were not observed failing first.
