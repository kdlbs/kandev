---
id: "03-idle-ttl-safety-net"
title: "Configurable idle safety net"
status: completed
wave: 1
depends_on: ["01-agent-stack-reaping-flag"]
plan: "plan.md"
spec: "../../../specs/platform/agent-stack-reaping.md"
---

# Task 03: Configurable idle safety net

## Acceptance

- The idle-session reaper tick additionally stops stacks whose *session* has
  been settled for `agentctl.idleTimeout` and whose session is in
  {WAITING_FOR_INPUT, IDLE, COMPLETED} with no active turn.
- Idle age comes from `sessionIdleSince` (session row `updated_at`, then
  `started_at`, then the executor row), never from `executors_running.updated_at`
  alone: that column is refreshed by execution persistence and status writes,
  so a long-lived stack that just finished a turn would be reaped immediately.
- The TTL pass lists every durable execution row independently of the stale-row
  query, so a recently refreshed executor row cannot hide an old idle session.
- Stops reuse the shared guarded primitive with reason
  `agent stack reaping: idle ttl`; row repair stays with the existing reclaim
  phase on later ticks.
- The reaper's documented "never call StopAgent" invariant is relaxed to the
  guarded contract instead of silently violated.

## Verification

- `cd apps/backend && go test ./internal/orchestrator -run 'TestAgentStackReaping_IdleTTL|TestIdleReaper'`

## Files

- `apps/backend/internal/orchestrator/idle_session_reaper.go`
- `apps/backend/internal/orchestrator/agent_stack_reaper.go`
