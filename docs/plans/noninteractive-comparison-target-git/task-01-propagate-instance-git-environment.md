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
- an instance `GIT_SSH_COMMAND` remains unchanged
- `ssh -oBatchMode=yes` is present when the instance does not supply an SSH command
- an ambient credential value does not replace the instance value
- root, repository, submodule, rescan, and lazy trackers receive detached environment copies
- later mutation of the source slice or another tracker does not change a tracker's environment

Record the focused red command before production changes.

## Implementation

- Store a detached Git environment on `WorkspaceTracker`.
- Build the environment from `InstanceConfig.AgentEnv`, not `os.Environ()`.
- Add established prompt controls with last-value precedence.
- Preserve an explicit `GIT_SSH_COMMAND`. Add the batch-mode default only when this variable is absent.
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
- Precedence: the manager snapshots `InstanceConfig.AgentEnv`; each tracker receives a detached copy; lockless and per-observation `GIT_INDEX_FILE` overlays are applied; non-interactive controls override inherited values; the batch-mode SSH default is appended only when the instance has no `GIT_SSH_COMMAND`.
- Coverage: root, repository, submodule, rescan, reconciliation, and lazy tracker construction all pass through `configureTracker` or `newTrackerForRepo`, which applies the manager snapshot before Git work starts.
