---
id: "01-runtime-terminal-environment"
title: "Runtime terminal environment propagation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Runtime Terminal Environment Propagation

## Acceptance

- The lifecycle retains a defensive, runtime-only copy of each execution's resolved environment
  and makes it available only to authorised task execution surfaces.
- Local/worktree user-shell terminals and passthrough-agent PTYs receive the same managed Git
  broker, indexed configuration, and shimmed `PATH` as their execution's agent subprocess.
- Executor-inheritance mode and unauthorised terminal access do not gain managed broker values.

## Verification

```bash
cd apps/backend && rtk go test ./internal/agent/runtime/lifecycle ./internal/gateway/websocket -run 'Test(ExecutionEnvironment|StartUserShellProcess|Passthrough).*ManagedGit' -count=1
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`
- `apps/backend/internal/gateway/websocket/terminal_handler.go`
- `apps/backend/internal/gateway/websocket/terminal_handler_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task establishes the runtime-only environment ownership used by later process
launch paths.

## Inputs

- Spec: `docs/specs/integrations/requirements/github-authentication.md` — What, Failure Modes, Scenarios.
- Plan: Runtime environment section.
- Existing patterns: `buildEnvForExecution`, `ExecutorProfileEnvForSession`, and
  `StartUserShell`.

## Output contract

Report the environment ownership boundary, files changed, focused test output, and any finding
that requires plan/spec revision. Focused lifecycle and gateway tests pass, including the
race-enabled regression set; agentctl process tests are covered by Task 02.
