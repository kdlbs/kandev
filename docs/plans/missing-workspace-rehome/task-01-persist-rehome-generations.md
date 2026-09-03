---
id: "01-persist-rehome-generations"
title: "Persist atomic rehome claims and loss evidence"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-001
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
acceptance_criteria:
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.2
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.3
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.1
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.2
system_design:
  - ../../specs/tasks/system-design/missing-workspace-rehome.md
---

# Task 01: Persist atomic rehome claims and loss evidence

## Summary

Create the durable loss-assessment and environment-binding rehome claim. Prove
the transaction elects one materialization owner while retaining all
task/workflow/profile identity.

## In scope

- Atomic claim repository API and task cleanup/snapshot-lock enforcement.
- Phase-current loss assessment based on Git status and remote reachability.
- Complete multi-repository inventory and concurrent-claim tests.

## Out of scope

- Calling lifecycle launch or rendering errors.
- Deleting superseded environments or paths.

## Acceptance

- The existing environment binding retains its identity while stale physical
  handles are cleared for fresh materialization.
- Concurrent equivalent claims elect one materialization owner.
- Unknown or unique repository evidence cannot win an automatic claim.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/task/service -run 'Test.*(Rehome|LossAssessment)'
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_rehome.go`
- `apps/backend/internal/task/repository/sqlite/git_snapshots.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/service/service_turns.go`

## Dependencies

None.

## Risks

- SQLite transaction serialization and PostgreSQL advisory locking differ.
- Shared-group and inherited workspace bindings must remain fail-closed.

## Parallelism

`sequential`

## Inputs

- Missing Workspace Rehome requirements and system design.
- Existing workspace-binding election and environment-owned Git snapshots.

## Results

Implemented a schema-free compare-and-swap on the existing environment binding.
Automatic recovery now requires clean completion evidence for every current
repository partition, from the launching session, while holding the snapshot
environment lock. Missing, stale, malformed, mixed clean/dirty, and incomplete
inventories fail closed; concurrent claims elect exactly one owner.
