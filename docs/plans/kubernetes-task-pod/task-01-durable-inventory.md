---
id: "01-durable-inventory"
title: "Durable environment inventory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.2
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.4
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.5
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.6
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 01: Durable environment inventory

## Summary

Add the durable environment-level Kubernetes inventory and conditional operation
claims as a compatible expansion. Existing lifecycle behavior remains unchanged
until Task 02 wires the new record into allocation and cleanup.

## In scope

- Typed runtime record, secret references, schema initialization and replayable migration.
- Transactional create/recover/delete claims and generation checks on both dialects.
- Legacy inventory classification is integrated and tested in Task 02 at the lifecycle consumer.

## Out of scope

Cross-task pooling, automated legacy consolidation, and UI redesign.

## Acceptance

- Concurrent claims elect one owner and cannot overlap deletion with attachment.
- Restart replay preserves exact inventory and secret references; migration deletes no remote resources or legacy rows.
- Changed ownership generations and stale cleanup operations cannot overwrite or release inventory.

## Verification

Use TDD for changed logic: run the named regression before and after implementation.
Commands run from repository root. Read scoped guidance and applicable TDD/E2E
skills before implementation.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'KubernetesEnvironment' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/task/repository/sqlite -run 'KubernetesEnvironment' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/environment.go`
- New `apps/backend/internal/task/repository/sqlite/environment_kubernetes.go` and `_test.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go` and schema/replay fixtures
- Task repository interface and lifecycle/backendapp adapters needed by the typed record

## Dependencies

None.

## Risks

Claims require real PostgreSQL multi-connection evidence using `KANDEV_TEST_POSTGRES_DSN`; an unset variable is skipped coverage, not a pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/kubernetes-task-pod.md).
- [System design](../../specs/executors/system-design/kubernetes-task-pod.md).
- [Plan](plan.md) regression matrix.
- Existing `executor_kubernetes_fakes_test.go`, checkpoint/restart tests, and
  task environment repository tests as patterns.

## Results

Passed targeted KubernetesEnvironment tests with SQLite and disposable PostgreSQL,
including the race detector. Passed `go run ./cmd/sqlguard ./internal` and
`go test -race ./internal/persistence/storeconformance -count=1`.

Initial inventory regression failed with missing table; claim regression failed
with missing repository capability; stale-generation checkpoint failed because
it accepted an outdated owner. Each passed after its implementation.
Checks used `GOCACHE=/tmp/kandev-kube-pod-gocache-87d89c84` because unrelated
shared-cache removal interrupted the first broad compiler runs.
PostgreSQL used an isolated Docker container on loopback port 36019.


Live validation exposed the task-delete cascade preceding asynchronous cleanup.
The regression failed with a foreign-key error and now passes on SQLite and
PostgreSQL: physical inventory survives task deletion, normal launch claims
reject deleted owners, and cleanup claims retain exact generation/revision
fencing. Direct environment deletion remains guarded until remote cleanup.

## PR remediation

PR remediation adds task_environment_kubernetes to the required-table probe; SQLite conformance and previous-stable upgrade tests pass with race detection.
