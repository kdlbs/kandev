---
id: "02-launch-intent"
title: "Protect explicit spawn launch intent"
status: done
wave: 2
depends_on: ["01-regression"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001
acceptance_criteria:
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.2
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.3
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.4
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.5
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.9
  - AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.10
system_design:
  - ../../specs/agents/system-design/spawn-session-effective-profile.md
---

# Task 02: Protect explicit spawn launch intent

MAIN owns spawn_session.go and ProfileExplicit comments in orchestrator/session_launch.go and task_operations.go. Sequential.
Mark every explicit MCP spawn, including empty-profile requests, with the existing routing bypass. Preserve normal defaults/errors, ordinary workflow reuse, profile validation and Office ownership guards. No global routing rewrite.

Verification: `cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/orchestrator -count=1` and `make -C apps/backend lint`.

## Results

`cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/orchestrator -count=1` passes (98.343s / 7.759s / 153.084s). Focused explicit-profile and workflow-route checks pass (0.488s). `make -C apps/backend lint` reports zero issues. Production behavior change is one launch flag; no live deployment effects.
