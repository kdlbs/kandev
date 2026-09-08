---
spec: docs/specs/ui/requirements/sidebar-empty-task-alignment.md
created: 2026-08-13
status: complete
---

# Implementation Plan: Sidebar empty task alignment

## Overview

Add a stable slot marker to the shared task-switcher empty state, then override only that slot's horizontal padding inside the full-width desktop Tasks host. Extend the existing empty-sidebar E2E with a text-anchor geometry assertion and add a mobile preservation assertion for the separate phone composition.

## Confirmed root cause

`apps/web/components/app-sidebar/app-sidebar.tsx` gives the navigation `px-2`. The Tasks section title adds another `px-2`, so its text starts at x=16 in the rendered 1280px diagnostic viewport. `apps/web/components/app-sidebar/sections/tasks-section.tsx` intentionally uses `-mx-2` so the task scroll surface still spans the full navigation width, but `apps/web/components/task/task-switcher.tsx` gives the empty message only `px-3`. Its text therefore starts at x=12, four pixels left of the title.

Browser evidence from an isolated mock instance confirmed the mismatch: Tasks title x=16, empty-message text anchor x=12. At 393px the desktop app sidebar had `display: none` and no visible sidebar empty message.

---

## Frontend

### Shared task-switcher empty state

- `apps/web/components/task/task-switcher.tsx`: add `data-slot="task-switcher-empty-state"` to the existing empty-state element. Keep its base `px-3` class so standalone and mobile task-switcher hosts retain their current inset.

### Desktop app sidebar Tasks host

- `apps/web/components/app-sidebar/sections/tasks-section.tsx`: extend the existing full-width wrapper with `[&_[data-slot=task-switcher-empty-state]]:px-4`. Within its `-mx-2` geometry, 16px padding places the message at the same x-coordinate as the section title without shifting task rows, group headers, loading/error states, or the shared mobile surface.

No API, store, state, copy, or backend change is required.

## Mobile parity

- **Desktop outcome:** the empty message aligns with the Tasks title while the list remains full-width.
- **Mobile entry point and exemplar:** phone task navigation remains the existing `SessionTaskSwitcherSheet` in `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`, whose `p-2` list wrapper and shared `px-3` task content remain unchanged.
- **Presentation, hierarchy, scrolling, and touch behavior:** unchanged. The desktop AppSidebar remains hidden below `md`; the mobile drawer remains the scroll owner.
- **Coverage:** extend the existing `mobile-task-listing-display.spec.ts` phone scenario to assert the desktop app sidebar is hidden. The plan-phase rendered check already confirmed `display: none` at 393px; implementation reruns the mobile project against the fresh production build.

## Tests

- **What:** the empty message's left text anchor equals the Tasks title's left edge while the task sidebar remains transparent and full-width.
  **File:** `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`.
  **How:** extend the existing empty-list test. Read the Tasks title bounding box, compute the empty message text anchor from its box plus rendered left padding, and compare the x-coordinates with subpixel tolerance. Write and run this assertion before the host override; it must fail with the confirmed 12px versus 16px coordinates, then pass after the fix.
- **What:** phone rendering continues to use the mobile composition rather than the desktop sidebar.
  **File:** `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`.
  **How:** extend the existing phone Kanban scenario with a visibility assertion for `app-sidebar-layout`. This is preservation coverage; the desktop geometry assertion is the required RED regression.

No unit test is needed because the defect is a rendered alignment relationship with no domain logic.

## E2E Tests

- **Scenario:** GIVEN the expanded desktop app sidebar has no tasks, WHEN the Tasks section renders, THEN **No tasks yet.** and **Tasks** share the same left text anchor while the transparent list still spans the navigation width.
  **File:** `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`.
  **What to verify:** x-coordinate equality, transparent scroll background, and unchanged full-width bounds.
- **Scenario:** GIVEN a phone viewport, WHEN the task listing renders, THEN the desktop app sidebar remains hidden and mobile Kanban remains visible.
  **File:** `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`.
  **What to verify:** `mobile-kanban-layout` is visible and `app-sidebar-layout` is hidden.

## Verification Results

- **RED:** the focused Chromium empty-sidebar test discovered one test and failed on the new geometry assertion with a 4px mismatch (`received: 4`, tolerance `0.5`).
- **GREEN:** the same production-build Chromium command passed its one test; the focused mobile Chrome preservation command also passed its one test.
- **Rendered geometry:** an isolated 1280x800 mock instance measured the Tasks title at x=16 and the empty-message text at x=16 (delta 0). The corrected desktop state was captured in `apps/web/.pr-assets/sidebar-empty-task-alignment.png` for PR publication.
- **Static checks:** web typecheck, targeted ESLint, targeted Prettier, the i18n ratchet, and `git diff --check` passed.
- **Cleanup:** the named browser session and isolated backend/frontend processes were stopped after capture.

## Implementation Waves And Parallel Candidates

Wave 1 - sequential:

- [x] [task-01-align-empty-task-message](task-01-align-empty-task-message.md)

The marker, desktop host override, and geometry regression describe one rendered behavior and share the same verification pass.

## Risks

- A global `px-4` change in `TaskSwitcher` would also shift the mobile drawer's empty state. The host-scoped selector avoids that regression.
- Testing only element boxes would measure the full-width empty-state container rather than its text. The E2E must include rendered left padding when deriving the text anchor.

## Open Questions

None.
