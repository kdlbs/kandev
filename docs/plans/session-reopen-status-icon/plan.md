---
spec: docs/specs/platform/requirements/background-work-liveness.md
created: 2026-08-01
status: completed
---

# Fix Plan: Session Reopen Status Icon

## Overview

The task add-panel menu currently treats every `WAITING_FOR_INPUT` session as
an active clarification, even though that lifecycle state also means an
ordinary turn has finished and the session is ready for another prompt. The fix
makes the reopen-menu indicator depend on explicit per-session pending-input
projections while preserving background-running and terminal lifecycle icons.

## Confirmed Root Cause

`shouldShowReopenStateIcon` returns `true` for an input-capable
`WAITING_FOR_INPUT` session after both pending-input checks are false. The row
then calls `getSessionStateIcon`, whose lifecycle fallback maps
`WAITING_FOR_INPUT` to `IconMessageQuestion`. The Dockview tab bar does not use
that fallback and instead shows the agent logo when the session is not active,
which creates the visible mismatch.

The smallest reliable reproduction is a normal mock-agent turn that settles to
`WAITING_FOR_INPUT` without a pending clarification or permission: opening the
Dockview `+` menu shows a question icon on that session row.

## Frontend

### Reopen-menu status derivation

- Update `apps/web/components/task/session-reopen-menu.tsx` so
  `shouldShowReopenStateIcon` returns `false` for a plain
  `WAITING_FOR_INPUT` session.
- Continue returning `true` when an input-capable session has an explicit
  pending clarification or permission, or when it carries background activity.
- Carry the pending action as a per-session boot/list projection so a closed
  session row can retain the actionable icon even when its transcript is not
  hydrated; loaded messages remain authoritative when available.
- Preserve the current behavior for `STARTING`, generating `RUNNING`, and
  terminal/created lifecycle states.
- Do not change the shared `getSessionStateIcon` mapping; other status surfaces
  and their labels are outside this repair.

### Responsive and mobile contract

This is a content/state-normalization change inside the existing Radix menu;
it does not alter composition, navigation, scrolling, safe-area handling, or
touch targets. The Dockview add-panel control is not part of the dedicated
phone task layout. The nearest mobile semantic exemplar is
`apps/web/components/task/mobile/mobile-sessions-section.tsx`, which already
uses explicit pending-input flags to distinguish real clarification and
permission variants. No mobile-specific presentation change is planned; the
pure helper regression matrix is viewport-independent.

## Tests

- **What:** a plain `WAITING_FOR_INPUT` session does not request a reopen-menu
  state icon, while explicit clarification/permission and background activity
  still do.
- **File:** `apps/web/components/task/session-reopen-menu.test.tsx`.
- **How:** update the existing Vitest helper matrix. First change the plain
  waiting assertion to the desired `false` result and run it against current
  production code to prove RED; then implement the minimal helper change and
  rerun for GREEN.

## E2E Tests

- **Scenario:** **GIVEN** a task has a secondary `WAITING_FOR_INPUT` session
  with a pending clarification, **WHEN** the user reloads the task and the
  secondary transcript is not loaded before opening the Dockview add-panel
  menu, **THEN** its row still contains the message-question icon (and not the
  shield-question icon); the ordinary idle-session scenario remains icon-free.
- **File:** `apps/web/e2e/tests/session/multi-session-ux.spec.ts`.
- **What to verify:** seed the secondary pending message, reload the task,
  evict that session's transcript to model an unloaded row, open the existing
  `dockview-add-panel-btn`, scope the assertion to
  `reopen-session-<session-id>`, and assert the projected icon variant. Keep
  the existing idle-waiting check for the absence of both misleading glyphs.
- **Mobile coverage:** no separate mobile Playwright scenario is added because
  the Dockview add-panel menu is absent from the phone composition and this
  repair changes only viewport-independent state normalization. Existing
  mobile session-picker tests continue proving that actual pending
  clarifications and permissions retain their distinct icons.

## Implementation Task

- [x] [Task 01: Correct the reopen-menu input indicator](task-01-correct-reopen-input-indicator.md)

Execution is sequential in the primary conversation. This single task is not a
parallel candidate and does not authorize subagent use.

## Environment Prerequisite

This worktree currently lacks `apps/node_modules`. Before the RED run, execute
`pnpm install --frozen-lockfile` from `apps/`; the lockfile must remain
unchanged.

## Risks and Boundaries

- The helper must not suppress background-running or terminal-state icons.
- The E2E must prove the exact backend state so a terminal `COMPLETED` session
  cannot make the regression assertion pass accidentally.
- Shared lifecycle icon mappings, session labels, tab-bar presentation, and
  mobile session-picker presentation remain out of scope.
