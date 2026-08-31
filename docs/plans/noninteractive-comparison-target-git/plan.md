---
spec: docs/specs/platform/requirements/workspace-git-status.md
created: 2026-08-31
status: implemented
---

# Implementation Plan: Non-interactive Comparison-target Git

## Overview

Remove the password prompt that blocks Kandev startup after a comparison-target fetch. Workspace trackers will use the instance's Git environment and established non-interactive controls. Comparison-target network work will run outside instance readiness and Git-status request paths.

This plan implements `AC-PLATFORM-WORKSPACE-GIT-STATUS-001.16` through `.18`.

## Confirmed root cause

Release `v0.92.0` added repository-qualified comparison-target materialization. The regression remains in `v0.92.2` and current `main`.

The blocking path has three parts:

- `WorkspaceTracker.runGitOutput` uses `os.Environ()` instead of `InstanceConfig.AgentEnv`.
- The command environment does not enforce non-interactive Git controls.
- Instance creation and lazy tracker creation wait for comparison-target materialization.

As a result, Git can read a user name and password from the terminal that launched Kandev.

## Invariants

- Kandev-owned comparison-target commands never read the launcher terminal.
- Workspace trackers use the exact instance Git credential environment.
- Managed `GIT_CONFIG_*` credential-helper entries remain intact.
- Direct OpenSSH commands and their options remain intact. Kandev forces `BatchMode=yes` before inherited batch-mode options; unsupported shell prefixes and wrappers use the safe non-interactive default.
- Git command deadlines and throttle admission remain in force.
- A target is pending and fail-closed before its fetch starts.
- Instance readiness, live target updates, rescan, and lazy tracker creation do not wait for network Git.
- Manager shutdown cancels unfinished target work.
- An HTTPS error does not cause an automatic SSH retry.
- `origin`, the checkout upstream, and push routing do not change.

## Design

### Tracker Git environment

The process manager owns a detached copy of `InstanceConfig.AgentEnv`. It gives a separate copy to every tracker that it creates. This includes root, repository, submodule, rescan, and lazy trackers.

The tracker command helper composes this environment with the established non-interactive controls. It does not use the ambient agentctl environment. Test helpers can still create a tracker without an instance environment.

### Background target materialization

The process manager sets the selected target to pending before it schedules network work. One manager-owned background operation materializes each current repository target.

The operation uses the manager lifetime context and the existing Git-command deadline. A newer target replaces stale work. A stale result cannot overwrite the current target state.

On success, the manager publishes the exact comparison ref and starts a detached status refresh. On error, it publishes the existing bounded unavailable code. Callers receive control after scheduling the work.

### Transport policy

Comparison targets keep their canonical HTTPS URLs. The command uses the instance credential environment once. It does not construct or try an SSH URL after an HTTPS error.

The policy is recorded in [ADR: Keep Internal Git Transport Deterministic and Non-interactive](../../decisions/2026-08-31-deterministic-noninteractive-git-transport.md).

## Test strategy

- A Git shim records the command environment and proves that managed credential configuration reaches the tracker.
- The shim proves that prompt controls override interactive values.
- Tracker-graph tests cover root, repository, submodule, rescan, and lazy tracker creation.
- A blocking Git shim proves that instance preparation and lazy tracker lookup return before the fetch ends.
- State tests prove pending, ready, unavailable, stale-result rejection, and shutdown cancellation.
- Command tests prove that one HTTPS error does not start an SSH retry.
- The existing fork-PR browser scenario proves that the session opens while its target is unavailable.

## Execution order

[x] 1. [Task 01: Propagate the instance Git environment](task-01-propagate-instance-git-environment.md) (done)
[x] 2. [Task 02: Detach comparison-target materialization](task-02-detach-comparison-target-materialization.md) (done)

Task 02 depends on Task 01. Background commands must use the correct environment before they leave the request path.

Both work orders are implemented. Validation results are recorded in the work-order implementation records; the remaining delivery steps cover commit, PR creation, and post-PR fixup.

The first PR fixup also serialized comparison-target publication with active-operation identity, removed stale-result recursion, and forced direct OpenSSH commands into batch mode. The second PR fixup gates target admission during manager teardown and preserves adapter and shell cleanup when target shutdown reports an error. Portable lifecycle fixtures run on Windows as well as Unix, and their nonblocking assertions use closed fetch gates rather than wall-clock thresholds.

## Validation

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/instance
make -C apps/backend lint
cd apps/web && pnpm e2e:run --host --no-build --project chromium -- e2e/tests/git/fork-pr-comparison-target.spec.ts
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Out of scope

- Automatic HTTPS-to-SSH fallback.
- A new private-repository credential scope.
- Changes to comparison-target identity or remote naming.
- Changes to the Git-status API or WebSocket payload.
- Changes to `origin`, upstream tracking, or push routing.
- New UI controls for Git credentials or transport selection.
