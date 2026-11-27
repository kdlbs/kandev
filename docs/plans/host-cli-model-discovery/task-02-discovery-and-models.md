---
id: "02-discovery-and-models"
title: "CLI version discovery and merged model API"
status: complete
wave: 2
depends_on:
  - "01-hostcli-package"
plan: "plan.md"
requirements:
  - REQ-AGENTS-HOST-CLI-001
  - REQ-AGENTS-HOST-CLI-002
  - REQ-AGENTS-HOST-CLI-004
acceptance_criteria:
  - AC-AGENTS-HOST-CLI-001.1
  - AC-AGENTS-HOST-CLI-001.3
  - AC-AGENTS-HOST-CLI-002.1
  - AC-AGENTS-HOST-CLI-002.2
  - AC-AGENTS-HOST-CLI-002.4
  - AC-AGENTS-HOST-CLI-002.5
  - AC-AGENTS-HOST-CLI-002.6
  - AC-AGENTS-HOST-CLI-004.3
system_design:
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 02: CLI version discovery and merged model API

## Summary

Carry the host CLI version through discovery to the discovery endpoint, and
merge CLI-discovered models with the bridge list in the model endpoints with a
`discovery` metadata block, a process-local cache, startup warmup, and
on-demand refresh.

## In scope

- `discovery.Availability` CLI fields and the version cache in the registry.
- `AgentDiscoveryDTO` CLI fields.
- `ModelDiscoveryDTO`, `discovery` on `DynamicModelsResponse` and
  `ModelConfigDTO`, `source` on model entries.
- Controller cache, warmup, refresh, and merge helpers.
- `backendapp` warmup call after the host utility manager starts.

## Out of scope

- Update endpoints and jobs; frontend.

## Acceptance

- `GET /agents/discovery` returns `cli_version` for a detected host-CLI agent
  and omits the CLI fields for other agents.
- `GET /agent-models/:agentName` returns CLI models first with `source: "cli"`,
  bridge-only models with `source: "acp"`, and a `discovery` block; a CLI
  failure keeps the bridge list and reports the failure; `refresh=true`
  re-runs discovery and a second call without refresh serves the cache.
- Non-host agents return responses byte-identical to today apart from no new
  fields.

## Verification

```bash
(cd apps/backend && go test -run 'HostCLI|Discovery|DynamicModels|AvailableAgent' ./internal/agent/discovery/... ./internal/agent/settings/controller/... ./internal/agent/settings/handlers/...)
(cd apps/backend && go build ./... && gofmt -l internal/agent internal/backendapp)
```

## Files likely touched

- `apps/backend/internal/agent/discovery/discovery.go`
- `apps/backend/internal/agent/discovery/discovery_test.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_discovery.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/settings/controller/host_cli_models.go`
- `apps/backend/internal/agent/settings/controller/host_cli_models_test.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

Task 01.

## Risks

- Version detection runs inside the bounded discovery sweep; a hung CLI must
  not delay the sweep beyond its own five-second timeout.

## Parallelism

`sequential`

## Inputs

- System design: Version detection, Discovery projection, Model discovery,
  Merged model response.
- Existing `FetchDynamicModels`, `buildModelConfigFromHostUtility`,
  `runtimeUpdateStatusEntry`.

## Results

Implemented as scoped: `discovery.Availability.CLIVersion`/`CLIVersionError`
and the per-path version cache in `discovery/host_cli.go`;
`AgentDiscoveryDTO.cli_version`/`cli_version_error`; `ModelDiscoveryDTO` and
`discovery` on `DynamicModelsResponse`/`ModelConfigDTO`; the controller's
host-CLI model cache, `WarmHostCLIModels`, and `refreshHostCLIModels` in
`controller/host_cli_models.go`; the `backendapp` warmup goroutine after
`SetHostUtility`. Covered by `discovery/host_cli_test.go` and
`controller/host_cli_models_test.go`.
