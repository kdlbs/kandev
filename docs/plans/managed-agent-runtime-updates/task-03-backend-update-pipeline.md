---
id: "03-backend-update-pipeline"
title: "Build the backend update pipeline"
status: done
wave: 2
depends_on: ["01-unpinned-managed-runtimes"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
role: implementer
model_tier: default
---

# Task 03: Build the backend update pipeline

## Acceptance

- Managed installed-agent metadata and the three interlocked update endpoints
  expose current/target versions, bounded progress, retained jobs, typed
  failures, and same-agent install/update mutual exclusion.
- Update jobs resolve npm metadata and execute only built-in direct argv,
  deduplicate while active, refresh ACP capabilities after package success, and
  preserve prior capability data on failure.
- Successful capability refresh broadcasts the updated catalogue; auth-required
  refresh is reported separately from package success.

## Verification

- `cd apps/backend && go test ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers`
- `cd apps/backend && go test ./pkg/websocket`

## Files likely touched

- `apps/backend/internal/agent/hostutility/public.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/manager_test.go`
- `apps/backend/internal/agent/settings/controller/controller.go`
- `apps/backend/internal/agent/settings/controller/agent_discovery.go`
- `apps/backend/internal/agent/settings/controller/agent_discovery_test.go`
- `apps/backend/internal/agent/settings/controller/agent_install.go`
- `apps/backend/internal/agent/settings/controller/agent_install_test.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/controller/agent_update_test.go`
- `apps/backend/internal/agent/settings/controller/maintenance_jobs.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

Task 01 supplies `ManagedNPMRuntimeAgent`, its package metadata, and the normal
and update command builders.

## Inputs

- Spec `API surface`, `State machine`, `Failure modes`, and scenarios
- Plan `Runtime update and capability refresh` and
  `HTTP, DTO, and WebSocket contracts`
- Existing install job store, host-utility `Refresh`, Settings interlock, and
  `BroadcastAvailableAgents` patterns

## Output contract

Report intent/acceptance, base/head SHA, changed entry points, named spec/ADR
sections, risk tags (`process-exec`, `job-concurrency`, `websocket`,
`capability-cache`), exact targeted results, and uncertainties. Update only
this task file to `done`; do not edit `plan.md`.
