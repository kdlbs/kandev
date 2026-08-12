---
spec: docs/specs/platform/setup-launch-timeout.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Setup and Launch Timeout

## Overview

Issue #2574 exposes two competing deadlines. Repository setup can continue past
the lifecycle manager's one-minute shared-launch limit, so runtime creation
receives an expired context and reports an `agentctl` readiness error. The
repair first creates one process-start setup-timeout policy and applies it to
all prepare paths. It then derives every shared launch deadline from that
policy and documents the operator contract.

## Backend

### Process timeout policy

- Update `apps/backend/internal/common/constants/timeouts.go` to resolve
  `KANDEV_TASK_PREPARATION_TIMEOUT` once at process start. Keep the exported
  `SetupScriptTimeout` value for existing consumers, change its default to 10
  minutes, and derive `AgentLaunchTimeout` as `SetupScriptTimeout + 5m`.
- Add a pure parser helper so `apps/backend/internal/common/constants/timeouts_test.go`
  can cover missing, valid, invalid, zero, and negative values without changing
  process-global state.

### Setup-path alignment

- In `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`, give
  each profile prepare script a child context bounded by
  `constants.SetupScriptTimeout`.
- Keep the worktree repository-script handler in
  `apps/backend/internal/backendapp/worktree.go` on the same value.
- Replace `spritePrepareTimeout` and `sshPrepareTimeout` with the common setup
  value in `executor_sprites_operations.go` and `executor_ssh_scripts.go`.
- Update `ContainerManager.waitForHealth` in `container.go` so the Docker
  bootstrap, which runs the prepare script before `agentctl`, waits for the
  common setup limit instead of the fixed two-minute retry count. Preserve
  caller cancellation and the last health error.

### Shared launch deadline

- Replace `coalescedExecutionCreationTimeout` in
  `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go` with
  `constants.AgentLaunchTimeout`. Keep `context.WithoutCancel` and manager-stop
  cancellation so one short-lived caller cannot end a launch needed by another
  caller.
- Update comments and timeout assertions that still describe a 60-second
  shared launch.

## Tests

- **What:** timeout parsing and the 10-minute and 15-minute defaults.
  **File:** `apps/backend/internal/common/constants/timeouts_test.go`
  **How:** table-driven unit tests for empty, valid, malformed, zero, and
  negative values.
- **What:** a shared execution can cross the old one-minute boundary and still
  complete.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
  **How:** `testing/synctest` regression that completes after 90 virtual seconds
  and before `constants.AgentLaunchTimeout`.
- **What:** a blocked shared execution ends at the derived launch deadline and
  releases its activity lease.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
  **How:** update the existing manager-deadline `testing/synctest` assertion.
- **What:** local prepare scripts and Docker bootstrap readiness use the common
  setup limit while preserving cancellation and setup-failure behavior.
  **Files:** `env_preparer_setup_script_test.go` and a focused new
  `container_health_test.go` beside `container.go`.
  **How:** focused context-deadline tests with virtual time or a short injected
  context. Do not add a real 10-minute test.
- **What:** existing launch waiters use the derived agent-launch value.
  **Files:** existing backend tests that assert `constants.AgentLaunchTimeout`.
  **How:** run the affected package tests after the constant change.

## Verification Results

- Task 01: `cd apps/backend && go test ./internal/common/constants ./internal/agent/runtime/lifecycle ./internal/backendapp` passed with 1,840 tests across 3 packages.
- Task 02: `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers` passed with 4,267 tests across 4 packages.
- Focused lifecycle regressions passed under `go test -race`, covering setup-script timeout, container health timeout, and shared launch deadline behavior.
- `git diff --check` passed.

## Implementation Waves And Parallel Candidates

Execution is sequential because Task 02 depends on the common policy from Task
01 and both tasks touch lifecycle timeout behavior.

- [x] [Task 01: Unify setup timeout paths](task-01-unify-setup-timeout-paths.md) (completed)
- [x] [Task 02: Align shared launch and docs](task-02-align-shared-launch-and-docs.md) (completed)

## Documentation Impact

Update `docs/public/configuration.md` with the exact environment variable,
duration syntax, fallback behavior, restart requirement, and derived 15-minute
default launch limit. Update `docs/public/executors.md` to state that prepare
scripts use the common setup limit while their fatal or non-fatal behavior
remains runtime-specific.

## Risks

- Docker runs its prepare script before `agentctl`, so readiness waiting is the
  only host-side boundary that can stop a hung bootstrap.
- The setup timeout is process-global. Tests must not depend on changing the
  environment after package initialization.
- A longer correct deadline delays failure for a truly blocked runtime. The
  derived 15-minute bound and activity-lease regression keep that case finite.
