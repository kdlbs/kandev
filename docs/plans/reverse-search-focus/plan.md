---
created: 2026-09-26
status: done
requirements:
  - REQ-UI-REVERSE-SEARCH-FOCUS-001
system_design:
  - ../../specs/ui/system-design/reverse-search-focus.md
legacy_specs: []
---

# Implementation Plan: Restore Chat Focus After History Escape

## Overview

One work order restores focus to the invoking composer when Escape dismisses
Ctrl+R history. Source inspection shows `MessageHistorySearch` calls only
`onClose` for Escape. `TipTapInput` wires that to a state-only close function,
while history selection already closes and focuses the editor. This explains
the observed blur: removing the focused search input leaves no editor focus.

The smallest reliable reproduction is to focus a task chat editor, press
Ctrl+R, then Escape, and type without clicking. The typed text does not reach
the editor. The existing Quick Chat browser test checks that the overlay closes
but does not assert focus return.

## Scope

### In scope

- Return focus to the invoking task chat or Quick Chat editor on Escape.
- Preserve the draft, dialog, Escape ownership, and unrelated dismissal paths.
- Add component and browser regression evidence, including the phone Quick Chat
  path with a hardware-keyboard shortcut.

### Out of scope

- History matching, pagination, result selection, layout, or shortcut changes.
- New touch-only entry points.

## Technical approach

Give `MessageHistorySearch` a dedicated Escape-dismiss callback and invoke it
from the input handler and document fallback. Pass it through `TipTapPopups`
from `TipTapInput`, which closes the overlay and calls the current editor's
focus command. Leave `onClose` for outside clicks, resize, and scroll; keep the
existing result-selection handler. Retain the fallback's overlay/dialog
ownership check and Escape propagation guard.

## ASCII UI preview

UI-01: Task chat, Ctrl+R search and Escape. The same composition is used on a
phone with a hardware keyboard. The caret marker is the behavior change; no
layout, copy, or control order changes. `AC-UI-REVERSE-SEARCH-FOCUS-001.1` and
`.3` apply.

```text
Before Escape                 After Escape
+--------------------------+  Search overlay closed
| Search past messages  [|] |  +--------------------------+
+--------------------------+  | Chat editor          [|] |  <- focus, type now
| Chat editor              |  +--------------------------+
```

UI-02: Quick Chat dialog, desktop and phone hardware-keyboard path. Escape
closes search, leaves the dialog open, and focuses its editor. Phone retains
its current full-screen dialog. `AC-UI-REVERSE-SEARCH-FOCUS-001.2` applies.

```text
+----------------------------+
| Quick Chat             Close |
| History search (open)       |  Escape
| Chat editor                 |    |
+----------------------------+    v
+----------------------------+
| Quick Chat             Close |  dialog stays open
| Chat editor            [|]  |  editor focused
+----------------------------+
```

## Tests

Extend `apps/web/components/task/chat/message-history-search.test.ts` to
assert the Escape callback is used once for the search input and the Quick
Chat document fallback. Assert outside clicks still use the normal close
callback and unrelated Escape targets are ignored. These tests fail on the
current callback contract before the fix.

## E2E tests

- Add `apps/web/e2e/tests/chat/reverse-search-focus.spec.ts` for task chat:
  seed an idle session, open Ctrl+R, Escape, assert editor focus, then type a
  follow-up without clicking and assert it appears only in the draft. Also
  verify resize dismissal keeps its existing focus behavior
  (`AC-UI-REVERSE-SEARCH-FOCUS-001.1`, `.3`).
- Extend `apps/web/e2e/tests/chat/quick-chat.spec.ts` to assert the existing
  Quick Chat search test returns focus and preserves the dialog. Add a focused
  check for Escape after focus moves elsewhere inside the dialog
  (`AC-UI-REVERSE-SEARCH-FOCUS-001.2`).
- Add `apps/web/e2e/tests/chat/mobile-reverse-search-focus.spec.ts` for the
  shared phone Quick Chat composer with Playwright keyboard Ctrl+R and Escape;
  assert focus and immediate typing. The physical-keyboard path shares the
  current phone dialog; touch-only users have no Ctrl+R or Escape key.

## Work orders

- [x] [Task 01: Restore editor focus on history Escape](task-01-restore-focus.md)

## Verification results

- `pnpm test -- components/task/chat/message-history-search.test.ts`: 11 passed.
- `make build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package`: passed; final frontend edits also passed `make build-web-e2e`.
- Desktop reverse-search E2E: 3 passed. Mobile Quick Chat E2E: 1 passed.
- `pnpm run typecheck`, focused ESLint, and `pnpm run i18n:ratchet`: passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.

## Risks

- Quick Chat's dialog focus trap and clarification Escape guard can compete
  with focus restoration. Browser assertions must verify the dialog stays open.
- Search can coexist with multiple composers. Escape must use the invoking
  editor and not claim an unrelated task-panel Escape.
