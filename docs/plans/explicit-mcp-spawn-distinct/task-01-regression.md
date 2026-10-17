---
id: "01-regression"
title: "Real spawn integration regression"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001
acceptance_criteria:
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.2
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.3
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.4
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.5
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.7
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.8
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.9
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.10
system_design:
  - ../../specs/agents/system-design/spawn-session-effective-profile.md
---

# Task 01: Real spawn integration regression

MAIN owns apps/backend/internal/mcp/handlers/spawn_session_integration_test.go. Sequential.
Use existing SQLite/service/runtime fixtures, valid workflow source binding and committed reuse route. Call the real MCP handler and orchestrator; verify returned, persisted and launched session identity and profile. Include same-profile, inherited-profile, empty-profile and normal-workflow controls. Preserve old session name and recorded workflow route. No new production test helpers.

Verification: `cd apps/backend && go test ./internal/mcp/handlers -run 'TestHandleSpawnSession' -count=1`.

## Results

Red on unpatched e8c1761c: `go test ./internal/mcp/handlers -run 'TestHandleSpawnSession_(DistinctUnderWorkflowReuse|OrdinaryWorkflowReuseControl)$' -count=1 -timeout=30s` fails with `Should not be: "sess-1"`; normal workflow control passes. Fixture setup errors were corrected before accepting this behavioral red. Final `cd apps/backend && go test ./internal/mcp/handlers -run 'TestHandleSpawnSession' -count=1 -timeout=120s` passes (2.931s), including explicit different/same profile, caller/primary inheritance, empty-profile error/defaults, ordinary reuse and Office scheduler guard. Per-test SQLite fixtures cleaned by existing t.Cleanup; no external runtime.
