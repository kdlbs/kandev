---
id: "01-transform-free-dialog"
title: "Restore sharp text in the Quick Chat modal"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-CHAT-ELEVATION-001
acceptance_criteria:
  - AC-UI-QUICK-CHAT-ELEVATION-001.1
  - AC-UI-QUICK-CHAT-ELEVATION-001.2
  - AC-UI-QUICK-CHAT-ELEVATION-001.4
  - AC-UI-QUICK-CHAT-ELEVATION-001.5
  - AC-UI-QUICK-CHAT-ELEVATION-001.6
  - AC-UI-QUICK-CHAT-ELEVATION-001.7
  - AC-UI-QUICK-CHAT-ELEVATION-001.8
system_design:
  - ../../specs/ui/system-design/quick-chat-elevation.md
---

# Task 01. Restore sharp text in the Quick Chat modal

## Summary

Remove the centering translation and scale animation from the Quick Chat
panel in WebKit. The fix covers both chat and Quick Terminal tabs.

## In scope

- Opt in with a local class on `QuickChatModalShell`'s `DialogContent`.
- Center the fixed panel with CSS insets and automatic margins under the
  existing WebKit marker. Use the existing opacity-only keyframes.
- Cover the desktop and phone geometry through their normal launchers.

## Out of scope

- Global browser detection and shared dialog defaults.
- Changes to the standalone Quick Chat picker and Create Task.
- Terminal rendering, settings persistence, and session lifecycle.

## Acceptance

- In WebKit, Quick Chat and Quick Terminal use the same panel. It opens and
  closes with opacity-only motion and has `translate: none` and
  `transform: none`. Both tabs pass the native Orion text comparison.
- At `sm` and above, the panel stays centered above the blurred backdrop and
  retains its saved width, viewport cap, 85dvh height, and resize controls.
  Chromium keeps the scale-and-fade animation.
- Below `sm`, the phone panel fills the viewport with safe-area padding and
  no document horizontal overflow. Its content and close control are usable.

## ASCII UI preview

[Full views and acceptance mapping](plan.md#ascii-ui-preview).

```text
UI-01. Desktop, open from the sidebar or shortcut
     +-------------------------+
     | Chat  Terminal  +     x |
    <| Sharp chat or shell     |>
     | Composer or shell input |
     +-------------------------+
          Blurred page

UI-02. Phone, open from the Home app menu
+-------------------------------+
| Chat  Terminal  +           x |
| Transcript or shell           |
| Composer or terminal keys     |
+-------------------------------+
```

The fixed tab strip and close control stay visible above the active content.
The phone panel uses the full viewport and clears the safe areas. The
desktop panel uses symmetric margins and side resize handles.

## Test sequence

1. Add `Quick Chat WebKit rendering` tests in
   `quick-chat-webkit-rendering.spec.ts`. Set a 1200 by 901 CSS pixel
   viewport and seed the saved width to 901 CSS pixels. Set the WebKit marker
   after navigation and before opening the dialog. Use
   `openQuickChatSetup(page, false)` with the sidebar action enabled.
2. Reuse `readDialogRenderingMetrics` to check final geometry and stacking.
   Assert `translate: none`, `transform: none`, and the WebKit animation
   names. Switch to a terminal tab via the add menu and check the same panel.
   Check the width and center after a side-handle drag. Wait for the saved
   width and the rendered width to agree before reading the final bounds.
3. Add the `keeps the WebKit Quick Chat dialog full-screen` scenario in
   `mobile-quick-chat-entry.spec.ts`. Open from the Home menu and close via
   touch. Check viewport bounds, opacity-only motion, no CSS translation,
   and document overflow.
   At 639 and 640 CSS pixels, check where the desktop sizing takes effect.
4. Run the new tests against the baseline. Check that the desktop test fails
   on `enter` and the percent translation. Check that the phone test fails
   on the WebKit motion rules. Add the local CSS and rerun the focused tests.
5. In native Orion on macOS at 100% zoom, compare chat and terminal text before
   and after at the same viewport and display. Repeat after a panel resize.
   Record the Orion version, display scale, viewport, and visual result.

## Verification

Run from the repository root. The first E2E command builds the app and
fixtures. The phone test uses those artifacts.

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web e2e:run --host --project chromium \
  tests/chat/quick-chat-webkit-rendering.spec.ts tests/chat/quick-chat.spec.ts \
  -- --grep 'Quick Chat WebKit rendering|resizes from either edge|stronger elevation backdrop'
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome \
  tests/chat/mobile-quick-chat-entry.spec.ts \
  -- --grep 'opens from the home menu and closes with the touch control|keeps the WebKit Quick Chat dialog full-screen'
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/chat/quick-chat-webkit-rendering.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`

## Dependencies

None. The current app already sets the WebKit marker and defines the dialog
fade keyframes.

## Risks

- Remove important declarations only from the panel position and translation
  utilities. Check that the CSS keeps the existing height and width limits.
- Read geometry after finite animations settle. Test dimensions and centering
  with a one CSS pixel tolerance.

## Parallelism

`sequential`

## Inputs

- [Quick Chat elevation requirement](../../specs/ui/requirements/quick-chat-elevation.md)
- [WebKit panel design](../../specs/ui/system-design/quick-chat-elevation.md#webkit-panel-rendering)
- [Create Task rendering precedent](../webkit-task-dialog-rendering/task-02-webkit-dialog-rendering.md)
- [Dialog rendering metrics](../../../apps/web/e2e/helpers/dialog-webkit-metrics.ts)

## Results

- RED: Before the CSS change, the desktop regression received the shared
  `enter` animation instead of the WebKit opacity animation; the phone
  regression observed the same shared animation.
- GREEN: The desktop E2E run passed 4 tests, including Chromium control,
  Quick Chat and Quick Terminal rendering, saved-width resize, backdrop, and
  both-edge resizing. The mobile E2E run passed 2 tests, including phone entry,
  touch dismissal, no horizontal overflow, and the 639/640 CSS-pixel breakpoint.
- Checks: The E2E runner built backend and Vite assets. Web typecheck, targeted
  Oxlint, specification validation, specification lint, and `git diff --check`
  passed.
- Native Orion check: Not run; this Linux environment does not provide Orion on
  macOS. Compare both tab labels and active text at 100% zoom after resize
  during macOS verification.
- Review follow-up: WebKit auto margins now apply at every viewport, the exit
  animation listener is installed before Escape with a bounded assertion, and
  the Chromium control verifies the runtime `other` marker.
- Delivery remains in progress until native Orion comparison results are recorded.
