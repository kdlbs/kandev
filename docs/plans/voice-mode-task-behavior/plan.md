---
spec: docs/specs/ui/requirements/voice-mode-task-behavior.md
created: 2026-08-11
status: implemented
---

# Implementation Plan: Voice Mode In Task Behavior

## Overview

Reuse the existing `VoiceModeSettings` section inside Task Behavior, then make Task Behavior the
single navigation and discovery owner for voice preferences. Remove the former route without a
compatibility redirect. Update focused component, catalog, route, desktop E2E, and mobile E2E
coverage before capturing both affected viewports and publishing those screenshots on the PR.

## Frontend

### Settings composition and navigation

- Update `apps/web/components/settings/task-behavior-settings.tsx` to render
  `VoiceModeSettings` after the existing message-queue section, separated consistently from the
  preceding content. Reuse the component and its `VoiceDraftProvider`; do not duplicate its save
  or capability logic.
- Remove the Voice Mode row and unused microphone import from
  `apps/web/components/app-sidebar/sections/settings/settings-menu-sections.ts`.
- Remove `/settings/voice-mode` from `apps/web/src/settings-routes.tsx` so Task Behavior is the
  only route that renders Voice Mode.
- Remove obsolete standalone breadcrumb ownership and compatibility constants when no remaining
  caller consumes them.

### Settings discovery

- Move the Voice Mode page and control definitions from
  `apps/web/lib/settings-discovery/catalog/standalone.ts` into
  `apps/web/lib/settings-discovery/catalog/preferences.ts` under the Task Behavior page.
- Retarget every voice result to `TASK_BEHAVIOR_SETTINGS_HREF`, preserve the current stable target
  IDs used by `VoiceModeSettings`, and make Task Behavior the parent page so search breadcrumbs and
  grouping describe the canonical location.
- Remove `VOICE_MODE_SETTINGS_HREF` and update catalog tests after all voice discovery entries use
  `TASK_BEHAVIOR_SETTINGS_HREF`.

### Mobile design contract

- **Outcome and entry point:** desktop and mobile users enter through the existing Task Behavior
  menu row and reach the same voice controls on the merged page.
- **Exemplar:** reuse the shipped Task Behavior mobile surface covered by
  `mobile-message-queue-settings.spec.ts`; it contributes direct navigation, one settings scroll
  owner, shared floating-save behavior, and narrow-width overflow expectations.
- **Hierarchy and action:** task actions remain first, message queue second, and Voice Mode third;
  voice cards retain their current order and touch behavior. The shared **Save changes** action
  remains the only persistence action.
- **Surface rationale:** these are infrequent preferences with linear content, so extending the
  existing direct-navigation Settings page is more appropriate than a drawer or a new nested route.
- **Shared versus responsive behavior:** component state, draft coordination, search targeting, and
  controls are shared. Mobile uses the existing settings shell without mounting a compressed
  desktop-only composition. The settings content remains the single scroll owner.
- **Proof:** desktop and Pixel 5 E2E navigate through Task Behavior, verify Voice Mode controls and
  the absence of the menu row, and capture the settled merged page. Mobile additionally proves
  scrolling and no document horizontal overflow.

## Tests

- **What:** Task Behavior owns the voice section while the main menu does not expose a Voice Mode
  page row. **Files:** `apps/web/components/settings/voice-mode-settings.test.tsx`,
  `apps/web/components/app-sidebar/sections/settings/settings-tree-render.test.tsx`, and
  `apps/web/components/app-sidebar/sections/settings/settings-nav-copy.test.ts` if its page-copy
  contract changes. **How:** component/render tests assert composition, control availability, and
  menu absence without changing voice draft semantics.
- **What:** the former URL is absent and all voice discovery entries resolve to Task Behavior with
  stable target IDs. **Files:** `apps/web/src/settings-routes.test.ts` and
  `apps/web/lib/settings-discovery/catalog.test.ts`. **How:** focused route-resolution and catalog
  ownership tests.

## E2E Tests

- **Scenario:** desktop Settings navigation contains Task Behavior but no Voice Mode row, and the
  merged page exposes the existing voice controls. **File:** extend an existing focused settings
  spec or add `apps/web/e2e/tests/settings/task-behavior-voice-mode.spec.ts`.
  **What to verify:** menu information architecture, section order, voice control availability,
  removed legacy route, and a saved voice draft where focused setup permits.
- **Scenario:** Pixel 5 navigation reaches the merged page and all voice controls remain reachable.
  **File:** migrate `apps/web/e2e/tests/chat/mobile-voice-mode.spec.ts` to the Task Behavior entry
  point. **What to verify:** menu-row absence, section/control visibility, settings-container
  scrolling, and no document horizontal overflow.
- Use the existing `prCapture` fixture or a disposable capture spec to create fresh desktop and
  mobile PNGs in `apps/web/.pr-assets` during the same managed E2E invocation when possible.
  Validate and compress both assets before PR creation, then publish them through the PR skill's
  orphan media branch flow and append a `## Screenshots` section to the PR body.

## Verification Results

Implemented. Focused unit coverage passes 95 tests across the Voice Mode component, settings menu,
discovery catalog, and route registry. Typecheck, i18n checks, i18n ratchet, formatting, public-doc
tests, desktop E2E, and mobile E2E all pass. Desktop and Pixel 5 screenshots are captured in the
ignored `.pr-assets` directory and published through the PR media branch. The ready PR is
[#2534](https://github.com/kdlbs/kandev/pull/2534); media is on
`media/pr-2534-screenshots` at commit `d8e82197519c4ef6b2bb311eb7ee8e7224d166b1`.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Consolidate Voice Mode settings](task-01-consolidate-settings.md)

Wave 2:

- [x] [Task 02: Verify responsive UI and publish PR](task-02-e2e-screenshots-pr.md) (depends on
  Task 01)

The tasks are sequential because the E2E routes, screenshots, and PR must exercise the completed
navigation and discovery changes.

## Risks

- Moving discovery definitions without preserving target IDs can make search open the right page
  but fail to scroll to the selected control.
- Rendering two independent voice draft providers on one route would create competing save
  contributors; Task Behavior must mount the existing section exactly once.
- The longer merged page can expose scroll-clearance regressions around the floating Save action,
  especially on mobile, so E2E must exercise the bottom controls rather than only assert headings.
- Removing the legacy route is intentionally breaking; tests must ensure no discovery or menu
  entry still produces the dead URL.
- PR screenshots must come from isolated synthetic E2E state and fresh affected-viewport captures;
  they must not include local user data or be committed to the feature branch.
