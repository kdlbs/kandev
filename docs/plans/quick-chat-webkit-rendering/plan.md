---
created: 2026-11-24
status: in_progress
requirements:
  - REQ-UI-QUICK-CHAT-ELEVATION-001
system_design:
  - ../../specs/ui/system-design/quick-chat-elevation.md
legacy_specs: []
---

# Fix plan for Quick Chat rendering in WebKit

## Overview

[Issue #5470](https://github.com/kdlbs/kandev/issues/5470) reports soft text
in Quick Chat and Quick Terminal in Orion on macOS. Both tabs use one fixed
dialog panel. The UI system owns its [elevation requirement](../../specs/ui/requirements/quick-chat-elevation.md)
and [panel design](../../specs/ui/system-design/quick-chat-elevation.md#webkit-panel-rendering).

One work order covers the shared panel and its regression tests. It reuses the
WebKit fade used by Create Task and gives Quick Chat its own centering rule.

## Cause and reproduction

At the `sm` breakpoint and above, `QuickChatModalShell` positions the panel
at 50% of each viewport axis and translates it by half its own size. The panel
retains that translation after the animation. The shared dialog also uses
scale keyframes at 95% on entry and exit. A fractional translation can soften
text when WebKit composites the panel. The page blur already lives on a
separate backdrop. [WebKit issue 120583](https://bugs.webkit.org/show_bug.cgi?id=120583)
describes this rendering mechanism.

To reproduce the reported symptom, open Quick Chat in Orion on macOS at 100%
zoom. Compare its tab labels and chat text with comparable text on the page.
Switch to Quick Terminal and repeat the comparison with terminal text.
Resize the panel from a side handle and repeat.

A Linux Playwright WebKit 26.5 probe compiled the current modal and shared
dialog classes with the workspace Tailwind configuration. At a 1200 by 901
CSS pixel viewport and a saved width of 901 CSS pixels, it observed a
persistent `translate: -50% -50%` and transform keyframes in both directions.
The proposed CSS produced `translate: none` and opacity-only keyframes. The
panel retained its 901 by 765.84 CSS pixel size and stayed above the blurred
backdrop. At 639 and 640 CSS pixels, the probe rendered the full-screen and
centered layouts, respectively. The Chromium control retained its original
positioning and keyframes. The reduced 2x WebKit text sample matched before
and after, so the macOS Orion visual check remains part of delivery.

## Scope

### In scope

- Shared Quick Chat and Quick Terminal panel rendering in WebKit.
- Centered tablet and desktop sizing with both resize handles and saved width.
- Full-screen phone layout, internal scroll regions, and touch dismissal.

### Out of scope

- Shared dialog defaults and browser classification.
- Other dialogs, bundled fonts, and xterm renderer settings.
- Conversation, terminal, and workspace state.

## Technical approach

Give `QuickChatModalShell` a local `.quick-chat-dialog` class. Use normal
Tailwind utilities for its insets and translation so the unlayered app CSS
can override them. Keep the existing size utilities and `--quick-chat-width`.

In `apps/web/app/globals.css`, scope `inset: 0`, `margin: auto`, and
`translate: none` to this class under the WebKit marker. Include this class
in the existing opacity-only entry and exit rules. The Create Task
`fit-content` height belongs to its own selector. Quick Chat uses its
viewport-based height.

The existing Radix portal owns the backdrop, focus trap, and dismissal. The
panel keeps its shadow and the backdrop supplies the surrounding blur.

## ASCII UI preview

### UI-01. Quick Chat and Quick Terminal on desktop

Open from the sidebar or keyboard shortcut. Select a chat or terminal tab.

```text
       WebKit report                      Proposed WebKit
    +------------------------+         +------------------------+
    | Chat  Terminal  +    x |         | Chat  Terminal  +    x |
   <| Text appears soft      |>       <| Sharp chat or shell    |>
    | Composer or shell input|         | Composer or shell input|
    +------------------------+         +------------------------+
         Blurred page                      Blurred page
```

The centered panel uses the saved width and an 85dvh height. Side handles
resize from the center. The tab strip stays above the active content.
The transcript or terminal viewport owns scrolling. The composer stays
reachable in a chat. The sketch shows structure. The rendered checks cover
elevation, resizing, and clarity under `AC-UI-QUICK-CHAT-ELEVATION-001.2`,
`.5`, and `.8`.

### UI-02. Quick Chat and Quick Terminal on a phone

Open Quick Chat or Quick Terminal from the Home app menu.

```text
+--------------------------+
| Chat  Terminal  +      x |  Top safe area and fixed tab strip
|                          |
| Transcript or shell      |  Active content scrolls here
|                          |
| Composer or terminal keys|  Bottom safe area
+--------------------------+
```

The current mobile entry flow is the exemplar. A full-screen dialog gives
the transcript or shell room for input. The close button dismisses it and
returns focus to the launcher. The existing composer or terminal key bar
keeps the primary actions reachable. Verify this view for `.4` and `.7`.

## E2E tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-QUICK-CHAT-ELEVATION-001.2`, `.5`, `.8` | New `quick-chat-webkit-rendering.spec.ts` checks the WebKit style path in Chromium and a Chromium control. Confirm the actual text in Orion. |
| `AC-UI-QUICK-CHAT-ELEVATION-001.1`, `.2`, `.5`, `.6` | Run the existing elevation and two-edge resize scenarios in `quick-chat.spec.ts`. |
| `AC-UI-QUICK-CHAT-ELEVATION-001.4`, `.7` | Extend `mobile-quick-chat-entry.spec.ts` with the WebKit layout check in the `mobile-chrome` project. |

The new desktop regression must fail against the current `enter` animation
and `-50% -50%` translation. See the work order for the exact commands.

## Work orders

- [ ] [Task 01. Restore sharp text in the Quick Chat modal](task-01-transform-free-dialog.md)

## Verification results

The compiled CSS probe checked the transforms, geometry, and keyframes in
Linux WebKit and Chromium. An isolated Kandev session exercised both tabs
with the proposed styles applied in the browser. It kept the panel above the
overlay, filled the phone viewport, and closed successfully. A 24 CSS pixel
drag stored a 48 CSS pixel width increase with either centering method.

The production E2E runner built the backend and Vite assets. The focused desktop
suite passed four tests, and the phone entry and WebKit breakpoint suite passed
two tests. Web typecheck, targeted Oxlint, document validation, specification
lint, and `git diff --check` also passed. The native Orion comparison remains
pending because this Linux environment cannot run Orion on macOS.
The review follow-up keeps WebKit auto margins active at every viewport and
asserts the app's normal Chromium engine marker.
The plan remains in progress until the native comparison is recorded.

## Risks

- The focused WebKit-path E2E checks verify computed translation, transform,
  centering, and dimensions after animations settle.
- A browser can use fractional layout coordinates without a transform.
  Check the absence of transforms and centered geometry within one CSS pixel.
- The native Orion rasterizer determines whether the reported text blur is
  fully resolved. Use the same viewport, zoom, and display before and after.
