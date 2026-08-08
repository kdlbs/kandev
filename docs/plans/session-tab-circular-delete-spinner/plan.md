---
spec: docs/specs/ui/session-tab-delete-feedback.md
created: 2026-08-08
status: done
---

# Implementation Plan: Circular session-tab delete spinner

## Overview

Correct the pending visual state for the desktop session-tab delete action and silence routine
progress/success feedback when a session is promoted to primary. The existing delete confirmation,
error handling, cleanup ordering, and session-picker composition remain unchanged. The
implementation copies the compact circular border treatment already used by terminal-tab closing so
the two tab types communicate the same in-progress action.

## Root cause

`apps/web/components/task/session-tab-close-action.tsx` renders `GridSpinner` while deletion is
pending. The terminal-tab close path renders a compact circular border spinner in
`apps/web/components/task/terminal-tab.tsx` (`TerminalTabClosingSpinner`). The existing session-tab
component test proves that a status spinner exists but does not constrain its shape, so the visual
regression passes unnoticed. Separately, `setPrimary` in
`apps/web/hooks/domains/session/use-session-actions.ts` uses the default toast feedback mode, so a
successful primary promotion creates loading and success notifications even though the tab's star
and backend state provide the result.

## Frontend

### Session-tab close action

Update `apps/web/components/task/session-tab-close-action.tsx` to remove the `GridSpinner` import
and render a `role="status"` child with the same compact circular classes as the terminal close
spinner: a small rounded circle, muted border, spinning top border, and `animate-spin`. Preserve the
button's existing `disabled`, `aria-busy`, accessible delete label, pointer suppression, test ID,
and close callback behavior. Do not change the agent-activity `GridSpinner` in `session-tab.tsx` or
any other loading surface.

### Primary promotion feedback

Update `apps/web/hooks/domains/session/use-session-actions.ts` so `setPrimary` uses the existing
error-only `inline` feedback mode. Successful promotion must not create loading or success toasts;
failed promotion still creates one error toast with the existing failure detail. This is shared by
the desktop session-tab context menu and the mobile Sessions picker, so both surfaces keep the same
routine-action feedback without duplicating state or handlers.

### Mobile design contract

- **Desktop outcome:** the session-tab X remains in the existing Dockview tab strip and changes to
  the terminal-style circular spinner only while its confirmed delete request is pending.
- **Mobile entry point:** the existing Sessions picker and row actions in
  `apps/web/components/task/mobile/mobile-sessions-section.tsx`; phones have no Dockview session-tab
  X and therefore need no new control.
- **Nearest shipped mobile exemplar:** `mobile-sessions-section.tsx` remains the authoritative
  bottom-picker composition, touch target, hierarchy, and delete flow.
- **Shared state and logic:** `useSessionTabDelete` remains unchanged. `useSessionActions` owns the
  shared primary-promotion feedback mode, while both desktop and mobile retain their existing
  action/menu composition.
- **Scroll and surface behavior:** no mobile surface, scroll owner, safe-area handling, or viewport
  composition changes.
- **Parity proof:** the existing `apps/web/e2e/tests/session/mobile-session-deletion.spec.ts` keeps
  proving that phone users can delete a session from the native picker. A new mobile test is not
  needed because the primary-promotion change is shared hook behavior without a mobile composition
  change; the mobile picker remains covered by its existing rendered flow.

## Tests

- **What:** the pending session-tab close control uses the terminal-style circular spinner and does
  not render the grid-spinner markup.
  **File:** `apps/web/components/task/session-tab-close-action.test.tsx`.
  **How:** render the pending state and assert the status element has the circular border/spin
  classes, has no grid-spinner cubes, remains disabled/busy, and blocks activation.
- **What:** the existing idle X and callback behavior remain intact.
  **File:** `apps/web/components/task/session-tab-close-action.test.tsx`.
  **How:** retain the current idle-state assertion and run the focused component test.
- **What:** primary promotion is silent on success but still reports failures.
  **File:** `apps/web/hooks/domains/session/use-session-actions.test.ts`.
  **How:** assert successful `setPrimary` emits neither loading nor success toast, while a rejected
  request emits one error toast and the existing WebSocket payload/timeout remain unchanged.

## E2E Tests

- **Scenario:** the desktop X-confirm-delete flow still removes the session and tab.
  **File:** existing `apps/web/e2e/tests/session/session-tab-management.spec.ts` test
  `tab close button shows delete confirmation and removes session on confirm`.
  **What to verify:** the integrated deletion path remains healthy; the transient spinner shape is
  deterministic in the component test rather than timing-sensitive in Playwright.
- **Scenario:** mobile session deletion remains reachable through the native picker.
  **File:** existing `apps/web/e2e/tests/session/mobile-session-deletion.spec.ts`.
  **What to verify:** the remaining session stays reachable after deleting one row; no desktop tab
  markup is required on mobile.
- **Scenario:** promoting a secondary desktop session to primary updates its star without routine
  toast notifications.
  **File:** existing `apps/web/e2e/tests/session/session-tab-management.spec.ts`, primary-session
  persistence scenario.
  **What to verify:** the backend and tab star update, and neither `Set primary...` nor
  `Set primary successful` appears.

## Verification Results

Implementation complete. The focused component and hook tests pass 17/17. TypeScript typecheck,
i18n key/ratchet checks, scoped ESLint, Prettier, and `git diff --check` pass. The managed production
build E2E runs pass for desktop deletion (1/1), desktop primary promotion with no matching toast
(1/1), and mobile session-picker deletion (1/1). The i18n check reports only the repository's
existing advisory real-locale parity warnings; no new user-facing copy was added.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-session-tab-feedback](task-01-circular-spinner.md)

The source and component test share one visual contract, so they are not parallel-safe.

## Risks

- The replacement must retain the existing `role="status"` and parent button semantics so the
  pending state remains accessible while the visual shape changes.
- Copying the terminal spinner's exact classes avoids a new visual variant but leaves no shared
  component abstraction; extracting one would expand this narrowly scoped repair without changing
  user behavior.

## Out of scope

- Session deletion transport, confirmation dialogs, Dockview reconciliation, active-session
  handoff, and feedback for stop/resume/context-menu deletion actions.
- Mobile session-picker markup or behavior.
