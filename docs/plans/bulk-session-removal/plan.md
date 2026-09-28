---
created: 2026-09-27
status: done
requirements:
  - REQ-TASKS-BULK-SESSION-REMOVAL-001
  - REQ-TASKS-BULK-SESSION-REMOVAL-002
system_design:
  - ../../specs/tasks/system-design/bulk-session-removal.md
legacy_specs: []
---

# Implementation Plan: Bulk Session Removal

## Overview

Make permanent task-session removal distinct from Dockview panel closure. Build
the task-scoped snapshot and ordered deletion helper first, then wire the same
flow into desktop tabs and the mobile Sessions picker with locale and E2E
evidence.

## Scope

### In scope

- Task-scoped Remove Others and Remove All with exact count confirmation.
- Existing `session.delete` requests, sequential client orchestration, and
  partial-failure reporting.
- Desktop tab-menu and phone picker access, localization, and focused browser
  coverage.
- A concise user-facing documentation update explaining permanent conversation
  deletion and workspace retention.

### Out of scope

- Backend bulk deletion or transaction support.
- Stopping active sessions, deleting task-owned workspace resources, or
  changing Quick Chat and preview-tab menus.

## Technical approach

Add a pure target/eligibility module near the session domain and test it before
production use. Extend `use-session-actions` with a callable deletion-by-ID
path that keeps current selection and projection cleanup correct as each target
finishes. A bulk owner captures and revalidates the task-session snapshot,
orders non-primary sessions before primary sessions, and stops on the first
failure.

Wire the owner into `session-tab.tsx` and `session-tab-menu.tsx` without
changing the existing Close Others callback. Keep the confirmation owner alive
outside the selected removable tab. Add the matching action controls and
hosted confirmation state in `mobile-sessions-section.tsx`; reuse the existing
mobile picker confirmation host. Add all localized strings in the task
catalogs and document the behavior in `docs/public/tasks-and-workflows.md`.

## ASCII UI preview

`UI-01: Desktop task-session context menu and confirmation` (AC-001.1 through
AC-001.6). Close Others only hides panels. Remove choices delete persisted
conversations.

```text
Session tab context menu
+----------------------------+
| Rename                     |
| ...                        |
| Close Others               |
| -------------------------- |
| Remove Others              |
| Remove All                 |
+----------------------------+

Remove Others confirmation
+-----------------------------------------+
| Remove 3 sessions?                      |
| Permanently delete 3 conversation       |
| histories. Task workspace and files     |
| remain. This action cannot be undone.   |
|                         [Cancel] [Remove]|
+-----------------------------------------+
```

`UI-02: Phone Sessions picker confirmation` (AC-002.1 through AC-002.3).
The existing sheet hosts this step; the list is restored after cancel or Back.

```text
+----------------------------------+
| Sessions                         |
| [Agent A] ... [Actions]          |
| [Agent B] ... [Actions]          |
+----------------------------------+
             Actions
        [Remove others]
        [Remove all]

+----------------------------------+
| Remove 2 sessions?               |
| Conversations are permanently    |
| deleted. Workspace files remain. |
| [Cancel]                 [Remove]|
+----------------------------------+
```

The control order, destructive grouping, count, hosted phone step, and
touch-size requirement are contractual. Spacing and exact visual tokens are
illustrative and must use existing primitives.

## Tests

- `AC-TASKS-BULK-SESSION-REMOVAL-001.2` and `.3` through `.6`: focused Vitest
  tests for target selection, snapshot validity, ordered dispatch, cleanup,
  duplicate submission, and partial failure.
- `AC-TASKS-BULK-SESSION-REMOVAL-001.1` and `.4`: focused menu/confirmation
  rendering tests.
- `AC-TASKS-BULK-SESSION-REMOVAL-002.1` through `.3`: mobile picker tests and
  locale checks.

## E2E tests

- Chromium: extend `e2e/tests/session/session-tab-management.spec.ts` for
  cancel, hidden-session Remove Others, Remove All, active-target refusal, API
  list, and reload persistence.
- Mobile Chrome: add `e2e/tests/session/mobile-bulk-session-removal.spec.ts`
  for both scopes, hosted confirmation, touch geometry, focus/dismissal, and
  document overflow.

## Work orders

- [x] [Task 01: Implement task-scoped bulk session removal](task-01-bulk-session-removal.md)

## Verification results

Completed. Focused Vitest, typecheck, localization checks, specification and
public-doc validators, backend/web builds, and the desktop and phone browser
flows passed. One unrelated task-switching browser case encountered a temporary
"web app unavailable" fixture response during the full desktop run; its focused
rerun passed. The phone confirmation screenshot was inspected at the Pixel 5
viewport.

The 2026-09-28 PR fixup adds coverage for partial Remove All failure cleanup and
failed-refresh messaging. Its focused tests (18 passed), web typecheck,
`i18n:check`, and `i18n:ratchet` passed.

QA on 2026-09-28 found that creating a session after Remove All left the
auto-provisioning fence active. The New Session success path now clears it;
the regression test, 62 focused unit tests, typecheck, spec validators, and
desktop/phone browser flows passed.

## Risks

- `session.delete` is non-atomic across targets; a server refusal after an
  earlier success leaves a truthful partial result.
- A tab-local coordinator can unmount when Remove All deletes its own session.
- WebSocket and REST reconciliation can race during the ordered loop.
