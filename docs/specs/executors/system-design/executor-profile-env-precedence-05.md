---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001
created: 2026-08-17
owners:
  - tbd
---
# Executor-Profile Environment Precedence System Design Part 5

## Purpose and boundaries

This design preserves the technical source detail for `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## References

- `apps/backend/internal/agent/runtime/environment/environment.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/orchestrator/executor/task_environment.go`
- `apps/backend/internal/orchestrator/executor/executor_state.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go` — the AGENT-profile save path,
  and the only home of all three save-time validators (`validateEnvVarValue:913`, reserved-key
  `:904`, duplicate-key `:908`). AC-38 no longer relies on any of them; AC-40 cites the reserved-key
  rule with its scope stated, and AC-6/AC-8d cite the duplicate-key rule the same way
- `apps/backend/internal/task/service/service_resources.go` — `CreateExecutorProfile` (`:1587`),
  `UpdateExecutorProfile` (`:1625`) and `validateGlobalProfileEnvRefs` (`:1665`): the
  EXECUTOR-profile save path, which enforces none of the three rules above
- `apps/web/components/settings/profile-edit/env-vars-card.tsx` — `rowsToEnvVars`, which filters
  empty keys only, so a blank-valued entry is savable from the UI as well
- `apps/backend/internal/workflow/signalmetrics/metrics_vars.go` — the expvar counter convention
  AC-24a follows (`expvar.NewMap` + `k1=v1;k2=v2` label keys)
- `docs/decisions/2026-08-03-scope-and-merge-repository-secrets.md`
- `docs/specs/workspaces/requirements/repository-secrets.md`
