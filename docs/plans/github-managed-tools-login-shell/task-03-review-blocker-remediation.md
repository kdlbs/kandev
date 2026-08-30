---
id: "03-review-blocker-remediation"
title: "Review blocker remediation"
status: done
wave: 3
depends_on: ["01-path-independent-git-helper", "02-login-shell-shim-restoration"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 03: Review Blocker Remediation

## Acceptance

- Docker and Sprites preparation publish an absolute Kandev-owned credential-helper executable
  before a prepare-script clone can invoke the managed indexed Git configuration.
- A running `agentctl` publishes its own absolute executable for agent, terminal, and task-command
  child processes; Git helper execution never depends on ambient `PATH` or the later-created CLI
  shim directory.
- Executor inheritance removes the current executable-variable helper plus both prior managed
  helper forms without disturbing unrelated indexed Git configuration.
- An inherited `BASH_ENV` path containing `$VAR` or `${VAR}` references is resolved from the
  effective child environment before the generated startup fragment sources it.

## Verification

RED first:

```bash
cd apps/backend && rtk go test ./internal/agent/runtime/lifecycle ./internal/agentctl/server/config -run 'Test.*(ManagedGitCredentialHelperBeforeAgentctl|ParameterizedBashEnv)' -count=1
```

GREEN and focused regression checks:

```bash
cd apps/backend && rtk go test ./cmd/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/githubauth -run 'Test.*(GitHub|ManagedGit|BashEnv|ShellExec|ContainerConfig|SpriteEnv)' -count=1
```

## Files likely touched

- `apps/backend/internal/githubauth/environment.go`
- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_config_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_helpers.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_clone_auth_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`
- this task file, `plan.md`, and the linked spec

## Dependencies

Tasks 01 and 02 establish the helper and login-shell contracts being corrected.

## Parallelism

Sequential. Both regressions alter the shared managed GitHub environment contract and its tests.

## Output contract

Report both RED failure reasons, the final helper executable lifetime, inherited-hook expansion
behavior, exact targeted test results, secret/runtime-only boundaries, and synchronized task/plan
status.

## Results

- RED: the lifecycle/config command failed in three intended tests because Docker and Sprites did
  not publish a pre-start helper executable and the parent `BASH_ENV` remained literal. The real
  Git regression failed with `/agentctl: not found`; the agentctl startup regression observed an
  empty helper-executable variable.
- GREEN: `rtk go test ./cmd/agentctl ./internal/agentctl/server/config
  ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle
  ./internal/orchestrator/executor ./internal/githubauth -run
  'Test.*(GitHub|ManagedGit|BashEnv|ShellExec|ContainerConfig|SpriteEnv)' -count=1` passed 73 tests.
- Full affected packages: the same six packages without `-run` passed 2,000 tests.
- Docker and Sprites bind `/usr/local/bin/agentctl` only for broker-enabled preparation. Agentctl
  replaces that boundary value with its absolute `os.Executable` path for children. The helper
  remains fail-closed, and no lease, token, scope, or credential value is written to a file or
  process argument.
- Parent Bash hooks now resolve simple `$VAR` and `${VAR}` path references from the effective child
  environment without `eval`; unsupported shell expressions remain literal rather than being
  executed by Kandev's composition logic.
