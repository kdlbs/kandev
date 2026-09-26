---
spec: docs/specs/tasks/requirements/task-create-executor-default.md
created: 2026-08-18
status: complete
---

# Implementation Plan: Reset Executor When Returning to Repo

## Overview

Reset the task-create executor selection whenever the dialog leaves repository-less mode so the
existing source-aware default effect can resolve the Repo destination again. Lock the transition
down with a focused handler regression, then exercise the complete Repo to None to Repo sequence in
the existing executor-default Playwright suite. No backend, persistence, API, copy, or layout change
is required.

## Confirmed root cause

`handleToggleNoRepository` in
`apps/web/components/task-create-dialog-handlers.ts` clears `executorId` and
`executorProfileId` only when entering repository-less mode. The source-aware auto-pick effect in
`apps/web/components/task-create-dialog-effects.ts` intentionally returns early while either
selection is populated. After repository-less mode selects Local, leaving the mode clears only
`workspacePath`. The stale Local profile therefore prevents Repo mode from reapplying the workspace
default or Worktree fallback.

The current focused handler and executor-effect suites pass 36 tests, but they do not cover the
repository-less-to-Repo transition.

## Frontend

### Source transition reset

- Update `handleToggleNoRepository` to clear `executorId` and `executorProfileId` when switching in
  either direction.
- Preserve the current mutually exclusive source flags, repository and branch selections, remote
  repository state, workspace-path cleanup, and last-used source cleanup.
- Reuse `useDefaultSelectionsEffect` for destination-specific selection. Repository-less mode
  continues to prefer direct Local. Repo mode honors a valid workspace default and otherwise
  prefers Worktree.
- Do not hardcode a Worktree profile in the handler. The existing policy remains authoritative for
  workspaces that explicitly default to Local and for fallback behavior when Worktree is absent.

### Mobile parity

- **Desktop and mobile outcome:** the responsive Create Task dialog applies the same executor policy
  after the Repo to None to Repo sequence.
- **Nearest shipped mobile exemplar:** the current mobile Create Task dialog remains the interaction
  and composition baseline.
- **Presentation and state:** no markup, navigation, overlay, scrolling, safe-area, focus, or touch
  behavior changes. The fix stays in the shared dialog handlers and selection effects.
- **Coverage:** the focused unit regression plus the desktop real-dialog E2E establish the shared
  state transition. A new mobile-only Playwright test is not required because this is state
  normalization inside an unchanged responsive component.

## Tests

- **What:** leaving repository-less mode invalidates the Local executor selection so destination
  policy can run again.
  - **File:** `apps/web/components/task-create-dialog-handlers.test.ts`
  - **How:** render `useDialogHandlers` with repository-less mode active and a Local executor.
    Invoke `handleToggleNoRepository`. Assert that both executor setters receive an empty value and
    that the handler clears the workspace path.
- **What:** entering repository-less mode retains its current reset behavior.
  - **File:** `apps/web/components/task-create-dialog-handlers.test.ts`
  - **How:** keep or add the paired entry assertion so a later refactor cannot make the reset
    one-directional again.
- **What:** an empty repository-backed selection still resolves to Worktree through existing policy.
  - **File:** `apps/web/components/task-create-dialog-effects-executor.test.ts`
  - **How:** retain the existing executor-first/default-policy cases as companion coverage. This
    fix does not introduce a new policy branch.

## E2E Tests

- **Scenario:** **GIVEN** Repo with no executor default, **WHEN** the user selects None and Repo,
  **THEN** the visible executor changes from Worktree to Local to Worktree.
  - **File:** `apps/web/e2e/tests/task/create-task-executor-default.spec.ts`
  - **Verification:** use the source-mode test IDs and the visible executor-profile selector.
    Restore the workspace default and task-create preference during cleanup. Use the managed
    production build.

## Verification Results

Task 01 and Task 02 are complete. During diagnosis, the unchanged baseline command
`cd apps/web && pnpm test -- --run components/task-create-dialog-handlers.test.ts components/task-create-dialog-effects-executor.test.ts`
passed 36 tests across 2 files. The Task 01 implementation command passed 37 tests across 2 files,
and the web typecheck passed. The managed Chromium command
`cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-executor-default.spec.ts`
passed all 3 tests after rebuilding the production backend and Vite assets.

## Implementation Tasks

Wave 1:

- [x] [Task 01: Reset executor on source transition](task-01-reset-source-executor.md)

Wave 2:

- [x] [Task 02: Prove the real dialog transition](task-02-dialog-transition-e2e.md)

Execution is sequential in the primary conversation. No subagent delegation is planned or
authorized.

## Risks

- The executor-ID and executor-profile effects settle through microtasks. Unit assertions prove
  invalidation at the handler boundary. Playwright proves the final visible selection.
- Hardcoding Worktree in the toggle handler would break explicit Local workspace defaults. The
  handler must only invalidate stale state and let the existing policy select the destination.
- The E2E fixture persists workspace and task-create settings across tests, so cleanup must restore
  both values even after a failed assertion.

## Out of scope

- Changing executor-default precedence or persistence.
- Changing Repo, Remote, or None presentation and labels.
- Changing repository, branch, or remote-source retention while switching modes.
