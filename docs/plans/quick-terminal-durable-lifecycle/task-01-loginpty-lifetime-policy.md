---
id: "01-loginpty-lifetime-policy"
title: "Per-session lifetime policy and StopAll in loginpty"
status: draft
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 01: Per-session lifetime policy and StopAll in loginpty

## Acceptance

- Host-shell sessions (`agentID == loginpty.HostShellAgentID`) run with no idle timeout and no hard
  (wall-clock) timeout. They end only on natural process exit or an explicit `stop()`/`Stop`/
  `StopSession`.
- Agent-login sessions (any other `agentID`) retain the existing `IdleTimeout` (10m) and
  `HardTimeout` (30m) behavior unchanged.
- The lifetime policy is set once at session creation (in `StartWithKey`, derived from `agentID`),
  not by scattered string checks inside `supervise`.
- Under the no-timeout policy, `supervise` still exits and cleans up correctly on natural process
  exit and on external `stop()`: the session is removed from `m.sessions` and `m.byID`, and `onExit`
  fires with the correct exit code.
- `Manager.StopAll()` stops every currently live session, is idempotent, and returns after all are
  stopped.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/loginpty/... -count=1)
(cd apps/backend && golangci-lint run ./internal/agent/loginpty/... --timeout=5m)
```

## Files likely touched

- `apps/backend/internal/agent/loginpty/manager.go`
- `apps/backend/internal/agent/loginpty/manager_test.go`

## Dependencies

None.

## Parallelism

Parallel-safe with Task 02 (frontend). Task 03 depends on this task's policy and `StopAll` surface.

## Inputs

- Spec sections: State machine (no idle/hard timeout row), Failure modes, Persistence guarantees.
- Existing `supervise()` loop: `exited`, `idle.C`, `ctx.Done()`, `activityCh` cases; `Session.stop()`
  idempotency; `waitForCommand()`.
- `HostShellAgentID` constant in `apps/backend/internal/agent/loginpty/handlers.go`.

## Output contract

Report the policy representation chosen, the `supervise` changes, the `StopAll` implementation, and
exact test results. Do not change agent-login timeout constants. Update this task and the parent plan
only after the focused tests and lint pass.

## Results

_To be filled in during implementation._
