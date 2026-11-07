---
id: "01-agent-origin-ceiling-admission"
title: "Admit agent-requested starts as automatic"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.13
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Admit agent-requested starts as automatic

## Summary

Mark the launch requests of an MCP-created task's auto-start as agent-requested
and admit them through the session ceiling as automatic, including replay.

## In scope

- `LaunchSessionRequest.Automatic` (server-only), honored by
  `launchOriginForActivation` and `agentLaunchOrigin`.
- `launchStart`, `launchStartCreated`, and `launchInitialCreatePrompt` pass the
  automatic origin; `startCreatedSessionOptions.origin` drives seam 2 admission
  and manual creation authority, and the `start_created` payload records it.
- `launchAutoStartTask` marks its prepare, start, and start-created requests.

## Out of scope

`spawn_session`, message follow-up resumes, recovery continuations, and the
user manual override.

## Verification

- `go test ./internal/orchestrator/ -run 'TestLaunchSession_AgentOrigin|TestLaunchSession_UserStartOverCeiling|TestLaunchOriginForActivation_AgentOrigin'`
- `go test ./internal/mcp/handlers/ -run TestLaunchAutoStartTask`
- Existing ceiling, session launch, create-prompt, and deferred launch tests.
