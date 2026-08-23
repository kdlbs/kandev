---
id: "03-idle-ttl-safety-net"
title: "Idle safety net and live-stack cap"
status: completed
wave: 1
depends_on: ["01-agent-stack-reaping-flag"]
plan: "plan.md"
spec: "../../../specs/platform/agent-stack-reaping.md"
---

# Task 03: Idle safety net and live-stack cap

## Acceptance

- The idle-session reaper tick additionally stops stacks whose *session* has
  been settled >= 10 min (`agentStackIdleTTL`) and whose session is in
  {WAITING_FOR_INPUT, IDLE, COMPLETED} with no active turn.
- Idle age comes from `sessionIdleSince` (session row `updated_at`, then
  `started_at`, then the executor row), never from `executors_running.updated_at`
  alone: that column is refreshed by execution persistence and status writes,
  so a long-lived stack that just finished a turn would be reaped immediately.
- A second pass caps concurrently live stacks at `agentStackLiveCap`. Counting
  uses every non-stopped `executors_running` row, so working sessions count
  toward the ceiling; eviction is oldest-idle first and only touches sessions
  that pass the reapable guards. The per-session read happens only once the
  count is known to exceed the ceiling.
- Both stops reuse the shared guarded primitive with reasons
  `agent stack reaping: idle ttl` and `agent stack reaping: live stack cap`;
  row repair stays with the existing reclaim phase on later ticks.
- The reaper's documented "never call StopAgent" invariant is relaxed to the
  guarded contract instead of silently violated.

## Verification

- `cd apps/backend && go test ./internal/orchestrator -run 'TestAgentStackReaping_IdleTTL|TestAgentStackReaping_LiveStackCap|TestIdleReaper'`

## Files

- `apps/backend/internal/orchestrator/idle_session_reaper.go`
- `apps/backend/internal/orchestrator/agent_stack_reaper.go`
