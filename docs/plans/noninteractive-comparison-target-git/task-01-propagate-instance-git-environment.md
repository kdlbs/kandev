---
id: "01-propagate-instance-git-environment"
title: "Propagate the instance Git environment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 01: Propagate the instance Git environment

Make every workspace tracker use the instance's effective Git environment. Enforce non-interactive prompt controls without removing managed credentials or explicit SSH configuration.

## Red tests first

Add focused tests that prove these behaviors:

- a tracker Git shim receives managed `GIT_CONFIG_COUNT`, `GIT_CONFIG_KEY_*`, and `GIT_CONFIG_VALUE_*` entries from `InstanceConfig.AgentEnv`
- the shim receives `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=Never`, `GIT_ASKPASS=echo`, and `SSH_ASKPASS=/bin/false`
- a direct OpenSSH `GIT_SSH_COMMAND` keeps its command and options while forcing `BatchMode=yes`; unsupported prefixes and wrappers use the safe non-interactive default
- `ssh -oBatchMode=yes` is present when the instance does not supply an SSH command
- an ambient credential value does not replace the instance value
- root, repository, submodule, rescan, and lazy trackers receive detached environment copies
- later mutation of the source slice or another tracker does not change a tracker's environment

Record the focused red command before production changes.

## Implementation

- Store a detached Git environment on `WorkspaceTracker`.
- Build the environment from `InstanceConfig.AgentEnv`, not `os.Environ()`.
- Add established prompt controls with last-value precedence.
- Preserve direct OpenSSH `GIT_SSH_COMMAND` options while placing `BatchMode=yes` before any inherited batch-mode option. Use the safe default for unsupported shell prefixes or wrappers, and add it when the variable is absent.
- Route `runGitOutput` and `runPollingGitOutput` through the same environment builder.
- Apply the environment in the process manager's central tracker configuration path.
- Cover trackers that the manager creates during rescan, reconciliation, submodule discovery, and lazy lookup.
- Keep direct test constructors compatible with an isolated default environment.

## Acceptance

- Comparison-target commands can use managed HTTPS credentials from the instance environment.
- Git, Git Credential Manager, and SSH cannot request terminal input.
- All manager-owned trackers use equivalent detached environments.
- Existing command timeout and throttle tests remain green.

## Likely files

- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_rescan.go`
- `apps/backend/internal/agentctl/server/process/manager_submodules.go`
- `apps/backend/internal/agentctl/server/process/manager_rescan_test.go`
- `apps/backend/internal/agentctl/server/process/manager_submodule_test.go`

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process -run 'TestWorkspaceGitEnvironment|TestManagerTrackerGitEnvironment|TestRunGitOutput' -count=1
cd apps/backend && go test ./internal/agentctl/server/process
```

## Parallelism

`sequential`. Task 02 consumes the tracker environment contract.

## Output contract

Record the red and green commands. Record the final environment precedence and proof for each tracker creation path.

## Implementation record

- Red: `cd apps/backend && go test ./internal/agentctl/server/process -run 'TestWorkspaceGitEnvironment|TestManagerTrackerGitEnvironment|TestRunGitOutput' -count=1` failed because tracker commands used ambient credentials and prompt settings.
- Green: the same focused command passed 5 tests after the change; the full process package passed 752 tests before Task 02 and 787 tests across process and instance after the complete package change.
- Precedence: the manager snapshots `InstanceConfig.AgentEnv`; each tracker receives a detached copy; lockless and per-observation `GIT_INDEX_FILE` overlays are applied; non-interactive controls override inherited values; a direct OpenSSH command keeps its options with `BatchMode=yes` placed first, while unsupported command shapes and missing commands use the safe default.
- Coverage: root, repository, submodule, rescan, reconciliation, and lazy tracker construction all pass through `configureTracker` or `newTrackerForRepo`, which applies the manager snapshot before Git work starts.

## PR fixup record

- Review fix: direct OpenSSH commands now receive `-oBatchMode=yes` immediately after the command word, so a later inherited `BatchMode=no` cannot re-enable terminal prompting; unsupported prefixes and wrappers fall back to the safe non-interactive command.
- Test fix: the Git-status freshness fixture now installs its command gate before manager construction and passes an explicit environment snapshot, so tracker commands use the same gate variables as the test without relying on ambient environment reads.
