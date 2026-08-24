---
id: "04-tests-and-verification"
title: "Regression tests and verification"
status: completed
wave: 2
depends_on: ["02-completed-transition-stop", "03-idle-ttl-safety-net"]
plan: "plan.md"
spec: "../../../specs/platform/agent-stack-reaping.md"
---

# Task 04: Regression tests and verification

## Acceptance

- Tests cover: REVIEW transition leaves the stack warm (both writers);
  COMPLETED transition stops idle stacks; `CompleteTask` reaches the IDLE
  session `StopByTaskID` misses; `StopTask` still stops agents when the REVIEW
  write fails; stopped-event preserves COMPLETED session state; working-session,
  active-turn, prompt-admission, flag-off, and missing-execution guards;
  admission marker balance; runtime stop failure retains the execution for a
  confirmed retry; idle TTL uses the session clock even when the executor row
  was recently refreshed; a deterministic prompt/reaper interleaving; sweeper
  joins its workers and cancels their context on stop; and turn re-entry after
  a stack stop.
- `make -C apps/backend test lint` passes; web feature-contract test,
  typecheck, and lint pass.

## Verification

- `KANDEV_BACKEND_PORT=19876 CGO_ENABLED=1 GOCACHE=/tmp/kandev-go-cache make -C apps/backend test lint`
- `cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts`
- `cd apps/web && pnpm run typecheck && pnpm run lint`

## Files

- `apps/backend/internal/orchestrator/agent_stack_reaper_test.go` (new)
- `apps/backend/internal/orchestrator/agent_stack_ttl_test.go` (new)
- `apps/backend/internal/orchestrator/agent_stack_prompt_race_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_accessors_test.go`
- `apps/backend/internal/orchestrator/prompt_idle_session_test.go`
