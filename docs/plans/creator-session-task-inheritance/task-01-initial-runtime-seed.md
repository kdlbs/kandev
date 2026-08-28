---
id: "01-initial-runtime-seed"
title: "Initial runtime seed"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/mcp-task-agent-profile-default.md"
---

# Task 01: Initial runtime seed

## Acceptance

- A typed helper returns the source session's effective model, mode, and option
  values with profile, provider, and explicit-override precedence.
- A task launch seed becomes runtime overrides only on the initial prepared
  session. Later sessions do not receive it.
- All copied maps are independent, and the task-only launch key does not remain
  in session metadata.

## Verification

```bash
cd apps/backend && go test ./internal/task/models ./internal/orchestrator/executor
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/session_runtime_config_test.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_test.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/session_test.go`
- `apps/backend/internal/task/repository/sqlite/task.go`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the metadata contract from this task.

## Inputs

- Spec: **What**, **Data model**, **Persistence guarantees**
- Plan: **Initial-session runtime seed**
- `docs/decisions/2026-07-18-turn-configuration-snapshots.md`
- `docs/decisions/2026-08-01-workflow-session-original-configuration.md`

## Output contract

Report the metadata key and merge precedence, files changed, exact tests run,
results, blockers, risks, and synchronized task/plan status.

## Results

Implemented the typed `MetaKeyInitialSessionRuntimeConfig` task metadata seed
and `LoadEffectiveSessionRuntimeConfig` merge helper. The merge order is agent
profile snapshot, provider `runtime_config`, persisted `session_mode`, then
explicit `runtime_config_overrides`; every returned option map is cloned.
Effective dynamic options remove duplicated reserved `model` and `mode` entries
when provider metadata stores those values in both locations.
`PrepareSession` consumes the seed only for the first task session, writes it as
session runtime overrides, and removes the launch-only key from every session
metadata copy. Later sessions do not receive the seed.

The concrete SQLite repository now claims and removes the seed in the same
transaction as session creation. This closes concurrent first-session and
delete-and-replace races that could otherwise reuse a stale task seed. The
regression test covers both races.

Files changed:

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/models_test.go`
- `apps/backend/internal/task/models/session_runtime_config_test.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_test.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/service/service_turns_test.go`

Verification:

```text
rtk go test ./internal/task/models             PASS (71 tests)
rtk go test ./internal/orchestrator/executor  PASS (377 tests)
rtk go test ./internal/task/service -run 'Test(BuildTurnRuntimeConfigSnapshotFallsBackToSelectorModel|BuildTurnRuntimeConfigSnapshotUsesSessionModeOverRuntimeMode|StartTurnPersistsImmutableEffectiveRuntimeConfigSnapshot)'  PASS (3 tests)
```
