---
id: "01-implement-taskless-coordinator-authority"
title: "Implement taskless coordinator authority"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-COORDINATOR-AUTHORITY-001
  - REQ-OFFICE-COORDINATOR-AUTHORITY-002
  - REQ-OFFICE-COORDINATOR-AUTHORITY-003
  - REQ-OFFICE-COORDINATOR-AUTHORITY-004
  - REQ-OFFICE-COORDINATOR-AUTHORITY-005
acceptance_criteria:
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.1
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.2
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.3
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.4
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.5
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.6
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.7
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.8
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.9
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.10
  - AC-OFFICE-COORDINATOR-AUTHORITY-001.11
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.1
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.2
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.3
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.4
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.5
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.6
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.7
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.8
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.9
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.10
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.11
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.12
  - AC-OFFICE-COORDINATOR-AUTHORITY-002.13
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.1
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.2
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.3
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.4
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.5
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.6
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.7
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.8
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.9
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.10
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.11
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.12
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.13
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.14
  - AC-OFFICE-COORDINATOR-AUTHORITY-003.15
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.1
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.2
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.3
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.4
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.5
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.6
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.7
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.8
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.9
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.10
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.11
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.12
  - AC-OFFICE-COORDINATOR-AUTHORITY-004.13
  - AC-OFFICE-COORDINATOR-AUTHORITY-005.1
  - AC-OFFICE-COORDINATOR-AUTHORITY-005.2
  - AC-OFFICE-COORDINATOR-AUTHORITY-005.3
  - AC-OFFICE-COORDINATOR-AUTHORITY-005.4
  - AC-OFFICE-COORDINATOR-AUTHORITY-005.5
  - AC-OFFICE-COORDINATOR-AUTHORITY-005.6
system_design:
  - ../../specs/office/system-design/taskless-coordinator-authority-01.md
  - ../../specs/office/system-design/taskless-coordinator-authority-02.md
---

# Task 01: Implement Taskless Coordinator Authority

## Summary

Add a governed board-read capability and a materialized task-mutation scope
for taskless Office runs, plus a narrow workspace-scoped annotation right, so
the pre-installed "Coordinator heartbeat" routine (and any other taskless
scheduled routine) can enumerate actionable board state and flag blockers
instead of launching into a run that can see and change nothing.

## In scope

- `CapabilityListTasks` added to the closed capability vocabulary
  (`office/runtime/capabilities.go`), granted unconditionally by `FromAgent`;
  `WithTaskScope` trims and discards empty/whitespace/wildcard entries.
- `GET /runtime/tasks` (`office/runtime/tasks_list.go`,
  `office/runtime/handler.go`, `office/routes.go`): capability check, then
  workspace-claim check, then parameter parsing, then the query; ordered,
  paginated, deterministic; workspace resolved only from the signed run
  token's claim.
- Taskless task-scope materialization
  (`office/runtime/context_builder.go`): `deriveScope` /
  `deriveRunnerSet` / `reusePersistedTaskScope`, capped and deterministically
  ordered (`scopeRunnerSetCap = 500`), computed once per run and persisted
  via `BuildAndPersist` with a CAS-guarded retry
  (`persistWithCAS`, `maxScopeSwapAttempts = 3`) against concurrent
  first-build races; `runtime.scope_truncated` / `runtime.scope_unavailable`
  audit events.
- `office/runtime/context.go`: `CanMutateTask` refuses an empty or wildcard
  `taskID` before the self-match check; new `TaskScopeSource` provenance
  field.
- `office/runtime/actions.go`: the pre-existing mutation-authorization check
  is named `authorizeTaskWorkspace` (used by `UpdateTaskStatus` /
  `CreateSubtask`, unchanged in behavior); a new `canAnnotateTask` (used only
  by `PostComment`) refuses a wildcard target and branches on task-bound
  (own-task-only, matching today) vs. taskless (workspace-scoped via
  `Tasks.GetTaskWorkspaceID`, fails closed on a lookup error or missing row).
- `internal/runs/repository/sqlite/runs.go`:
  `UpdateRunRuntimeSnapshotCAS(ctx, id, prevCapabilities, capabilities,
  inputSnapshot, sessionID)` — conditional `UPDATE ... WHERE id = ? AND
  COALESCE(capabilities, '') = ?`.
- `office/service/budget_admission.go`: `resolveRunProject` trims `task_id`
  before the emptiness check, matching the taskless definition used
  elsewhere ("absent, empty, or whitespace-only, i.e. empty after
  trimming").
- `cmd/agentctl/kandev_tasks.go`: `agentctl kandev tasks list` calls the new
  runtime endpoint, one flag per accepted query parameter.

## Out of scope

- The taskless session seam (tracked separately).
- Workflow-step-move, task-archive, or step-decision capabilities.
- Widening taskless *mutation* authority beyond the runner set.

## Results

- `go build ./...`, `go vet ./...` — clean.
- `go test -count=1 ./internal/office/... ./internal/runs/... ./cmd/agentctl/...` — all `ok`.
- `go test -race -count=1 ./internal/office/runtime/... ./internal/office/repository/sqlite/... ./internal/office/service/... ./internal/runs/repository/sqlite/...` — all `ok`.
- `gofmt -l` — clean on every changed file.
- `make -C apps/backend lint` — 0 issues.
- `python3 scripts/lint-architecture.py --all` — clean.
- `python3 scripts/lint-spec-files.py --all` — all passed.
- Reviewed across 6 rounds (architecture/quality, security, test-rigor, and
  an independent cross-vendor pass); no open production-bug residual.

## Tests

- `internal/office/runtime/capabilities_test.go`,
  `internal/office/runtime/tasks_list_test.go`,
  `internal/office/runtime/tasks_list_query_test.go`,
  `internal/office/runtime/handler_test.go` — board-read capability gating,
  ordering, pagination, workspace-claim scoping (REQ-001, REQ-002).
- `internal/office/runtime/scope_derivation_test.go`,
  `internal/office/runtime/context_builder_test.go`,
  `internal/office/runtime/context_test.go`,
  `internal/runs/repository/sqlite/runs_snapshot_cas_test.go` — runner-set
  materialization, capping/ordering, CAS-guarded persistence and reuse,
  fail-closed on exhaustion (REQ-003, REQ-005).
- `internal/office/runtime/actions_annotation_test.go`,
  `internal/office/runtime/handler_taskless_annotation_test.go`,
  `internal/office/runtime/actions_test.go` — taskless workspace-scoped
  annotation, task-bound own-task-only annotation unchanged, no existence
  oracle (REQ-004).
- `internal/office/service/scheduler_taskless_launch_test.go`,
  `internal/office/service/budget_admission_test.go` — taskless launch
  integration and the whitespace `task_id` trim.
- `apps/backend/cmd/agentctl/kandev_tasks_test.go` — CLI switched to the new
  runtime endpoint.
