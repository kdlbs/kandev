---
id: "02-agentctl-shell-process-environment"
title: "Agentctl shell and process environment"
status: done
wave: 2
depends_on: ["01-runtime-terminal-environment"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 02: Agentctl Shell and Process Environment

## Acceptance

- The legacy agentctl shell, per-terminal agentctl shells, and task-scoped command processes
  inherit the instance `AgentEnv` before explicit call-site environment overrides.
- Managed Git configuration and shim-first `PATH` remain intact as one effective environment;
  explicit per-call values retain their established precedence.
- Process startup reports an error before execution if managed environment preparation fails and
  never falls back to an ambient Git helper or `gh` login.

## Verification

```bash
cd apps/backend && rtk go test ./internal/agentctl/server/process -run 'Test(StartShell|StartTerminalShell|StartProcess).*AgentEnvironment' -count=1
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/runner.go`
- `apps/backend/internal/agentctl/server/process/interactive_shells.go`
- `apps/backend/internal/agentctl/server/process/runner_shells_test.go`
- `apps/backend/internal/agentctl/server/process/runner_test.go`

## Dependencies

Task 01 establishes the canonical runtime environment boundary and its credential-safety tests.

## Parallelism

Sequential.

## Inputs

- Spec: `docs/specs/integrations/requirements/github-authentication.md` — What, Failure Modes, Scenarios.
- Plan: Agentctl shell and command launch section.
- Existing patterns: `mergeAgentEnvIntoShellConfig`, `buildShellEnv`, `mergeEnvWithStrip`, and
  `gitEnvironment`.

## Output contract

Report precedence behavior, files changed, focused test output, and any need to revise the
security contract. Explicit request values retain precedence over inherited instance values;
focused and package-level process tests pass.
