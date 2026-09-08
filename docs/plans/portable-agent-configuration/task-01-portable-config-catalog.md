---
id: "01-portable-config-catalog"
title: "Define portable configuration catalog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/portable-agent-configuration.md"
---

# Task 01: Define portable configuration catalog

## Acceptance

- Agent integrations declare stable, allowlisted configuration bundles that remain independent from `RemoteAuth`.
- The catalog API returns metadata and local availability, but it never returns file data.
- Claude, Codex, and OpenCode expose the required bundles, and Codex authentication contains `auth.json` only.

## TDD scenarios

1. RED: Add catalog tests for stable IDs, operating-system paths, availability, and empty declarations.
2. RED: Add handler tests for the response contract and the no-file-data rule.
3. RED: Update provider tests to require the initial bundle declarations.
4. GREEN: Add the optional agent capability, catalog, provider declarations, and handler route.
5. REFACTOR: Keep provider declarations separate from host discovery and API types.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/agent/agents ./internal/agent/remoteconfig ./internal/task/handlers`
- `cd apps/backend && go test -tags fts5 -run 'Test.*PortableConfig|TestHTTPListAgentConfigBundles' ./internal/agent/agents ./internal/agent/remoteconfig ./internal/task/handlers`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/agents/claude_acp_test.go`
- `apps/backend/internal/agent/agents/codex_acp.go`
- `apps/backend/internal/agent/agents/codex_acp_test.go`
- `apps/backend/internal/agent/agents/opencode_acp.go`
- `apps/backend/internal/agent/agents/opencode_acp_test.go`
- `apps/backend/internal/agent/remoteconfig/catalog.go`
- `apps/backend/internal/agent/remoteconfig/catalog_test.go`
- `apps/backend/internal/task/handlers/executor_profile_handlers.go`
- `apps/backend/internal/task/handlers/executor_profile_handlers_test.go`
- `apps/backend/internal/task/handlers/route_registration_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task defines the contract that Tasks 02 and 03 consume.

## Inputs

- The feature specification.
- The portable-bundle ADR.
- Existing `RemoteAuth` declarations and catalog behavior.
- Provider documentation for Claude, Codex, and OpenCode configuration files.

## Output contract

Report the API shape, bundle IDs, provider paths, RED evidence, GREEN evidence, and test results.

## Risks

- A provider path can change after a provider release.
- A bundle declaration can accidentally include credentials or runtime state.

## Results

Implemented the optional portable-configuration capability, stable catalog IDs,
Claude/Codex/OpenCode declarations, metadata-only HTTP endpoint, and Codex
auth/config separation. Backend catalog and handler tests passed: 827 passed in
3 packages.
