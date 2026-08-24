---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-08-01
status: completed
---

# Implementation Plan: Managed GitHub Tools Across Login Shells

## Overview

The managed GitHub environment reaches the agent process correctly, but a later non-interactive
login shell can replace `PATH` and hide Kandev's temporary `agentctl` and `gh` shims. Repair the
exact Git failure first by making the credential helper resolve an absolute Kandev-owned
`agentctl` executable, then restore the CLI shim directory after Unix login-shell initialization so
broker-aware `gh` remains selected as well. Both changes stay conditional on a valid broker
contract and preserve executor-inheritance behavior.

## Confirmed Root Cause

`Executor.configureGitHubCredentialBrokerForRepositories` injects the managed helper and
`CollectAgentEnvWithError` prepends `KANDEV_GITHUB_CLI_SHIM_DIR` to the agent process `PATH`. The
running failing session contained that correct environment. Commands launched with `bash -lc` then
sourced `/etc/profile`, which replaced `PATH`; Git's `!agentctl git-credential` helper consequently
exited 127 before contacting the broker. The existing config test proves only the pre-shell
environment and therefore does not cover the failing boundary.

## Backend

### Make the Git credential helper path independent

Files:

- `apps/backend/internal/githubauth/environment.go`
- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_helpers.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`

Changes:

- Define the managed helper executable and helper command as shared GitHub runtime-environment
  constants. The helper uses a shell-quoted absolute executable variable instead of resolving
  `agentctl` through `PATH`.
- Bind the helper executable before Docker and Sprites preparation to the installed
  `/usr/local/bin/agentctl`, then let a running `agentctl` refresh it from `os.Executable` for child
  processes. The temporary shim directory remains the `gh`/shell-PATH mechanism, not the Git helper
  lifetime boundary.
- Keep the empty helper reset, managed helper, and `credential.useHttpPath=true` ordering unchanged.
- When executor inheritance removes managed credentials, recognize both the new helper and the
  legacy `!agentctl git-credential` value so stale runtime blocks cannot retain either helper.
- Keep the shim path runtime-only; do not persist it in the task-session credential snapshot or
  expose it in logs, errors, browser payloads, or process arguments.

### Restore managed tools after login-shell initialization

Files:

- `apps/backend/cmd/agentctl/github_cli_shim.go`
- `apps/backend/cmd/agentctl/github_cli_shim_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/process/shell_unix.go`
- `apps/backend/internal/agentctl/server/process/shell_test.go`

Changes:

- Install a mode-restricted Bash startup fragment beside the existing `agentctl` and `gh` shims.
  `prepareGitHubCLIShim` publishes its path through an internal runtime environment variable.
- When and only when an instance has a broker URL, compose `BASH_ENV` so the generated fragment
  runs after Bash login/profile initialization. Preserve an existing `BASH_ENV` through a separate
  internal variable, resolve simple `$VAR` and `${VAR}` hook-path references from the effective
  child environment, source it once, avoid recursive composition, and prepend the shim directory
  idempotently.
- Prefix agentctl's Unix `sh -lc` command body with a broker-conditional, shell-quoted PATH restore.
  This covers task-scoped process launches whose `/bin/sh` does not honor `BASH_ENV`; Windows keeps
  its existing non-login command path.
- Broker-disabled instances, executor-inheritance sessions, and explicit profile-token sessions
  retain their original `PATH` and shell-hook variables.

## Frontend

No frontend or browser-contract changes are required. This repair restores the already documented
managed task credential behavior.

## Tests

- **What:** the managed Git helper works when `PATH` deliberately excludes the helper executable and
  fails closed when the managed helper is absent.
  **File:** `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`.
  **How:** configure a broker request, publish a fake absolute `agentctl` path containing whitespace,
  and run a real `git credential fill` subprocess with an ambient-only `PATH`. The RED test must
  fail with a PATH- or shim-dependent helper and pass after the helper becomes path independent.
- **What:** executor inheritance removes new and legacy managed helper forms without disturbing
  unrelated indexed Git configuration.
  **File:** `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`.
  **How:** table-driven filtering tests over ordered indexed configuration blocks.
- **What:** a non-interactive Bash login shell that replaces `PATH` runs an existing `BASH_ENV`
  hook and then resolves both managed shims ahead of ambient tools.
  **Files:** `apps/backend/cmd/agentctl/github_cli_shim_test.go` and
  `apps/backend/internal/agentctl/server/config/config_test.go`.
  **How:** install real temporary shims plus a marker hook, collect a broker-enabled instance
  environment, and execute a short-lived Bash login subprocess. Assert the marker and resolved
  executable paths without invoking the credential broker.
- **What:** agentctl Unix command processes restore the managed shim path after login initialization,
  while broker-disabled commands preserve their existing runtime behavior apart from the
  broker-conditional wrapper.
  **File:** `apps/backend/internal/agentctl/server/process/shell_test.go`.
  **How:** unit-test the broker-conditional shell prelude and run a short-lived `sh -lc` process
  with a login profile that deliberately resets `PATH` and a non-secret marker executable.

## E2E Tests

No Playwright test is planned. There is no UI change, and a browser fixture would either expose a
credential environment or merely duplicate the subprocess boundary exercised by focused backend
tests.

## Verification Results

- Task 01 RED/GREEN and focused helper checks passed; see
  `task-01-path-independent-git-helper.md`.
- Task 02 RED/GREEN, generated-startup-script, affected-package, lifecycle, and Git-config checks
  passed; see `task-02-login-shell-shim-restoration.md`.
- Local semantic review found two lifecycle edge cases after Task 02: remote preparation ran before
  the shim directory existed, and parameterized parent `BASH_ENV` paths were sourced literally.
  Task 03 records their TDD remediation. Its RED checks failed at all four intended boundaries;
  the focused remediation suite passed 73 tests and the complete affected-package suite passed
  2,000 tests across six packages.
- PR #2141 review found that Local and Worktree preparation also precedes task-instance creation,
  so those host paths still lacked the launcher-owned helper executable. It also found an
  overflow-prone Sprites slice-capacity expression. Task 04 records the TDD and CodeQL remediation;
  seven focused regressions and 2,969 tests across five affected packages passed, and the CI-style
  changed-file Go lint reported no issues.
- `git diff --check` passed. No temporary artifacts remain.

## Implementation Waves And Parallel Candidates

Execute sequentially in the primary conversation. The tasks change the shared managed GitHub
runtime-environment contract, so they are not parallel-safe.

- [x] [Task 01: Path-independent Git credential helper](task-01-path-independent-git-helper.md)
- [x] [Task 02: Login-shell shim restoration](task-02-login-shell-shim-restoration.md)
- [x] [Task 03: Review blocker remediation](task-03-review-blocker-remediation.md)
- [x] [Task 04: PR review fixup](task-04-pr-review-fixup.md)

## Risks And Out Of Scope

- Shell startup hooks are an execution boundary. The repair must compose an existing `BASH_ENV`,
  avoid recursive sourcing, and keep all secret broker values out of script contents and argv.
- Git helper resolution is shell-independent after Task 01. Task 02 covers non-interactive Bash
  launched by agents and agentctl's Unix `sh -lc` processes; it does not rewrite user profile files
  or attempt to override a command that deliberately changes `PATH` after startup.
- Windows does not use the Unix login-shell wrapper and retains its current managed PATH behavior.
- No database, API, UI, credential-policy, lease, token-cache, or persistence changes are included.
