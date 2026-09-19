---
id: "02-login-shell-shim-restoration"
title: "Login-shell shim restoration"
status: done
wave: 2
depends_on: ["01-path-independent-git-helper"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 02: Login-Shell Shim Restoration

## Acceptance

- A broker-enabled non-interactive Bash login shell preserves an existing `BASH_ENV` hook and
  restores the instance-owned `agentctl` and `gh` shims ahead of ambient tools after profile
  initialization resets `PATH`.
- Agentctl Unix `sh -lc` task processes restore the same managed path immediately before the
  requested command, including shells that do not honor `BASH_ENV`.
- Broker-disabled, executor-inheritance, explicit profile-token, and Windows launch paths receive
  no managed Bash hook or shell prelude behavior change.

## Verification

RED first:

```bash
cd apps/backend && rtk go test ./cmd/agentctl ./internal/agentctl/server/config -run 'Test.*GitHubCLIShim.*LoginShell' -count=1
```

GREEN and focused regression checks:

```bash
cd apps/backend && rtk go test ./cmd/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/process -run 'Test.*(GitHubCLIShim|CollectAgentEnv|ShellExec|ManagedGitHubPath|BashEnv)' -count=1
```

## Files likely touched

- `apps/backend/cmd/agentctl/github_cli_shim.go`
- `apps/backend/cmd/agentctl/github_cli_shim_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/process/shell_unix.go`
- `apps/backend/internal/agentctl/server/process/shell_test.go`
- this task file and `plan.md`

## Dependencies

Task 01 establishes the shared managed-helper and shim-environment contract.

## Parallelism

Sequential.

## Inputs

- Spec: managed runtime-tool behavior, runtime-only environment security, and login-PATH scenarios.
- Plan: Bash startup-fragment composition and agentctl `sh -lc` restoration design.
- Existing patterns: `installGitHubCLIShim`, `prepareGitHubCLIShim`,
  `CollectAgentEnvWithError`, `prependPathEntry`, and `shellExecArgs`.

## Output contract

Report the RED failure reason, startup-hook precedence and recursion handling, broker activation
conditions, Unix/Windows behavior, files changed, exact test results, cleanup evidence, remaining
shell portability limits, and synchronized task/plan status.

## Results

- RED: `cd apps/backend && rtk go test ./cmd/agentctl ./internal/agentctl/server/config -run 'Test.*GitHubCLIShim.*LoginShell' -count=1`
  failed as expected: the generated Bash environment was absent and the collected `BASH_ENV`
  remained the parent hook.
- GREEN: `cd apps/backend && rtk go test ./cmd/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/process -run 'Test.*(GitHubCLIShim|CollectAgentEnv|ShellExec|ManagedGitHubPath|BashEnv)' -count=1`
  passed (14 tests), including the actual generated startup script, existing-hook preservation,
  and the Unix shell wrapper.
- Additional focused checks passed: `go test ./cmd/agentctl -run 'TestInstallGitHubCLIShim.*' -count=1`
  (3 tests); the affected backend package set (`cmd/agentctl`, agentctl config/process,
  orchestrator executor, and `githubauth`) passed; lifecycle GitHub/environment checks passed (52
  tests); and `internal/gitconfigenv` passed (5 tests).
- All temporary shim directories and hook files are test-owned and cleaned up by `t.TempDir`.
  No broker values or credentials were persisted. Final `git diff --check` passed.
