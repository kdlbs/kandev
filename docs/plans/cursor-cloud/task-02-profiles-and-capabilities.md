---
id: "02-profiles-and-capabilities"
title: "Add gated cloud profiles and execution capabilities"
status: complete
wave: 2
depends_on:
  - "01-cursor-api-client"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-004
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-001.1
  - AC-EXECUTORS-CURSOR-CLOUD-001.2
  - AC-EXECUTORS-CURSOR-CLOUD-001.3
  - AC-EXECUTORS-CURSOR-CLOUD-001.4
  - AC-EXECUTORS-CURSOR-CLOUD-004.3
  - AC-EXECUTORS-CURSOR-CLOUD-001.5
  - AC-EXECUTORS-CURSOR-CLOUD-004.5
  - AC-EXECUTORS-CURSOR-CLOUD-001.6
  - AC-EXECUTORS-CURSOR-CLOUD-001.7
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 02: Add gated cloud profiles and execution capabilities

## Summary

Enabled backend configuration validates secret scope, model, callback, and profile compatibility without exposing secret values.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Add the cloud executor and managed agent identities without local binary discovery. Enforce compatible pairs and reject unsupported runtime modes.
- Add global secret-reference and HTTPS callback configuration, server-side connection/catalog handlers, and model validation through the client.
- Add the typed capability projection and backend rejection of unavailable workspace actions. Preserve legacy defaults and existing remote executors.
- Add features.cursorCloud / KANDEV_FEATURES_CURSOR_CLOUD through all typed registry, config, profile, and frontend-default layers; all shipped profiles remain off.
- Gate new entry points and retain the documented observation/cancel drain exception for existing bindings. Prevent standalone fallback for the new type.

- Apply the mandatory [runtime-feature-flags checklist](../../../.agents/skills/runtime-feature-flags/SKILL.md). Name every backend gate and retain all-off shipped feature defaults.
- Implement the e2e-only mock-origin and loopback HTTP callback exceptions from the design. Add mock selector defaults through profiles.yaml without enabling the feature.
- Add TestCursorCloudProdIgnoresMockOrigin, TestCursorCloudDevIgnoresMockOrigin, and TestCursorCloudE2ELoopbackCallback to backend composition tests.
- Disclose shared Cursor billing; authorize profile use and task/workspace access before dispatch. Reject mid-session model/profile/permission/plan-mode changes.
- Explicit completeness coverage: runtimeflags registry/config tests, profiles/profiles_test.go, and the frontend features-contract.test.ts. Mock environment selectors are separate from typed feature identities.

- Gate Agents-page type discovery on an accessible, saved cloud executor with valid required configuration. Keep executor setup independent of agent discovery.
- Cover absent, incomplete, newly saved, last removed, inaccessible, and temporarily disconnected executor states. Preserve saved agent profiles and history.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- Enabled backend configuration validates secret scope, model, callback, and profile compatibility without exposing secret values.
- Disabled HTTP, WS, MCP configuration, discovery, startup, and dispatch paths produce no new remote work or grants.
- Legacy executor tests retain their behavior; cloud workspace actions fail even when called directly.

## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/agent/executor ./internal/agent/agents ./internal/agent/registry ./internal/task/handlers ./internal/backendapp -count=1)
(cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck)
make -C apps/backend lint
```

### Evidence mapping

- 001.1, 001.2, 001.4: `internal/backendapp/cursor_cloud_admission_test.go: TestCursorCloudAdmission`.
- 001.3: `internal/backendapp/cursor_cloud_gate_test.go: TestCursorCloudDisabledEntryPoints`.
- 004.3: `internal/backendapp/cursor_cloud_capabilities_test.go: TestCursorCloudWorkspaceDenied`.

- 001.6-001.7: `internal/backendapp/cursor_cloud_admission_test.go: TestCursorCloudAgentDiscovery` and desktop/phone `cursor-cloud.spec.ts` discovery scenarios.

## Files likely touched

- `profiles.yaml`.
- `apps/backend/internal/common/config/`.
- `apps/backend/internal/runtimeflags/`.
- `apps/backend/internal/agentruntime/`.
- `apps/backend/internal/agent/executor/`.
- `apps/backend/internal/agent/agents/`.
- `apps/backend/internal/agent/registry/`.
- `apps/backend/internal/task/models/`.
- `apps/backend/internal/task/handlers/executor_profile_handlers.go`.
- `apps/backend/internal/backendapp/task_agent_executor_compatibility.go`.
- `apps/backend/internal/backendapp/agents.go`.
- `apps/web/lib/state/slices/features/types.ts`.
- `apps/web/lib/types/http.ts`.

- `apps/backend/internal/runtimeflags/registry.go`.
- `apps/backend/internal/runtimeflags/config.go`.
- `apps/backend/internal/runtimeflags/registry_test.go`.
- `apps/backend/internal/runtimeflags/config_test.go`.
- `apps/backend/internal/profiles/profiles_test.go`.
- `apps/web/lib/state/slices/features/features-contract.test.ts`.

## Dependencies

01-cursor-api-client

## Risks

Existing remote-executor checks assume CLI credentials and workspace access. Adding only an enum would incorrectly enable those paths.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Completed. `go test ./internal/agent/executor ./internal/agent/agents ./internal/agent/registry ./internal/task/handlers ./internal/backendapp -count=1`, runtimeflags/config/profiles tests, Cursor Cloud service/model/DTO/handler tests, the feature contract test, and web typecheck passed. `make -C apps/backend lint` passed with zero issues. Existing remote executor tests remain green.
