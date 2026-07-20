---
name: mobile-parity
description: Ensures Kandev UI work uses native mobile interaction patterns instead of compressed desktop adaptations while preserving desktop/mobile capability parity, responsive behavior, and mobile Playwright E2E coverage. Use when implementing, planning, reviewing, or testing any new feature, page, component, workflow, form, dialog, sidebar, navigation, dashboard, or visual UI change; if work touches frontend or user-facing UI, this skill must run even when user mentions only desktop or says "new feature".
kandev:
  system: true
  version: "0.51.0"
  default_for_roles: []
---

# Mobile Parity

Use this skill before planning or changing UI. Goal: desktop and mobile deliver the same user value, while each viewport gets an intentional composition and tests prove the mobile path.

Before proposing a mobile design, read [Kandev Mobile UI Language](references/kandev-mobile-ui-language.md) and inspect the closest shipped mobile surface. Responsive CSS alone does not establish mobile parity.

## When It Applies

Apply when task changes user-facing UI:

- new or changed pages, routes, components, forms, dialogs, drawers, navigation, dashboards, tables, cards, toolbars, editors, settings, onboarding, or visual states
- new frontend behavior attached to backend/API work
- bug fixes where layout, touch behavior, scrolling, or viewport width can affect success

If task has no UI surface, say why this skill does not apply and continue.

## Mobile Design Contract

When a task changes composition, navigation, overlays, touch behavior, scrolling, or breakpoint behavior, state these choices in the working plan or task notes. Keep them brief; do not create a separate document unless the task already uses a spec or committed plan. For copy, icon, color, or content-only styling inside an unchanged surface, identify the nearest mobile exemplar and the rendered mobile check instead of forcing the full contract.

- desktop user outcome and mobile entry point
- nearest shipped mobile exemplar and which interaction/geometry it contributes
- mobile information hierarchy and primary action
- presentation choice: inline, inset bottom drawer, full-height surface, or direct navigation
- surface rationale: why task frequency and content depth make that choice preferable to the alternatives
- single scroll owner, dynamic viewport behavior, safe-area handling, and touch targets
- shared state, view-model, filtering/selection, and business logic versus mobile-specific presentation
- mobile Playwright scenario proving the same user value

## Workflow

1. Map affected surfaces.
   - Identify every page, modal, menu, tab, empty state, loading state, and error state the feature touches.
   - Check where desktop layout assumptions can fail: fixed widths, hover-only controls, sidebars, tables, dense toolbars, keyboard shortcuts, overflow, and absolute positioning.
   - Use `rg` to find the nearest existing mobile component and the route's current `useResponsiveBreakpoint` branch. Name the closest curated exemplar from the reference and state which parts are reused.
   - Treat live code as the source of truth for APIs, state, and current behavior. Treat this skill's curated guide and exemplar list as the desired mobile interaction baseline; nearby legacy surfaces that deviate from it need justification, not automatic reuse.

2. Design desktop and mobile behavior together.
   - Preserve capability, data, and state semantics; do not require identical markup or navigation.
   - Apply the surface decision guide in the mobile UI language reference. Prefer a focused, one-dimensional phone flow over stacked desktop panes or two-axis scrolling.
   - Explicitly distinguish temporary choices that belong in a drawer from primary or dense content that deserves direct navigation or a full-height surface.
   - When a card or row has an obvious primary destination and no competing selection, drag, or inline-control behavior, make its body tap perform that action. Otherwise expose an explicit touch control. Put secondary actions behind a visible target, never hover, right-click, or undiscoverable long press.
   - Define mobile navigation, hierarchy, scroll owner, touch targets, truncation, empty/error states, and responsive fallback behavior before coding.

3. Implement responsive UI.
   - Reuse domain hooks, state, view-model derivation, filtering/selection, and action handlers across viewports; keep responsive wrappers focused on presentation. Branch composition when the desktop interaction model depends on width or a fine pointer. Do not mount a heavyweight desktop workbench and merely hide or squeeze it on phones.
   - Use `useResponsiveBreakpoint` and existing `@kandev/ui` or mobile primitives. Reuse current Dropdown/ContextMenu primitives for contextual actions; use `Drawer` or an existing picker shell for structured phone navigation and choices. Use `useTouchDrawer` when a hover disclosure needs a coarse-pointer alternative.
   - Keep touch targets large enough for touch use, generally at least 44px in the active dimension.
   - Use dynamic viewport units and an explicit internal scroll region for viewport-bound/full-height or potentially overflowing surfaces. Ensure bottom-fixed controls and tall drawers clear safe-area insets; short drawers can retain the shared primitive's intrinsic sizing. Keep document-level horizontal overflow at zero.
   - If mobile substitutes an unsupported desktop view, derive an effective mobile view without overwriting the user's saved desktop preference.
   - Use semantic controls, visible labels or accessible names, focus return, and existing design-system components.
   - Avoid hiding required functionality on mobile unless there is a clear alternate path.

4. Add E2E coverage.
   - Add or update Playwright tests for the feature's happy path on desktop if missing.
   - Add mobile Playwright coverage for the same user value, using existing mobile projects/devices when configured.
   - In this repo, name mobile test files `mobile-*.spec.ts` so the `mobile-chrome` Playwright project picks them up automatically.
   - Cover the actual mobile composition: drawer or full-height surface, visible overflow action, focused navigation, direct route, or bottom control.
   - For overlay and dense-navigation changes, assert viewport containment, internal scrolling, and the absence of document horizontal overflow where those properties are part of the regression.
   - When a touch-only control is replaced or hidden, run `rg` across mobile E2E tests for the removed control. Replace every affected interaction with the intended gesture or alternate control, then run those tests together.

5. Verify visually and behaviorally.
   - Run the narrowest relevant viewport locally or with screenshots when possible.
   - Even small user-facing UI tweaks need at least focused rendered verification when feasible: dev-server/browser check, Playwright screenshot, or targeted E2E. If not run, report the exact reason.
   - Check phone ergonomics, not only responsive fit: entry point is discoverable, primary action is thumb-reachable, hierarchy is understandable, sheet shape matches nearby surfaces, and back/dismiss behavior is predictable.
   - Check that text does not overlap, controls remain clickable, focus/keyboard flows still work, safe-area content is unobstructed, and no unintended horizontal scroll appears.
   - Run the focused Playwright tests. If full E2E cannot run, report the command and blocker.
   - E2E runs against the production Vite build served by the Go backend, not a dev server, so rebuild after frontend changes: `make build-web` (and `make build-backend` for Go), or use `make test-e2e` which rebuilds both. Skipping this silently tests stale code. See `/e2e`.

## Mobile E2E Expectations

Every UI feature should end with one of these:

- mobile Playwright test added or updated
- existing mobile Playwright test explicitly identified as covering the changed behavior
- written justification for no mobile test, limited to impossible-to-test infrastructure gaps

For frontend changes that are purely state/data normalization inside an existing component and do not alter rendered layout, touch behavior, scrolling, navigation, or viewport-dependent interaction, targeted unit/component tests plus an explicit note can satisfy mobile parity. New mobile Playwright coverage is not required for that narrow case.

Good mobile tests assert real user outcomes, not only visibility. Prefer:

- open feature from mobile navigation and complete primary action
- use drawer/menu/sheet variant of desktop controls
- submit form and verify result
- handle empty/error/loading state on narrow viewport
- confirm no required action is desktop-only

## Playwright Routing

Create `mobile-*.spec.ts` files and let the `mobile-chrome` project apply its configured Pixel 5 device; do not add per-test device overrides. Follow `/e2e` for fixtures, page objects, selectors, build requirements, and local reproduction. Mobile parity owns which interaction and geometry contracts need proof; `/e2e` owns test mechanics.

## Done Checklist

- Desktop path still works.
- Structural/touch changes include a mobile design contract naming entry point, nearest shipped exemplar, hierarchy, surface, scroll owner, and primary action.
- For those changes, surface rationale explains why the chosen composition fits task frequency and content depth.
- Content-only styling changes identify the nearest mobile exemplar and focused rendered mobile check instead.
- Mobile path follows a shipped Kandev pattern or explains why a new pattern is needed.
- Mobile composition is intentional, not desktop UI stacked, squeezed, or hidden with CSS.
- Required controls are reachable by touch.
- No required workflow depends on hover, wide viewport, or hidden desktop-only UI.
- Viewport-bound/full-height or potentially overflowing surfaces use dynamic viewport sizing, safe-area clearance, and internal scrolling.
- Responsive fallbacks do not overwrite saved desktop preferences.
- Mobile E2E tests no longer invoke touch controls that the change replaced or hid.
- Mobile Playwright coverage exists or absence is justified.
- Focused rendered/visual verification was run for UI tweaks, or exact "not run" reason is reported.
- Focused tests were run, or exact blocker is reported.
