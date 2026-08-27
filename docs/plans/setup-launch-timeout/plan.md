---
spec: docs/specs/platform/requirements/setup-launch-timeout.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Setup and Launch Timeout

## Overview

Issue #2574 exposes two competing deadlines. Repository setup can continue past
the lifecycle manager's one-minute shared-launch limit, so runtime creation
receives an expired context and reports an `agentctl` readiness error. The
repair first creates one process-start setup-timeout policy and applies it to
all prepare paths. It then derives runtime launch-phase deadlines from that
policy, keeps preparation in an independent context, and documents the
operator contract.

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
- Update the Docker bootstrap and `ContainerManager.waitForHealth` in
  `container.go`: enforce the common setup limit around the prepare command so
  a timeout remains non-fatal, then give `agentctl` readiness the derived
  launch budget. Preserve caller cancellation and the last health error.

### Launch-phase deadlines

- Keep the coalesced manager context independent of the launch-phase deadline
  so environment resolution cannot consume the runtime launch budget.
- Start a fresh `constants.AgentLaunchTimeout` context after environment
  resolution, and let each runtime give its setup script an independent
  `constants.SetupScriptTimeout` context.
- Update comments and timeout assertions that still describe a single shared
  wall-clock launch.

## Tests

- **What:** timeout parsing and the 10-minute and 15-minute defaults.
  **File:** `apps/backend/internal/common/constants/timeouts_test.go`
  **How:** table-driven unit tests for empty, valid, malformed, zero, and
  negative values.
- **What:** coalesced environment preparation can cross the old one-minute
  boundary without consuming the runtime launch-phase budget.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/execution_context_test.go`
  **How:** `testing/synctest` advances virtual time through preparation, then
  verifies a fresh launch-phase deadline and an independent setup deadline.
- **What:** a blocked runtime launch phase ends at the derived launch deadline
  and releases its activity lease.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
  **How:** update the existing manager-deadline `testing/synctest` assertion.
- **What:** local prepare scripts and Docker bootstrap readiness use the common
  setup limit while preserving cancellation and setup-failure behavior.
  **Files:** `env_preparer_setup_script_test.go` and a focused new
  `container_health_test.go` beside `container.go`.
  **How:** focused context-deadline tests with virtual time or a short injected
  context. Do not add a real 10-minute test.
- **What:** existing launch phases use the derived agent-launch value while
  preparation retains its full independent setup value.
  **Files:** existing backend tests that assert `constants.AgentLaunchTimeout`.
  **How:** run the affected package tests after the constant change.

## Verification Results

- Task 01: `cd apps/backend && go test ./internal/common/constants ./internal/agent/runtime/lifecycle ./internal/backendapp` passed with 1,844 tests across 3 packages.
- Task 02: `cd apps/backend && go test -count=1 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers` passed with 4,587 tests across 4 packages.
- Focused lifecycle regressions passed under `go test -race`, covering setup-script timeout, container health timeout, independent launch/preparation contexts, and launch-phase deadline behavior.
- The host-mode Docker launch E2E suite passed 12 tests with one pre-existing `fixme` skipped; the Alpine BusyBox preparation test passed three consecutive repetitions without retries.
- The web typecheck, E2E fixture Prettier check, focused Go vet, public-doc validation, and public-doc validation tests passed.
- `git diff --check` passed.

## Implementation Waves And Parallel Candidates

Execution is sequential because Task 02 depends on the common policy from Task
01 and both tasks touch lifecycle timeout behavior.

- [x] [Task 01: Unify setup timeout paths](task-01-unify-setup-timeout-paths.md) (completed)
- [x] [Task 02: Align shared launch and docs](task-02-align-shared-launch-and-docs.md) (completed)

## Documentation Impact

Update `docs/public/configuration.md` with the exact environment variable,
duration syntax, fallback behavior, restart requirement, and derived 15-minute
default launch-phase limit. Update `docs/public/executors.md` to state that
prepare scripts use the common setup limit while their fatal or non-fatal
behavior remains runtime-specific.

## Risks

- Docker runs its prepare script before `agentctl`, so the bootstrap timeout and
  readiness phase are separate boundaries that can stop a hung launch.
- The setup timeout is process-global. Tests must not depend on changing the
  environment after package initialization.
- A longer correct launch-phase deadline delays failure for a truly blocked
  runtime. The derived 15-minute phase bound and activity-lease regression keep
  that case finite.
