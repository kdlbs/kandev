---
spec: docs/specs/agents/requirements/settings-profile-layout.md
created: 2026-08-13
status: done
---

# Implementation Plan: Inline agent profile actions

## Overview

Add a full-desktop presentation for profile-row Duplicate and Delete actions
while retaining the existing overflow menu below the full-desktop breakpoint.
The same action handlers and delete confirmation remain shared across both
presentations. The work is frontend-only and does not change the profile API,
store contract, or translated copy.

## Frontend

### Profile row action presentation

- `apps/web/components/settings/agents/agent-profiles-section.tsx`
  - Use the canonical `useResponsiveBreakpoint` hook and its
    `isFullDesktop` value, which maps to viewport widths of at least 1024px.
  - Render compact icon-only Duplicate and Delete buttons inline on full
    desktop widths, retaining translated accessible names and showing the same
    translated action names in hover/focus tooltips.
  - Keep the existing three-dots menu as the only action presentation below
    that boundary, including compact desktop, tablet, and mobile.
  - Reuse `useProfileDuplicate`, `setConfirmOpen`, and
    `AgentProfileDeleteConfirmDialog`; do not duplicate mutation or store
    synchronization logic.
  - Keep the absolute profile link underneath the action cluster and preserve
    stable test IDs for both action presentations.

## Tests

- **What:** Profile rows render the correct action presentation at full desktop
  and below the breakpoint, while Duplicate and Delete retain their existing
  wiring.
  **File:** `apps/web/components/settings/agents/agent-profiles-section.test.tsx`.
  **How:** Mock the canonical responsive hook for full desktop and compact
  layouts. Assert inline button visibility, overflow-menu visibility, and the
  existing duplicate/delete behavior through the shared row controls.

- **What:** Existing profile list behavior remains intact after the action
  presentation split.
  **File:** `apps/web/components/settings/agents/agent-profiles-section.test.tsx`.
  **How:** Keep the current store read-after-write deletion and duplicate
  coverage, updating only selectors needed for the two responsive branches.

## E2E Tests

- **Scenario:** A full desktop profile row exposes Duplicate and Delete inline
  and the user can duplicate the seeded profile without opening a menu.
  **File:** `apps/web/e2e/tests/settings/agent-profile-duplicate.spec.ts`.
  **What to verify:** Assert the inline controls and absence of the overflow
  trigger, verify both tooltips on hover, activate Duplicate, and verify the
  copied row and model.

- **Scenario:** A 900px tablet-width profile row keeps both actions in the
  overflow menu and does not overflow its viewport.
  **File:** `apps/web/e2e/tests/settings/agent-profile-duplicate.spec.ts`.
  **What to verify:** Use the existing `tabletTestPage` fixture, assert inline
  controls are absent, open the profile-actions menu, and verify both menu
  actions are reachable with no document-level horizontal overflow.

- **Scenario:** A Pixel 5 profile row keeps the overflow-menu path and retains
  the same duplicate outcome.
  **File:** `apps/web/e2e/tests/settings/mobile-agent-profile-duplicate.spec.ts`.
  **What to verify:** Keep the touch-based menu interaction, assert the inline
  controls are absent, and retain the existing 44px hitbox and overflow checks.

## Mobile design contract

- Desktop outcome: profile users can duplicate or delete directly from a full
  desktop row without an extra menu click.
- Mobile and tablet entry point: the existing visible three-dots trigger opens
  the profile action menu; no new drawer or route is needed because these are
  short contextual actions and the existing Radix menu has the repository's
  mobile bottom-sheet treatment.
- Nearest shipped exemplar: the current `ProfileRowActions` dropdown and the
  existing mobile agent-profile duplicate flow. Reuse its trigger, menu,
  touch-sized hitbox, and action handlers.
- Mobile hierarchy: profile identity and metadata remain primary, with the
  overflow trigger as the only row action control below the full-desktop
  boundary.
- Scroll owner and geometry: preserve the existing settings scroll owner; the
  row action cluster must remain contained at 900px and phone widths, with no
  document-level horizontal overflow.
- Shared state and logic: both presentations call the same duplicate hook and
  delete confirmation callback, so only visual composition changes by viewport.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-profile-row-actions](task-01-profile-row-actions.md)

Wave 2:

- [x] [task-02-profile-actions-e2e](task-02-profile-actions-e2e.md)

The tasks are sequential because the E2E selectors and responsive assertions
depend on the component markup from task 01.

## Verification Results

Passed.

- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/web && pnpm test -- components/settings/agents/agent-profiles-section.test.tsx` passed (9 tests).
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm exec eslint components/settings/agents/agent-profiles-section.tsx components/settings/agents/agent-profiles-section.test.tsx` passed with no errors or warnings.
- `cd apps/web && pnpm run i18n:check` passed with the repository's existing advisory catalog warnings; no locale keys were added.
- `cd apps/web && pnpm run i18n:ratchet` passed with 0 new-code violations and an intact guard allowlist.
- `cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-duplicate.spec.ts` passed (3 tests: full desktop inline actions, 900px tablet overflow actions, and profile-page duplicate flow).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-duplicate.spec.ts` passed (1 test).
- The final desktop/tablet selector assertion rerun, `cd apps/web && pnpm e2e:run --no-build --project chromium tests/settings/agent-profile-duplicate.spec.ts`, passed (3 tests).
- Both full managed E2E runs rebuilt the backend and pseudo-locale Vite assets; the final no-build rerun used those production artifacts. All runs cleaned their isolated E2E artifact directories. No failure artifacts remain.
- Three fresh synthetic PR screenshots were captured, visually inspected, compressed, and validated through `.pr-assets/manifest.json` for full desktop, tablet, and mobile overflow-menu states. The capture-only spec was removed afterward.
- The icon-only refinement was verified with the focused component suite (9
  tests), managed desktop E2E (3 tests), managed mobile E2E (1 test), and a
  fresh full-desktop screenshot. The screenshot shows compact 28px action
  buttons centered in the profile row; the old visible labels are absent while
  translated accessible names remain.
- The tooltip refinement was verified with focus assertions in the component
  suite and hover assertions in the desktop E2E flow. Both tooltips reuse the
  existing translated Duplicate and Delete labels; the mobile overflow path is
  unchanged.
