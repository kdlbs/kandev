---
created: 2026-09-06
status: implemented
requirements:
  - REQ-OFFICE-COORDINATOR-AUTHORITY-001
  - REQ-OFFICE-COORDINATOR-AUTHORITY-002
  - REQ-OFFICE-COORDINATOR-AUTHORITY-003
  - REQ-OFFICE-COORDINATOR-AUTHORITY-004
  - REQ-OFFICE-COORDINATOR-AUTHORITY-005
system_design:
  - ../../specs/office/system-design/taskless-coordinator-authority-01.md
  - ../../specs/office/system-design/taskless-coordinator-authority-02.md
legacy_specs: []
---

# Implementation Plan: Taskless Coordinator Authority

## Overview

The pre-installed "Coordinator heartbeat" routine is a taskless (no
`task_template`) scheduled run whose job is to monitor the workspace, surface
blockers, and react to events with no human driving the loop. Today it cannot
do that job: the runtime capability vocabulary has no board-read key at all,
and a taskless run's task-mutation scope is computed as `AllowedTaskIDs =
[""]`, which matches no real task id or wildcard. The run launches, sees
nothing, and can change nothing.

This plan implements
[`taskless-coordinator-authority-01.md`](../../specs/office/system-design/taskless-coordinator-authority-01.md)
(capability vocabulary, task-scope derivation, annotation predicate) and
[`-02.md`](../../specs/office/system-design/taskless-coordinator-authority-02.md)
(board-read wire contract) end to end in a single work order: a new
`list_tasks` capability and `GET /runtime/tasks` endpoint scoped strictly to
the run token's workspace claim, a materialized "runner set" task scope for
taskless runs in place of the dead `[""]` placeholder, and a narrow
workspace-scoped annotation right (`post_comment` on any task in-workspace)
for taskless runs only, with state mutation staying limited to task scope
exactly as today.

## Scope

### In scope

- `CapabilityListTasks` in the closed capability vocabulary, granted
  unconditionally by `FromAgent`.
- `GET /runtime/tasks`: capability-gated, paginated, ordered, deterministic
  board read whose workspace comes only from the signed run token's claim.
- Taskless task-scope materialization from the agent's runner set (bounded,
  deterministically ordered, computed once per run, reused verbatim on
  re-mint via a compare-and-swap guard against concurrent first-build races).
- `canAnnotateTask`: taskless runs may `post_comment` on any task in their own
  workspace; task-bound runs keep their existing own-task-only reach.
- No existence oracle: annotation and mutation paths on an out-of-scope or
  cross-workspace task return the same denial regardless of whether the task
  exists.
- `agentctl kandev tasks list` switched to the new runtime endpoint.

### Out of scope

- The taskless session seam (how the run's session is created in the first
  place) — tracked separately, not part of this diff.
- Workflow-step-move, task-archive, or step-decision capabilities — these stay
  explicitly denied.
- Widening a taskless run's *mutation* authority to its whole workspace — only
  board-read and annotation are workspace-scoped; state mutation stays
  limited to the run's own runner set.

## Technical approach

See the system design documents for the full contract. In short:

- `office/runtime/capabilities.go` adds the `list_tasks` key to the closed
  vocabulary and trims/discards empty or wildcard entries passed to
  `WithTaskScope`.
- `office/runtime/context_builder.go` adds scope derivation
  (`deriveScope`/`deriveRunnerSet`/`reusePersistedTaskScope`), a
  CAS-guarded persistence path (`BuildAndPersist` /
  `persistWithCAS`, capped retries), and a scope-truncation/unavailable audit
  event.
- `office/runtime/actions.go` splits the pre-existing mutation-authorization
  check (`authorizeTaskWorkspace`, used by `UpdateTaskStatus` /
  `CreateSubtask`) from the new annotation predicate (`canAnnotateTask`, used
  only by `PostComment`), which branches on task-bound (own-task-only) vs.
  taskless (workspace-scoped) runs.
- `office/runtime/tasks_list.go` implements the `GET /runtime/tasks` handler:
  capability check, then workspace-claim check, then parameter parsing, then
  the query — in that order, so a malformed request from an unauthorized
  caller is rejected on capability first.
- `internal/runs/repository/sqlite/runs.go` adds
  `UpdateRunRuntimeSnapshotCAS` for the compare-and-swap persistence path.
- `office/service/budget_admission.go`'s `resolveRunProject` trims `task_id`
  before the emptiness check, closing a regression a newer upstream
  pre-launch budget gate would otherwise have hit against a whitespace-only
  `task_id`.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-OFFICE-COORDINATOR-AUTHORITY-001.*` | `internal/office/runtime/capabilities_test.go`, `internal/office/runtime/tasks_list_test.go`, `internal/office/runtime/handler_test.go` — capability gating, audit logging of accepted/denied calls |
| `AC-OFFICE-COORDINATOR-AUTHORITY-002.*` | `internal/office/runtime/tasks_list_query_test.go`, `internal/office/runtime/tasks_list_test.go` — ordering, pagination bounds, determinism |
| `AC-OFFICE-COORDINATOR-AUTHORITY-003.*` | `internal/office/runtime/scope_derivation_test.go`, `internal/office/runtime/context_builder_test.go`, `internal/runs/repository/sqlite/runs_snapshot_cas_test.go` — runner-set materialization, capping/ordering, CAS-guarded persistence and reuse |
| `AC-OFFICE-COORDINATOR-AUTHORITY-004.*` | `internal/office/runtime/actions_annotation_test.go`, `internal/office/runtime/handler_taskless_annotation_test.go`, `internal/office/runtime/actions_test.go` — taskless workspace-scoped annotation, task-bound own-task-only annotation unchanged, no existence oracle |
| `AC-OFFICE-COORDINATOR-AUTHORITY-005.*` | `internal/office/runtime/capabilities_test.go`, `internal/office/runtime/context_test.go` — denied-authority invariants (no step-move/archive/decision keys; wildcard scope never matches; whitespace `task_id` trimmed) |

Also: `internal/office/service/scheduler_taskless_launch_test.go` (taskless
launch integration), `internal/office/service/budget_admission_test.go`
(whitespace `task_id` trim), `apps/backend/cmd/agentctl/kandev_tasks_test.go`
(CLI switched to the new endpoint).

## E2E tests

None. This is a backend runtime-authorization contract with no `apps/web`
change (`git diff --stat -- apps/web` is empty for this diff).

## Work orders

- [done] [Task 01: Implement Taskless Coordinator Authority](task-01-implement-taskless-coordinator-authority.md)

## Dependency order

```text
Task 01
```

Single task; the capability, scope-derivation, and annotation changes land
together because the scope-derivation change is what makes the annotation
predicate's taskless branch meaningful (an annotation right with no
materialized scope to reason about would be untestable in isolation).

## Verification results

- `go build ./...`, `go vet ./...` — clean.
- `go test -count=1 ./internal/office/... ./internal/runs/... ./cmd/agentctl/...` — all packages `ok`.
- `go test -race -count=1 ./internal/office/runtime/... ./internal/office/repository/sqlite/... ./internal/office/service/... ./internal/runs/repository/sqlite/...` — all `ok`.
- `gofmt -l` on every changed Go file — clean.
- `make -C apps/backend lint` (golangci-lint) — 0 issues.
- `python3 scripts/lint-architecture.py --all` — clean.
- `python3 scripts/lint-spec-files.py --all` — all passed.
- Reviewed across 6 rounds (architecture/quality, security, test-rigor, and
  an independent cross-vendor pass) with no open production-bug residual.

## Risks

- The taskless annotation widening (comment on any task in-workspace) is a
  deliberate, narrow authority increase reviewed specifically for
  existence-oracle and cross-workspace leakage; state-mutation authority is
  unchanged in shape from today's task-bound behavior.
- The CAS-guarded scope persistence caps retries
  (`maxScopeSwapAttempts = 3`); a reviewer should confirm the fail-closed
  behavior on exhaustion (empty scope, not a stale or partial one) still
  holds if that constant changes.

## Open questions

None outstanding.

## Package handoff

Implementation is complete on branch `feature/office-beta-taskless-6n8`. This
plan and its work order were added to satisfy the repository's PR
documentation coverage gate for the non-doc, non-test source files this diff
touches; the implementation itself predates this plan file and was developed
directly from the requirements and system-design documents linked above.
