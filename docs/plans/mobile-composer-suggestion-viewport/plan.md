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

PR screenshot feedback exposed a separate test-model defect. The browser test
replaced only `window.visualViewport.height`, so it declared the still-rendered
composer occluded without causing layout reflow. The shared popup correctly
used its containment fallback at the visible bottom edge, producing a screenshot
with a 136-pixel gap to the composer. Task 02 replaces that impossible visual
state with an actual keyboard-sized page viewport and makes direct
menu-to-composer adjacency a permanent browser assertion.

## Requirement coverage

| Acceptance criterion | Work order coverage |
| --- | --- |
| `AC-UI-COMPOSER-OVERLAY-001.1` | Unit geometry regression and mobile saved-prompt E2E in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.2` | Visual-viewport resize component regression in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.3` | Phone geometry, row-height, and overflow assertions in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.4` | Touch insertion and focus assertions plus sibling mobile suites in Task 01 |
| `AC-UI-COMPOSER-OVERLAY-001.5` | Keyboard-sized layout and menu-to-composer bounding-box assertion in Task 02 |

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
- Keep the raw off-screen-anchor condition in deterministic geometry coverage.
- For reviewer-visible browser evidence, resize the page to a keyboard-sized
  height so the mobile layout and composer reflow together, then type a matching
  `@` query.
- Assert the popup surface is directly adjacent to the composer and inside the
  visual viewport, its result row is at least 44 pixels high, and the document
  has no horizontal overflow.
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
- Follow-up browser RED: the new adjacency assertion measured a 136-pixel gap
  when only `visualViewport.height` changed and the composer remained below the
  declared visible area.
- Follow-up GREEN: resizing the page itself to 420 pixels reflowed the mobile
  layout and composer together. The saved-prompt surface remained within eight
  pixels of the composer while retaining viewport, touch-size, overflow,
  insertion, and focus coverage.
- Follow-up shared-consumer run passed all five targeted mobile `@`, `#`, `/`,
  and task-create scenarios. The focused popup suite passed 11 tests; typecheck,
  full web lint, and specification lint also passed.
- Fresh phone evidence was captured and visually inspected at
  `apps/web/.pr-assets/mobile-prompt-mention-composer--mobile-composer-prompt-menu.png`.

## Implementation wave

Execution stays in the primary conversation.

Wave 1:

- [x] [Task 01: Clamp composer suggestions to the visual viewport](task-01-clamp-composer-suggestions.md)

Wave 2:

- [x] [Task 02: Preserve visible composer adjacency](task-02-preserve-composer-adjacency.md)

No task is parallel-safe, and this plan does not authorize subagents.

## Risks

- Computing maximum height from the raw anchor after clamping only `top` would
  leave the surface internally inconsistent. Both values must use the same
  normalized edge.
- Replacing the existing transform with a fixed full-height box would regress
  short-result bottom anchoring.
- Browser E2E runners do not open a real operating-system keyboard. Raw
  visual-viewport occlusion stays in deterministic geometry coverage; visual
  browser evidence must resize page layout so the real composer and popup
  reflow together.
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
