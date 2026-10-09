---
id: "03-refresh-triggers"
title: "Discovery refresh triggers"
status: complete
wave: 3
depends_on:
  - "02-discovery-and-models"
plan: "plan.md"
requirements:
  - REQ-AGENTS-HOST-CLI-003
acceptance_criteria:
  - AC-AGENTS-HOST-CLI-003.1
  - AC-AGENTS-HOST-CLI-003.2
  - AC-AGENTS-HOST-CLI-003.3
  - AC-AGENTS-HOST-CLI-003.4
  - AC-AGENTS-HOST-CLI-003.5
system_design:
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 03: Discovery refresh triggers

## Summary

Wire the host-CLI model cache into the points where Kandev already learns a
CLI may have changed: backend startup, an Agents settings page load or
Rescan, a successful agent install, and an explicit profile-editor refresh.
No new install or update mechanism is added; this task only decides when the
existing cache in `controller/host_cli_models.go` (built in Task 02) is
invalidated and re-warmed.

## In scope

- `backendapp/main.go`: warm every stale or missing catalogue in a goroutine
  after `SetHostUtility`, so the first settings page load can serve a
  discovered list without blocking startup.
- `GET /api/v1/agents/discovery` (`httpDiscoverAgents`): after responding,
  warm stale catalogues in a bounded background goroutine
  (`warmHostCLIModelsAsync`, `hostCLIWarmupTimeout`).
- `JobStore`'s `onSuccess` callback (`SetJobBroadcaster`): call
  `hostCLIInstallSucceeded` so a successful agent install drops and
  rediscovers that agent's catalogue.
- `GET /api/v1/agent-models/:agentName?refresh=true`
  (`FetchDynamicModels(refresh=true)`): re-run host-CLI discovery for that
  agent synchronously with the existing capability-probe refresh.

## Out of scope

- Any new CLI install or update API, job, HTTP route, or WS action.
- Frontend.

## Acceptance

- Backend startup schedules a warmup goroutine that does not block
  `startGatewayAndServe`.
- A discovery request returns immediately and still refreshes a stale
  catalogue afterward, bounded by `hostCLIWarmupTimeout`.
- A successful install job's `onSuccess` path invalidates and rediscovers
  only that agent's model catalogue.
- `refresh=true` on the model endpoint re-reads the CLI synchronously; a
  request without it serves the cache.

## Verification

```bash
(cd apps/backend && go test -run 'HostCLI|WarmHostCLI' ./internal/agent/settings/controller/... ./internal/agent/settings/handlers/...)
(cd apps/backend && go build ./... && gofmt -l internal/agent internal/backendapp)
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/controller/controller.go`
- `apps/backend/internal/agent/settings/controller/host_cli_models.go`
- `apps/backend/internal/agent/settings/controller/host_cli_models_test.go`

## Dependencies

Task 02.

## Parallelism

`sequential`

## Inputs

- System design: Refresh triggers.
- Existing `SetJobBroadcaster`, `httpDiscoverAgents`,
  `FetchDynamicModels(refresh=true)`.

## Results

Implemented as scoped: `backendapp/main.go` starts
`agentSettingsController.WarmHostCLIModels` in a goroutine after
`SetHostUtility`; `Handlers.httpDiscoverAgents` calls
`warmHostCLIModelsAsync` (bounded by `hostCLIWarmupTimeout`, 30s) after
writing its response; `SetJobBroadcaster`'s `onSuccess` callback calls
`c.hostCLIInstallSucceeded(agentName)`; `FetchDynamicModels` re-reads the CLI
when called with `refresh=true`. No update mechanism, job, or route was
added. Covered by `controller/host_cli_models_test.go`
(`TestWarmHostCLIModelsOnlyVisitsModelSourceAgents`,
`TestHostCLIInstallSucceededRediscoversModels`,
`TestHostCLIModelsCacheAndRefresh`).
