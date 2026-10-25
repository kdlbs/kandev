---
id: "03-project-refresh-launch-failures"
title: "Project refresh launch failures"
status: done
wave: 3
depends_on:
  - "02-reject-stale-fallbacks"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 03: Project Refresh Launch Failures

## Summary

Carry a required-refresh failure through lifecycle and task launch boundaries.
Prove that Kandev starts no agent and shows the durable repository-specific
error on desktop and mobile.

## In scope

- Add a failing multi-repository launch test where the second repository cannot
  complete required refresh.
- Preserve exact repository identity while wrapping refresh errors.
- Use the existing generic typed launch-failure category unless a focused test
  proves a new category is necessary for a valid recovery action.
- Bound and redact error detail through existing launch-error helpers.
- Prove that no agent runtime starts after a required refresh error.
- Extend existing desktop and mobile launch-failure recovery E2E fixtures with
  a stale-origin refresh failure and reload assertion.
- Prove that normal retry succeeds after the remote and credentials are fixed.

## Out of scope

- A new recovery action, WebSocket schema, or task-error store.
- New UI components, responsive layout, or translations.
- Partial agent launch for multi-repository tasks.

## Acceptance

- A required refresh error prevents runtime startup and names the exact failed
  repository without exposing credentials or an authenticated URL.
- The active task error survives reload and renders through the existing task
  error surface on desktop and mobile.
- A later normal retry reruns refresh and can launch after the underlying Git
  problem is corrected.

## Verification

Start with the multi-repository no-start regression and confirm it fails before
error propagation is complete. Then run:

```bash
# From apps/backend:
rtk go test ./internal/orchestrator/... ./internal/orchestrator/executor/... -run 'LaunchFailure|RepositoryRefresh|MultiRepo' -race
rtk go test ./internal/orchestrator/... ./internal/worktree/... -race

# From the repository root:
rtk make -C apps/backend build
rtk make -C apps/backend e2e-plugin-package

# From apps/web:
rtk pnpm run build:e2e
rtk pnpm e2e:raw --project=chromium e2e/tests/task/launch-failure-recovery.spec.ts
rtk pnpm e2e:raw --project=mobile-chrome e2e/tests/task/mobile-launch-failure-recovery.spec.ts

# From the repository root:
rtk make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
- `apps/backend/internal/orchestrator/task_launch_failure_test.go`
- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`

## Dependencies

- Task 02 provides the required-refresh error from the worktree boundary.

## Risks

- Raw Git output can contain a credential-bearing URL. Test redaction with a
  synthetic secret and assert that it is absent from logs and projected detail.
- An E2E fixture that fails clone instead of refresh does not prove this bug.
  Seed a valid cached checkout, make its origin fail, and request a new
  worktree.
- Backend E2E artifacts must be rebuilt before Playwright starts.

## Parallelism

`sequential`

## Inputs

- `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6` and `.7`.
- `REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001` as an adjacent task-system
  contract.
- Existing desktop and mobile launch-failure recovery E2E tests.

## Results

- Multi-repository preparation preserves exact repository identity and rolls
  back preparation without starting an agent runtime.
- Launch error details use existing credential-safe sanitization while keeping
  repository-specific recovery actions.
- Desktop and mobile launch-failure fixtures cover a valid cached checkout with
  a failing origin, reload persistence, and successful retry after recovery.
- Verified with the focused and full orchestrator/worktree race suites, backend
  build, E2E plugin package, production E2E build, Chromium and mobile-chrome
  specs, and backend lint.
