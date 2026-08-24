---
spec: docs/specs/ui/requirements/composer-suggestion-overlays.md
system_design: docs/specs/ui/system-design/composer-suggestion-overlays.md
created: 2026-08-23
status: implemented
---

# Implementation Plan: Keep Mobile Composer Suggestions Visible

## Overview

Repair the shared popup geometry used by composer suggestion menus. A normal
Pixel-sized browser run proved that typing `@mobile-prompt` opens the saved
prompt menu and touch selection inserts the prompt chip. A second run modeled a
software keyboard by shrinking `window.visualViewport.height` to 420 CSS
pixels. The menu still rendered with its bottom at 551 pixels, leaving 131
pixels behind the keyboard. The focused browser assertion failed as expected.
The throwaway diagnostic spec was removed after recording this evidence.

The root cause is in `computePopupMenuStyle`. It constrains width and maximum
height with visual-viewport dimensions, but it derives the vertical anchor from
the unchanged layout-viewport caret coordinate. The existing visual-viewport
resize listener therefore rerenders the same off-screen vertical position.

The default above placement is shared by chat `@` mentions (including custom
prompts), `/` agent commands, `#` entity references, and the shared task/agent
prompt composer. The plan editor's slash menu shares `PopupMenu` but requests
below placement; it needs a geometry regression, not new behavior.

## Requirement coverage

| Acceptance criterion | Work order coverage |
| --- | --- |
| `AC-UI-COMPOSER-OVERLAY-001.1` | Unit geometry regression and mobile saved-prompt E2E in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.2` | Visual-viewport resize component regression in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.3` | Phone geometry, row-height, and overflow assertions in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.4` | Touch insertion and focus assertions plus sibling mobile suites in Task 01 |

## Frontend

- Add a failing `computePopupMenuStyle` regression where an above-placement
  anchor remains below a shrunken visual viewport.
- Normalize the rendered vertical edge against the current visual viewport
  before calculating available height.
- Preserve the existing eight-pixel inset, 420-pixel focused width, 280-pixel
  height cap, short-list bottom anchoring, and body portal.
- Keep the existing window and visual-viewport event subscriptions. No new
  hook, store state, or dependency is required.
- Add ordinary above- and below-placement cases so the fix cannot shift desktop
  menus or the plan editor's below-caret menu.
- Do not change mention queries, custom-prompt loading, entity-reference search,
  agent command handling, localization, or insertion logic.

## Tests

- Extend `apps/web/components/task/chat/popup-menu.test.tsx` first. Link the new
  cases to `AC-UI-COMPOSER-OVERLAY-001.1` and
  `AC-UI-COMPOSER-OVERLAY-001.2` in nearby comments.
- Prove that a raw anchor at `y=560` with a 420-pixel visual viewport yields a
  rendered above-menu bottom no lower than the padded viewport bottom and a
  maximum height calculated from that normalized edge.
- Preserve the current offset-viewport, focused-width, short-list anchor, and
  resize cases. Add a below-placement assertion for unchanged ordinary
  geometry.

## Mobile E2E

- Add
  `apps/web/e2e/tests/chat/mobile-prompt-mention-composer.spec.ts` using the
  managed E2E fixture and `mobile-chrome` project.
- Create a uniquely named custom prompt through the API, open a ready task, and
  activate the real TipTap composer.
- Model an overlay software keyboard with an EventTarget-compatible
  `visualViewport`, then dispatch the resize event and type a matching `@`
  query.
- Assert the popup surface is inside the visual viewport, its result row is at
  least 44 pixels high, and the document has no horizontal overflow.
- Tap the saved prompt and assert that the menu closes, the prompt appears in
  the draft, the composer retains focus, and no message is sent.
- Delete the created prompt in `finally` so the test remains isolated.
- Run the existing mobile entity-reference, slash-command, and task-create
  mention-menu scenarios as shared-consumer regressions.

## Backend

No backend, API, persistence, or migration change is required. The diagnostic
run proved that the custom-prompt data reaches the menu when geometry is
visible.

## Verification commands

Run the focused unit test during both red and green TDD steps:

```bash
cd apps/web && pnpm test -- components/task/chat/popup-menu.test.tsx
```

Run the rebuilt phone-browser scenarios after the unit regression passes:

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-prompt-mention-composer.spec.ts \
  tests/chat/mobile-entity-reference-composer.spec.ts \
  tests/chat/mobile-slash-command-composer.spec.ts \
  tests/task/mobile-task-create-escape.spec.ts
```

Run frontend static checks after implementation:

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Verification results

- Unit RED: `pnpm test -- components/task/chat/popup-menu.test.tsx` failed the
  two new viewport regressions as expected (2 failed, 9 passed). Both received
  the stale `552px` top instead of the contained `412px` top.
- Browser RED: the new `mobile-chrome` saved-prompt scenario failed with popup
  bottom `551px` beyond the simulated visual-viewport limit of `421px`.
- Unit GREEN: the focused popup-menu suite passed (11 tests).
- Browser GREEN: the rebuilt saved-prompt scenario passed (1 test), including
  visual-viewport containment, a 44-pixel touch row, no document horizontal
  overflow, touch insertion, and focus retention.
- Shared-consumer GREEN: the rebuilt `mobile-chrome` command covering the new
  `@` scenario, existing `#` entity-reference scenario, existing `/` command
  scenario, and task-create mention-menu scenarios passed (5 tests).
- `pnpm run typecheck` passed.
- Full web lint initially reported the popup test callback over its line limit.
  Splitting geometry and viewport-update groups resolved it; the final full web
  lint passed with zero warnings.
- PR review remediation qualified the legacy entity-reference anchoring text so
  it matches the authoritative off-screen-anchor clamp. Specification lint
  passed afterward.

## Implementation wave

Execution stays in the primary conversation.

Wave 1:

- [x] [Task 01: Clamp composer suggestions to the visual viewport](task-01-clamp-composer-suggestions.md)

No task is parallel-safe, and this plan does not authorize subagents.

## Risks

- Computing maximum height from the raw anchor after clamping only `top` would
  leave the surface internally inconsistent. Both values must use the same
  normalized edge.
- Replacing the existing transform with a fixed full-height box would regress
  short-result bottom anchoring.
- Browser E2E runners do not open a real operating-system keyboard. The test
  must model the Visual Viewport API condition explicitly and still exercise
  the real composer and popup.
- A shared primitive change can affect the plan editor's below placement even
  though the reported menus use above placement. Focused below-path coverage is
  required.

## Public documentation

None. The fix restores expected responsive behavior and changes no public
command, configuration key, API, or workflow.

## Decisions

No ADR is required. The selected change corrects existing viewport containment
inside one shared UI primitive and introduces no durable architectural
alternative.
