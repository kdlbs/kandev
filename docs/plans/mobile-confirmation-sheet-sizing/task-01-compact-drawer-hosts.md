---
id: "01-compact-drawer-hosts"
title: "Compact hosted drawer geometry"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.5
  - AC-UI-MOBILE-CONFIRMATION-001.6
  - AC-UI-MOBILE-CONFIRMATION-001.7
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.4
  - AC-UI-MOBILE-CONFIRMATION-002.5
  - AC-UI-MOBILE-CONFIRMATION-002.6
  - AC-UI-MOBILE-CONFIRMATION-002.7
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.2
  - AC-UI-MOBILE-CONFIRMATION-003.5
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
---

# Task 01: Compact Hosted Drawer Geometry

## Summary

Make short confirmations content-sized inside existing bottom drawers. Preserve
the same modal and mounted origin, restoring the editor/list geometry and state
when the user cancels.

## In scope

- Add explicit drawer composition to the shared host, with scoped active sizing.
- Remove the hidden origin from layout without clamping its scroll containers.
- Adopt in Tasks, sidebar-filter, Threads and `MobilePickerSheet` owners.
- Extend the real phone regressions and preserve centered-dialog behavior.

## Out of scope

Provider filter conversion (02), mutations, global primitive changes, new copy,
new preferences, publishing and any change to main :9998.

## Acceptance

1. The English short Threads deletion fixture occupies less than 60% of the
   configured phone viewport, ends at the existing bottom inset, and has no
   more than 32px between description content and primary action. Long text
   instead uses a contained body scroller with title/actions visible.
2. Cancel/Back restores the same dialog identity, original editor height,
   unsaved values, selection, scroll and trigger focus. Tasks and both pickers
   exhibit the same behavior. Opening/cancelling never dispatches the action.
3. Centered dialogs and 768px+ fallback remain unchanged. Narrow, landscape,
   Portuguese/pseudo and reduced-motion checks retain usable 44px+ hit targets,
   safe areas and no document overflow.

## Verification

After an explicit implementation request, use `/tdd` and `/e2e`. Extend the
Threads saved-view deletion test first; run it against the current build and
observe failure on bounds/spacing, not setup or selectors. Then implement.
Replace the Tasks same-height assertion with compact-size plus restored-size
checks; do not remove its scroll, target, focus or modal-identity assertions.

Dependencies are installed in this worktree. In a fresh worktree, first run
`pnpm install --frozen-lockfile` from `apps/`. Run from `apps/web`:

```bash
pnpm run build:e2e
pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-view.spec.ts --grep 'confirms deletion'
pnpm exec vitest run components/confirmation/mobile-confirmation-host.test.tsx components/confirmation/mobile-action-confirmation.test.tsx components/confirmation/mobile-confirmation-content.test.tsx
pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-view.spec.ts tests/task/mobile-sidebar-views.spec.ts tests/task/mobile-action-confirmations.spec.ts tests/terminal/mobile-terminal-close.spec.ts tests/session/mobile-session-deletion.spec.ts
pnpm e2e:run --project mobile-chrome tests/settings/mobile-management-confirmations.spec.ts --grep 'workflow'
pnpm e2e:run --project chromium tests/task/sidebar-filter.spec.ts tests/task/threads-view.spec.ts --grep 'delete|deletion|tablet'
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/confirmation/mobile-confirmation-host.tsx components/confirmation/mobile-confirmation-content.tsx components/task/mobile/task-switcher-drawer.tsx components/task/mobile/mobile-picker-sheet.tsx components/task/sidebar-filter/sidebar-filter-popover.tsx components/threads/threads-view-controls.tsx
```

Rebuild after the implementation's final frontend edit before GREEN E2E runs.
Use the existing managed runner's single-worker limits. Inspect Threads and
Tasks phone screenshots; record actual bounds, not just a screenshot path.

## Files likely touched

- `apps/web/components/confirmation/mobile-confirmation-host.tsx`,
  `mobile-confirmation-content.tsx` if needed, corresponding tests and `AGENTS.md`.
- `apps/web/components/task/mobile/task-switcher-drawer.tsx`,
  `mobile-picker-sheet.tsx` and `task/sidebar-filter/sidebar-filter-popover.tsx`.
- `apps/web/components/threads/threads-view-controls.tsx`.
- The five main phone E2E files in the verification command; reuse the existing
  viewport/animation helpers instead of adding an artificial component route.
- The paired requirements/design and this work order for execution results.

## Dependencies

The original mobile confirmation implementation at `90c65b7664` is present.
No pending work-order dependency.

## Risks

Frozen origin dimensions must preserve nested scroll without exposing hidden
content to focus/accessibility. Primitive max-height variants can override
ordinary utility classes. Do not write competing transforms onto Vaul's root.

## Parallelism

`sequential`

## Inputs

The paired design's Hosted content, Mobile composition and Verification
sections; current standalone confirmation and picker shell; host lifecycle
tests; `apps/web/AGENTS.md` and scoped confirmation/Threads guidance.

## Results

Implemented on 2026-09-11 after the explicit implementation request.

- RED: the Threads fixture measured a 581.59px sheet against a 436.2px compact
  limit. The failure was rendered geometry, not visibility or fixture setup.
- GREEN: explicit drawer composition overrides tall host sizing; the hidden
  origin keeps measured dimensions outside layout. Tasks, sidebar filters,
  Threads and session/terminal picker hosts opt in. Centered dialogs do not.
- The Threads regression now uses a 393x640 viewport, an invalid unsubmitted
  column input and a nonzero editor scroll. Compact height/spacing, same modal,
  restored size/input/scroll/focus, and persisted deletion pass.
- The Tasks/list, sidebar-view, session and terminal geometry cases pass;
  standalone and hosted archive cases pass at 320x640 Portuguese/light,
  767x900 English/dark and 667x375 pseudo/light. Hosted cases use reduced motion
  and rotate an already open request; a child task forces scrolling long copy
  in landscape. Cancel never archives the parent.
- Host/content/request unit tests: 20 passed. Typecheck and changed-file lint
  passed. The centered workflow-removal/retry case passed. Four desktop
  saved-view deletion/eligibility regressions passed.
- Corrected an unrelated E2E timestamp assertion to inspect visible compact
  text separately from its intentionally verbose screen-reader text.
- A legacy Tasks ellipsis CSS rule hides its entry at coarse-pointer widths
  640-767px. This pre-existing, general-menu issue is outside this geometry
  correction; the wide hosted tests cover portrait entry followed by rotation,
  not direct entry at those widths.
- An interrupted E2E run left this worktree's orphan backend on 18081, causing
  a later fixture workspace-404 failure. Its exact process was identified and
  stopped; the clean rerun passed. Main 9998 and seeded 48429 were not stopped.
- Final screenshot inspection exposed a separate scroll-containment regression:
  the Threads root retained a 274px scroll offset while its measured box still
  passed sizing assertions. A traced RED run reproduced a completely clipped
  heading. Drawer composition now uses `overflow: clip` even in the origin
  step, so only the inner list/body scrolls. Initial Cancel focus also uses
  `preventScroll`, covered by a RED/GREEN unit assertion. Browser checks now
  require zero root scroll and full heading/action intersection with the
  viewport, including after capturing the edited-input case.

The final combined regression pass and fresh captures after work order 02 are
recorded in the parent plan.
