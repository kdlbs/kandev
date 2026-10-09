---
id: "03-profile-admission"
title: "Expose provider profiles and enforce remote admission"
status: done
wave: 3
depends_on:
  - "01-provider-contract"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-002
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.1
  - AC-EXECUTORS-PLUGIN-001.2
  - AC-EXECUTORS-PLUGIN-001.3
  - AC-EXECUTORS-PLUGIN-002.2
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 03: Expose provider profiles and enforce remote admission

## Summary

Expose a backend-owned provider catalog through existing executor routes and profile APIs. Route plugin_remote through its own backend and reject incompatible launches before allocation.

## In scope

- Create immutable provider-owned executor entries with stable IDs and schema-validated scalar profiles. Add redacted secret field semantics using existing vault storage.
- Extend DTO, boot payload and WebSocket projections with availability, schema and capability descriptors. Do not expose provider state or secrets.
- Audit executor/runtime mappings and all admission paths: task create, session start, workflow, MCP, internal callers and runner switching. Reject sharing, local-only repositories, missing reverse connectivity and unavailable owners.

## Out of scope

- Profile rendering, remote compute allocation, custom provider form scripts.

## Acceptance

- Authorized profile create/update/read/reload works with secret unchanged/replace/clear semantics and rejects cross-provider secret references.
- HTTP, WebSocket, MCP and internal admission converge on one policy; no rejected request allocates resources or falls back to local.
- Built-in alias and capability regressions pass; provider catalog availability does not rewrite saved selections.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/backend && go test ./internal/plugins/... ./internal/task/handlers ./internal/task/service ./internal/agent/executor ./internal/agentruntime ./pkg/api/v1 -count=1)
(cd apps/web && pnpm run typecheck)
```

Required evidence:

- `internal/plugins/executor_profiles_test.go: TestPluginExecutorProfileSecrets, TestPluginExecutorCatalog`
- `internal/task/service/service_plugin_executor_test.go (new): TestPluginExecutorAdmissionPaths, TestPluginExecutorNoLocalFallback`

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/agent/executor/executor.go`
- `apps/backend/internal/agentruntime/runtime.go`
- `apps/backend/internal/task/service/service_resources.go and service_runner_switch.go`
- `apps/backend/internal/task/handlers/executor_handlers.go and executor_profile_handlers.go`
- `apps/backend/pkg/api/v1/`
- `apps/backend/internal/plugins/ (new executor_profiles.go and executor_profiles_test.go)`
- `apps/web/lib/types/executor.ts and http.ts`

## Dependencies

[Task 01](task-01-provider-contract.md).

## Risks

Two current type-to-runtime mappings default to standalone. Preserve known legacy aliases but fail closed for plugin providers.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Completed. Added provider-owned executor catalog and profile CRUD with schema validation, vault-backed secret replace/clear/unchanged behavior, redacted DTO/event projections, and fail-closed `plugin_remote` executor mapping. Wired provider admission into task creation and runner switching; exposed provider descriptors through HTTP/WS executor/profile surfaces.

Verification passed:

- `(cd apps/backend && go test ./internal/plugins/... ./internal/task/handlers ./internal/task/service ./internal/agent/executor ./internal/agentruntime ./pkg/api/v1 -count=1)`
- `(cd apps/web && pnpm run typecheck)`

Review remediation: profile rotation retains replaced vault items so inventory-held references continue to
rehydrate during recovery. Clearing a credential or deleting a profile is rejected while a retained plugin
environment references that profile. Once inventory settles absent, profile deletion removes all current
and historical secret items. `TestPluginExecutorProfileSecrets` exercises rotation, recovery from the
recorded old reference, clear/delete rejection, and cleanup after confirmed absence.
