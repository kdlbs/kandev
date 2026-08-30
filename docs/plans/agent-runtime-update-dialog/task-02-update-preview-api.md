---
id: "02-update-preview-api"
title: "Add the read-only update preview API"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 02: Add the read-only update preview API

## Acceptance

- A managed installed agent exposes current version, upstream target version,
  trusted update argv, and a display command through a read-only endpoint.
- The display command quotes the update recipe's empty final argv entry as
  `""`, preserving the exact direct-execution command.
- Preview never creates a job, claims maintenance state, or executes the
  update command.
- Unmanaged, missing, unavailable-updater, and registry-resolution failures
  return explicit errors and do not mutate runtime state.

## Verification

- RED/GREEN controller:
  `cd apps/backend && go test -run 'TestAgentUpdatePreview' ./internal/agent/settings/controller`
- RED/GREEN handler:
  `cd apps/backend && go test -run 'TestAgentUpdatePreviewEndpoint' ./internal/agent/settings/handlers`

## Files likely touched

- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/controller.go`
- `apps/backend/internal/agent/settings/controller/controller_test.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_test.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`

## Dependencies

None.

## Parallelism

Sequential by default. It is file-disjoint from Task 01 and may run in
parallel only with explicit user authorization.

## Inputs

- Spec update-preview API and approval scenarios
- Existing `RuntimeUpdater`, `ManagedNPMRuntimeSpec.CacheUpdateCommand`, and
  `buildCommandString`
- Existing update handler error mapping and direct-argv security boundary

## Output contract

Report RED and GREEN results, response/error contracts, changed files, command
trust-boundary risk, and update this task plus `plan.md` status.
