---
id: "01-unify-setup-timeout-paths"
title: "Unify setup timeout paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/setup-launch-timeout.md"
---

# Task 01: Unify Setup Timeout Paths

## Acceptance

- `KANDEV_TASK_PREPARATION_TIMEOUT` is read once at process start, defaults to
  10 minutes, and accepts only positive Go duration values.
- `AgentLaunchTimeout` is always the resolved setup value plus five minutes.
- Repository, Local, Worktree, Docker, Sprite, and SSH setup paths use the
  common setup value without changing their failure semantics.

## Verification

```bash
cd apps/backend && go test ./internal/common/constants ./internal/agent/runtime/lifecycle ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/common/constants/timeouts.go`
- `apps/backend/internal/common/constants/timeouts_test.go`
- `apps/backend/internal/backendapp/worktree.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_setup_script_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_health_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_scripts.go`

## Dependencies

None.

## Parallelism

`sequential`. Task 02 consumes the timeout values and lifecycle behavior from
this task.

## Inputs

- Setup and Launch Timeout spec, especially the default, override, and failure
  scenarios.
- ADR-2026-08-12-setup-timeout-owns-launch-budget.
- Existing `getEnvDuration` fallback pattern in
  `internal/agentctl/server/config/config.go`.
- Existing detached repository setup handling in
  `internal/worktree/script_message_handler.go`.

## Output contract

Report the RED tests, resolved timeout policy, each setup path changed, exact
verification results, residual risks, and update this task plus `plan.md`
status.

## Results

- Added the process-start setup timeout policy with a 10-minute default, positive-duration parsing, and fallback for unset, malformed, zero, or negative values.
- Applied the policy to local/worktree setup scripts, Docker bootstrap preparation, Sprites preparation, and SSH preparation while preserving each executor's existing failure semantics; Docker readiness uses the derived launch budget after the bounded prepare command.
- RED verification first demonstrated that setup-script and container-health operations ignored the configured timeout; the focused regressions now pass.
- Verification: `cd apps/backend && go test ./internal/common/constants ./internal/agent/runtime/lifecycle ./internal/backendapp` passed with 1,844 tests across 3 packages.
- The environment value is read when the backend process starts. Changing it requires a backend restart, as specified.
