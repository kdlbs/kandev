---
spec: docs/specs/tasks/requirements/task-create-executor-default.md
created: 2026-08-01
status: complete
---

# Implementation Plan: Task Create Executor Default

## Overview

Change the task-create selection order so source/workspace policy resolves the executor before a
portable last-used profile is considered. Lock the behavior down first with focused Vitest
regressions, then prove the backend-settings/cross-browser path through the existing Create Task
Playwright flow. No backend, schema, API, or rendered-layout change is required.

## Confirmed root cause

`pickDefaultExecutorProfileId` accepts any eligible `lastUsedExecutorProfileId` before calling
`pickDefaultExecutorId`. For an ordinary repository-backed task, a valid saved Local profile is
therefore returned before the no-workspace-default Worktree fallback is resolved. The executor-ID
effect also waits for any valid saved profile and then derives the executor from that profile,
making the Local selection portable across browsers and updates by design.

## Frontend

### Executor policy and profile restoration

- Update `apps/web/components/task-create-dialog-effects.ts` so
  `pickDefaultExecutorProfileId` first obtains the executor selected by `pickDefaultExecutorId`.
- Restore `lastUsedExecutorProfileId` only when its flattened profile candidate belongs to that
  resolved executor.
- Prefer the resolved executor's first profile when the saved profile belongs to another executor.
  Preserve the current first-eligible fallback if the resolved executor has no usable profile.
- Update `shouldWaitForLastUsedExecutorProfile` so executor-ID auto-pick waits only for a saved
  profile compatible with the resolved executor. A saved Local profile must not suppress the
  Worktree executor-ID fallback for an ordinary repository-backed task.
- Preserve the current task-source rules in `pickDefaultExecutorId`: explicit unmanaged local
  paths and repository-less tasks prefer direct Local; a valid workspace default wins; otherwise
  `DEFAULT_LOCAL_EXECUTOR_TYPE` (`worktree`) is preferred.
- Keep task-create persistence and submit handlers unchanged. A manual selection still applies to
  the current task and is still recorded by the backend.

### Mobile design contract

- **Desktop and mobile outcome:** the existing Executor Profile selector shows the same
  policy-resolved profile for the same task source and workspace settings.
- **Nearest shipped mobile exemplar:** the existing mobile Create Task dialog remains the
  composition and interaction baseline.
- **Presentation and hierarchy:** no markup, overlay, scrolling, safe-area, focus, or touch target
  changes; only the shared selection state changes.
- **Shared logic:** desktop and mobile already call the same task-create effects, so the fix stays
  in that shared hook.
- **Mobile proof:** focused unit coverage is sufficient because this is state normalization inside
  the unchanged dialog. Existing mobile task-create E2E remains the layout/touch coverage; no new
  mobile-only interaction is introduced.

## Tests

- **What:** a saved Local profile cannot override the Worktree fallback for an ordinary
  repository-backed task with no workspace default.
  - **File:** `apps/web/components/task-create-dialog-effects-executor.test.ts`
  - **How:** render `useDefaultSelectionsEffect` with Local and Worktree executors, loaded backend
    settings, and `lastUsedExecutorProfileId` set to Local; assert Worktree profile and executor ID.
- **What:** an explicit Local workspace default remains authoritative.
  - **File:** `apps/web/components/task-create-dialog-effects-executor.test.ts`
  - **How:** use the same executor set with `default_executor_id` set to Local and a saved Worktree
    profile; assert Local profile and executor ID.
- **What:** source constraints remain authoritative for an explicit local path and repository-less
  task.
  - **File:** `apps/web/components/task-create-dialog-effects-executor.test.ts`
  - **How:** extend the existing local-path/no-repository cases with a saved Worktree profile and
    assert a direct Local profile.
- **What:** a saved alternative Worktree profile is still restored when Worktree is the resolved
  executor, and missing-profile fallback behavior remains unchanged.
  - **File:** `apps/web/components/task-create-dialog-effects-executor.test.ts`
  - **How:** retain the existing restoration case and add/adjust table cases around resolved
    executor membership.

## E2E Tests

- **Scenario:** **GIVEN** backend user settings contain a last-used Local profile and the workspace
  has no executor default, **WHEN** Create Task opens for the seeded repository in a browser
  context, **THEN** the Executor Profile selector shows Worktree.
  - **File:** `apps/web/e2e/tests/task/create-task-executor-default.spec.ts`
  - **What to verify:** seed the Local profile through `saveUserSettings`, navigate through the
    regular Kanban UI, open the dialog, and assert the visible Worktree selection.
- **Scenario:** **GIVEN** the workspace explicitly defaults to Local, **WHEN** Create Task opens for
  the seeded repository, **THEN** the selector shows Local even when the saved profile is Worktree.
  - **File:** `apps/web/e2e/tests/task/create-task-executor-default.spec.ts`
  - **What to verify:** set and later restore `default_executor_id`, assert the visible Local
    profile, and restore the backend last-used profile to Worktree so the worker-scoped fixture
    does not leak state.

## Implementation Tasks

- [x] [Task 01: Enforce executor-first profile selection](task-01-executor-first-selection.md)
- [x] [Task 02: Prove portable settings behavior in Create Task](task-02-create-task-e2e.md)

Execution is sequential in the primary conversation. No subagent delegation is planned or
authorized.

## Validation

Run from the repository root unless noted otherwise:

- `cd apps/web && pnpm test -- --run components/task-create-dialog-effects-executor.test.ts components/task-create-dialog-executor-wait.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-executor-default.spec.ts`

Completed validation: 14 focused Vitest tests passed, the web typecheck passed, targeted ESLint and
Prettier checks passed, and the managed Chromium run passed both Create Task scenarios.

The managed E2E runner performs the required production build before Playwright. This worktree ran
`cd apps && pnpm install --frozen-lockfile` before the recorded validation; fresh worktrees must run
that prerequisite if frontend dependencies are missing.

## Risks

- The executor-ID and executor-profile effects run independently and settle through microtasks.
  Tests must assert the final paired selection, not only one setter call, so a transient mismatch
  cannot become a regression.
- Existing E2E fixtures preserve task-create last-used fields on partial settings writes. New tests
  must restore a Worktree profile, and tests that set a workspace default must clear it in cleanup.
- The single stored last-used profile cannot retain a preferred profile for multiple executor
  types; adding per-executor history remains explicitly out of scope.
