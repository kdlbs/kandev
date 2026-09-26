---
spec: docs/specs/ui/requirements/mobile-quick-chat-topbar.md
created: 2026-08-18
status: complete
---

# Implementation Plan: Mobile Topbar Action Strip

## Overview

The phone header gives Quick Terminal and Quick Chat a 44px box. Search and menu use the shared
32px icon size. The action container also refuses to shrink, so metrics and plugin actions can push
the header past the viewport. This repair normalizes the phone controls and gives the middle actions
one contained horizontal scroll owner with directional fades.

## Confirmed root cause

- `apps/web/components/kanban/kanban-header-mobile.tsx` adds `!size-11` to the terminal and chat
  buttons. The shared `icon-lg` variant used by search and menu is 32px.
- `apps/web/components/page-topbar.tsx` renders the full action cluster with `shrink-0`. The mobile
  header provides no nested overflow owner for optional metrics or plugin contributions.
- `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx` provides the nearest shipped fade
  treatment. `apps/web/components/task/task-sidebar-scroll-area.tsx` provides the resize and content
  observation pattern for conditional scroll cues.
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts` records the regression as expected
  behavior by requiring the two oversized header launchers to be at least 44px.

## Frontend

### Scrollable phone action strip

- Add `apps/web/components/kanban/mobile-topbar-action-strip.tsx`. It owns the horizontal viewport,
  content row, hidden scrollbar, and left and right fades. When the content fits, the row aligns
  its actions against the fixed menu; overflow retains the same horizontal scroll order.
- Observe both the viewport and content row with `ResizeObserver`. Recalculate the fades after
  content changes, viewport changes, and horizontal scrolling.
- Show each fade only while content remains hidden in that direction. Use the gradient treatment
  from the mobile chat-input controls.
- Update `apps/web/components/kanban/kanban-header-mobile.tsx` so `PageTopbar` lets its actions use
  the remaining width. Keep the Kandev link outside the strip and keep the menu button outside it.
- Put the optional non-Home context, plugin actions, metrics, terminal, chat, and search inside the
  strip. Preserve their current order. Remove the 44px overrides from terminal and chat.
- Keep the current labels, launch handlers, search state, status indicator, and menu behavior.

### Native, metric, and plugin icon sizing

- Update `apps/web/components/system-metrics/topbar-metrics.tsx` so topbar metric icons inherit the
  same 16px size as native header icons. Keep the metrics container at 32px high.
- Add a mobile presentation option to
  `apps/web/components/kanban/main-top-bar-plugin-actions.tsx`. The mobile wrapper normalizes
  `host.ui.Button` contributions to a 32px box and their SVG icons to 16px.
- Keep Quick Terminal and Quick Chat as 32px visual buttons inside 44px interaction wrappers. The
  wrappers keep the strip item's width at 32px while preserving a real touch target.
- Keep desktop plugin contribution sizing unchanged. Keep existing plugin slot fields and lifecycle
  compatible; the mobile presentation adds the `presentation` value described by the public SDK.
- Export `MainTopBarSlotProps` from `@kandev/plugin-sdk` and assert the host binding remains an exact
  consumer of that public type.

### Plugin contract documentation

- Update `apps/web/lib/plugins/types.ts`, `docs/plans/plugins/PLUGIN-API.md`, and
  `docs/public/plugins-authoring.md` with the mobile `main-top-bar` sizing and overflow contract.
- State that phone contributions join the middle scroll strip. Host UI icon buttons use the native
  32px box and 16px icon.
- This public page is a reference document. No new page or navigation entry is required.

## Mobile design contract

- **Desktop outcome:** desktop Home, Kanban, and Tasks headers keep their current layout and sizes.
- **Mobile entry point:** the existing Home and Tasks topbar remains the direct entry point.
- **Nearest exemplars:** the mobile chat-input toolbar supplies the fade. The task sidebar supplies
  conditional cue measurement after content and viewport changes.
- **Hierarchy:** Kandev is fixed on the left. Menu is fixed on the right. Page context and workspace
  actions scroll between them, align against the menu when they fit, and retain their current order.
- **Presentation:** the actions stay inline because they are frequent, shallow controls. A drawer
  hides direct actions and adds an extra interaction.
- **Scroll owner:** only the middle strip scrolls horizontally. The document and board do not gain
  horizontal overflow.
- **Touch and state:** the visible boxes match the existing compact search and menu chrome. Launch,
  search, metrics, plugin, and menu state remain shared with wider layouts. Quick Terminal and Quick
  Chat keep a 44px coarse-pointer hit area around their 32px visible boxes.
- **Mobile proof:** Pixel 5 coverage enables metrics, installs the real plugin fixture, and proves
  control equality, fixed edges, directional fades, contained scrolling, and action reachability.

## Tests

- **What:** native mobile controls use one visual size and retain their order and handlers.
  **File:** `apps/web/components/kanban/kanban-header-mobile.test.tsx`.
  **How:** render the real header actions through the existing mocks. Assert shared size classes,
  the fixed-menu boundary, the title's scroll ownership, action order, and launcher callbacks.
- **What:** fades reflect the available scroll direction and react to size changes.
  **File:** `apps/web/components/kanban/mobile-topbar-action-strip.test.tsx`.
  **How:** use controlled `clientWidth`, `scrollWidth`, and `scrollLeft` values with a mock
  `ResizeObserver`. Cover fitting content, initial overflow, middle scroll, and end scroll.
- **What:** mobile plugin and metric wrappers normalize icon geometry without changing desktop.
  **Files:** `apps/web/components/kanban/main-top-bar-plugin-actions.test.tsx` and
  `apps/web/components/system-metrics/topbar-metrics.test.tsx`.
  **How:** render host UI plugin buttons and metric icons. Assert the mobile wrapper contract and
  unchanged desktop classes.

## E2E Tests

- **Scenario:** GIVEN metrics and a real plugin contribution that overflow the phone header, WHEN
  Home renders, THEN Kandev and menu stay fixed while the middle strip scrolls with correct fades.
  **File:** `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`.
  **What to verify:** equal native button boxes, 32px metrics height, 16px metric icons, actual
  44px terminal/chat hit areas, initial and final fade states, successful search or launcher
  activation after scrolling, and no document horizontal overflow. Restore user settings and
  uninstall the fixture in `afterEach`.
- **Scenario:** GIVEN the existing mobile terminal flow, WHEN the launcher renders, THEN its visible
  box matches search, chat, and menu instead of the old 44px expectation.
  **File:** `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`.
  **What to verify:** relational header geometry while retaining the existing dialog, menu-row,
  terminal containment, scrolling, dismissal, and focus-return assertions.

## Verification Results

- Focused Vitest: 5 files and 20 tests passed after the hit-target review fix.
- Plugin SDK typecheck: passed.
- Focused ESLint: passed with no errors or warnings on the changed frontend files.
- Web typecheck: passed.
- Public documentation tests: 61 passed, 0 failed; validator covered 41 published pages.
- Managed Pixel 5 E2E: 1 test passed for the real plugin and metrics overflow scenario, including
  actual 44px terminal/chat hit-target checks when each action is in view.
- Managed Pixel 5 quick-terminal regression: 1 test passed with relational header geometry.
- Follow-up alignment fix: fitting action rows use right justification against the fixed menu; the
  focused action-strip suite passed 3 tests and focused ESLint passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Build the mobile action strip](task-01-mobile-action-strip.md)

Wave 2:

- [x] [Task 02: Prove mobile overflow behavior](task-02-mobile-overflow-e2e.md)

The tasks run sequentially. The E2E task depends on the component and contract changes.

## Risks

- Plugin components can render arbitrary markup. Host-enforced normalization applies to
  `host.ui.Button` and SVG contributions, which are the documented plugin path.
- Metrics and plugin contributions can appear after first paint. The observer must measure both the
  viewport and content row so the fades do not become stale.
- The fixed menu status dot must remain visible and clickable outside the fading scroll viewport.
