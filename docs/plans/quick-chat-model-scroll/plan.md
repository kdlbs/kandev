---
created: 2026-09-15
status: implemented
requirements:
  - REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001
system_design:
  - ../../specs/ui/system-design/quick-chat-viewport-layout.md
legacy_specs: []
---

# Implementation Plan: Quick Chat model scrolling

## Overview

Restore scrolling in the composer model picker within Quick Chat. One sequential
work order covers portal containment and desktop/mobile behavioral regression tests.

## Scope

Include popup containment, real scroll input, selection, focus, and dismissal.
Exclude catalog changes, model APIs, global dialog defaults, and UI redesign.
UI owns this scroll contract; adjacent agents specifications own provider data.
The user requests existing scrolling behavior, with no unresolved product choice.

## Evidence and technical approach

Source trace: `QuickChatModal` uses modal `Dialog`; `ModelConfigSelector` uses
non-modal `Popover` with default body portal. `CommandList` already provides
bounded overflow. Installed Radix Dialog wraps its overlay with `RemoveScroll`
and permits `contentRef` as a shard. Installed react-remove-scroll 2.7.2 checks
`node.contains(event.target)` and prevents wheel/touch events outside its shards.
The body-portaled picker is outside the permitted content subtree.

This is source evidence, not a browser reproduction. Worktree dependencies are
absent; behavioral RED is required before the production correction.

Resolve the selector trigger's nearest dialog content on opening and use the
existing `PopoverContent.portalContainer`; keep the body fallback outside dialogs.
The existing task-create selectors and composer reverse-search use dialog-local
portals. Keep this change local to `ModelConfigSelector`, without introducing a
new shared portal framework or disabling modal behavior.

## ASCII UI preview

### UI-01: Composer model picker, open

```text
Quick Chat dialog
  [Search models                 ]
  [Model A                       ]
  [Model B                       ]  <- model list scrolls
  [More models below             ]
  [Provider options              ]
  [Composer text                 ]
  [Model v]                 [Send]
```

Desktop and phone retain this control hierarchy. Desktop is anchored above the
composer; phone uses the existing viewport-contained picker in the full-height
dialog with coarse-pointer targets. Search and provider options are outside the
model scroll region. Structure and scroll ownership are required; spacing and
labels are illustrative. Existing localized copy remains authoritative.
Maps to AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.7 and .8.

## Tests and E2E tests

Extend `components/model-config-selector.test.tsx` for dialog containment and
non-modal fallback, with a replaced-dialog/reopen case if container state is kept.

Add `e2e/tests/chat/quick-chat-model-scroll.spec.ts` (chromium): seed an overflowing
model list using the existing E2E store pattern, retain real supported mock models
at the end, open Quick Chat, wheel over the list, assert scrollTop increases, and
select a revealed supported model. Test search, Escape focus return, background
position, and the non-modal task selector. Cover .7 and .8.

Add `e2e/tests/chat/mobile-quick-chat-model-scroll.spec.ts` (mobile-chrome): open
through Home's mobile Quick Chat action, send a real touch gesture over the list,
assert scroll movement and tap a revealed supported model. Check viewport
containment and horizontal overflow. Cover .7 and .8.

## Work orders

- [x] [Task 01: Restore modal picker scrolling](task-01-modal-picker-scroll.md)

## Verification results

Task 01 complete. The component RED test and real Chromium wheel RED reproduced
the scroll-boundary defect before the correction. Final checks passed:

- 18 component tests.
- 3 desktop and 2 mobile Playwright scenarios using one worker.
- TypeScript, targeted ESLint, and Prettier checks.
- Local runtime/plugin and E2E frontend builds.
- Specification catalog, specification lint, and `git diff --check`.

Exact commands and environment adaptations are recorded in the work order.
No production CSS, provider contract, or public copy changed.

## Risks

- A portal inside transformed dialog content must retain correct positioning.
- Synthetic scrollTop changes would miss the reported event-cancellation bug.
- Synthetic catalogs can be overwritten by late hydration; seed after readiness.
- Confirm browser behavior before production edits; source evidence does not
  establish every viewport or input-device outcome.
