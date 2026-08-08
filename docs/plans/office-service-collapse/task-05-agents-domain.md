---
id: "05-agents-domain"
title: "Delete the facade's agents mirror and repoint scheduler"
status: pending
wave: 3
depends_on: ["01-duplication-detector"]
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 05: Agents Domain and the Scheduler Repoint

The only CRUD domain with a live facade consumer. `office/scheduler` reaches
three of these methods through its `svc *service.Service` field
(`scheduler/run.go:154`), so this task is a repoint, not just a deletion — which
is why it sits in wave 3 while the other leaf domains sit in wave 2.

## Scope

Delete `internal/office/service/agents.go` (313 LOC) and the agent readers in
`config_read.go`. **Seven** identical groups are removed here:
`GetAgentFromConfig` `ListAgentInstancesFiltered` `UpdateAgentInstance`
`UpdateAgentStatus` `validateReportsTo` `validateStatusTransition`, and
`countAgentsByRole` → `countAgentsByRoleInWorkspace` (**renamed**; the text
heuristic missed this one).

The eighth agents-domain group, `CreateDefaultInstructions`, is owned by
`office/agents` but lives in `internal/office/service/instructions.go` — it is
deleted by **task 07**, not here. Do not double-count it.

### D3 — a real fix comes with the deletion

`service/agents.go:275` `validateAgentNameUnique` swallows the repository error:

```go
exists, err := s.repo.AgentInstanceExistsByName(ctx, workspaceID, name, excludeID)
if err != nil {
    return nil          // ← facade: admits a duplicate name on any query failure
}
```

`agents/service.go:804` returns `fmt.Errorf("check agent name uniqueness: %w", err)`.
**`office/agents` is correct.** Deleting the facade copy fixes this by
construction; add a test in `agents/service_test.go` that injects a failing
repository and asserts the error propagates, so the fix is pinned rather than
incidental.

## The scheduler repoint

Three call sites hold `*service.Service` and must move to `*agents.AgentService`:

| Site | Call | Target |
| --- | --- | --- |
| `scheduler/run_processing.go:43` | `ss.svc.GetAgentFromConfig` | `agents.AgentService.GetAgentFromConfig` |
| `scheduler/run.go:316` | `ss.svc.GetAgentFromConfig` | same |
| `scheduler/retry.go:73` | `ss.svc.GetAgentFromConfig` | same |
| `scheduler/retry.go:109` | `ss.svc.ListAgentInstancesFiltered` | `agents.AgentService.ListAgentInstancesFiltered` |
| (also) | `ss.svc.ListAgentInstances` | `agents/service.go:213`, already forwards |

`agents/service.go:207,213` already forward `GetAgentInstance` →
`GetAgentFromConfig` and `ListAgentInstances` → `ListAgentsFromConfig` with
matching signatures, so the seam exists. Add a narrow interface on
`SchedulerService` (e.g. `agentReader`) rather than a concrete
`*agents.AgentService` field, to keep `scheduler` testable and avoid widening the
`scheduler → agents` edge.

`scheduler/retry.go:110` also references `service.AgentListFilter`. Check whether
`agents` exports an equivalent; if not, alias it in `agents` rather than keeping
the `scheduler → service` import alive purely for a type name.

`ss.svc.LogActivityWithRun` (`scheduler/retry.go:85`) is **task 08**, not this
one. Leave it.

## Test migration

`office/agents` has 7 test files / 1,240 LOC; the facade has `agents_test.go`
(249 LOC). The **`office/agents` suite survives**. Before deleting
`service/agents_test.go`, diff its assertions against `agents/service_test.go`
and port anything not already covered — in particular any `validateReportsTo` /
`validateStatusTransition` / `UpdateAgentStatus` transition cases. Record in
`## Results` which assertions were ported and which were already duplicated.

## Acceptance

1. Detector Section A drops by **7** groups; Section B same-name pairs drop by **3**
   (`validateAgentCreate` 0.983, `validateAgentUpdate` 0.979,
   `validateAgentNameUnique` 0.845).
2. `internal/office/scheduler` no longer calls `svc.GetAgentFromConfig`,
   `svc.ListAgentInstances`, or `svc.ListAgentInstancesFiltered`.
3. A new test pins D3: a failing `AgentInstanceExistsByName` produces an error,
   not a silent pass.
4. No assertion lost from `service/agents_test.go`.

## Verification

```bash
cd apps/backend && go test ./internal/office/agents/... ./internal/office/scheduler/... -count=1 -v
cd apps/backend && go test ./internal/office/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m

# confirm the repoint actually happened:
grep -rn 'svc\.GetAgentFromConfig\|svc\.ListAgentInstances' apps/backend/internal/office/scheduler/   # expect no hits
```

## Files likely touched

- deleted: `internal/office/service/agents.go`, `internal/office/service/agents_test.go`
- `internal/office/service/config_read.go` (agent readers)
- `internal/office/scheduler/run.go`, `run_processing.go`, `retry.go`
- `internal/office/agents/service.go` (possible `AgentListFilter` alias)
- `internal/office/agents/service_test.go` (D3 test + ported assertions)
- `internal/office/services.go`, `internal/backendapp/main.go` (scheduler wiring)

## Dependencies

Task 01. Independent of tasks 02–04, but shares `office/services.go` wiring with
task 03 — sequence after it if both are in flight.

## Parallelism

`sequential`.

## Rollback position

Single revert. The scheduler repoint and the `agents.go` deletion **must be one
commit** — reverting only the deletion would leave `scheduler` pointing at
methods that still exist but are no longer the ones under test.

## Output contract

Summary, files changed, detector delta, the ported-assertion list, D3's new test
with its pre-fix failure output, and the grep showing zero remaining scheduler
calls into the facade's agent methods.

## Results

Pending.
