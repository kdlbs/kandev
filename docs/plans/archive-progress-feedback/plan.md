---
created: 2026-09-17
status: implemented
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-004
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
legacy_specs: []
---

# Implementation Plan: Archive progress feedback

## Overview

Add one persistent loading toast around every user-initiated archive request,
then prove its desktop and phone presentation with the existing deferred
archive scenarios. The shared action hook changes first so all task surfaces
receive the behavior; the `/tasks` listing then joins that boundary, and the
same end-to-end flows capture review screenshots.

## Scope

### In scope

- Localized `Archiving in progress` feedback in the existing bottom-right toast
  stack for single and bulk user archive actions.
- One progress toast per operation or bulk batch, retained until the last
  archive request settles and dismissed on either success or failure.
- The `/tasks` listing and every archive surface already routed through
  `useTaskActions`.
- Desktop and phone Playwright evidence plus PR-ready screenshots of the
  controlled pending state.

### Out of scope

- Archive API, persistence, permissions, confirmation, cascade, navigation,
  recovery, success, or failure semantics.
- Delete, unarchive, API, CLI, MCP, agent-driven, and other programmatic archive
  operations.
- A new toast component, placement system, setting, feature flag, or mobile
  navigation composition.

## Technical approach

### Shared archive action feedback

Add a private progress-toast lifecycle inside
`apps/web/hooks/use-task-actions.ts`. `archiveTaskById` creates the existing
non-expiring `loading` toast before invoking `archiveTask`, tracks concurrent
requests from the same hook instance, and dismisses it in `finally` only after
the final request settles. Keep outcome feedback with the current coordinator
and surface handlers.

Update `apps/web/app/tasks/tasks-page-client.tsx` to call
`archiveTaskById` instead of importing the transport function directly. Preserve
that listing's current refresh and terminal toast behavior.

Add `tasks:archivingInProgress` to English, pseudo, Portuguese, Simplified
Chinese, and generated Traditional Chinese catalogs. Do not place translated
copy at module scope.

### Rendered verification and PR evidence

Extend `apps/web/e2e/tests/task/sidebar-immediate-archive-helpers.ts` while its
archive route is deliberately held. Assert the loading toast, spinner, polite
live region, bottom-right placement, and later removal before checking the
existing success and failure outcomes. Reuse the helper from the current
desktop and `mobile-chrome` specs.

Pass `prCapture` from both specs and capture one full-page pending-state image
per viewport. `CAPTURE_PR_ASSETS=1` writes both screenshots and their captions
to `apps/web/.pr-assets/manifest.json` for the PR workflow.

Nearest phone exemplar: the existing `ToastProvider` stack itself. It supplies
the fixed, status-bar-aware placement, live announcement, and non-blocking
presentation. This change adds a state to that shared surface, with no new
phone action, navigation, scroll owner, or touch target.

## ASCII UI preview

### UI-01: Desktop archive pending

Entry point: any desktop task archive action after confirmation or immediate
acceptance. State: the archive HTTP request is pending.

```text
+------------------------------------------------------------------+
| Kandev                                                     [user] |
|                                                                  |
|  Current task or destination remains usable                      |
|                                                                  |
|                              +-------------------------------+   |
|                              | (spinner) Archiving in progress|   |
|                              +-------------------------------+   |
+------------------------------------------------------------------+
                               fixed bottom-right toast stack
```

### UI-02: Phone archive pending

Entry point: any phone task archive action after confirmation or immediate
acceptance. State: the archive HTTP request is pending.

```text
+------------------------------+
| Task                         |
|                              |
| Current task or destination  |
| remains usable               |
|                              |
| +--------------------------+ |
| | (spinner) Archiving in   | |
| |           progress       | |
| +--------------------------+ |
+------------------------------+
  fixed above app status bar; viewport-contained
```

Structural requirements: the toast is non-blocking, localized, announced by
the existing polite live region, contains a visible loading indicator, and
does not time out while the request is pending. One operation or bulk batch
shows one toast. Spacing and line wrapping are illustrative and continue to use
the existing toast component. These previews map to
`AC-TASKS-REMOVAL-NAVIGATION-004.1` through `.3`.

## Tests

- `apps/web/hooks/use-task-actions.test.ts`: a deferred archive creates the
  localized loading toast before settlement, does not dismiss it early,
  dismisses on resolve and reject, and coalesces concurrent requests from one
  hook instance.
- Existing terminal success and failure assertions remain unchanged, proving
  the progress state adds no second outcome notification.

## E2E tests

- `apps/web/e2e/tests/task/sidebar-immediate-archive.spec.ts` covers
  `AC-TASKS-REMOVAL-NAVIGATION-004.1` through `.3` on desktop and captures
  `UI-01`.
- `apps/web/e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts` covers the
  same criteria in the `mobile-chrome` project, verifies viewport containment,
  and captures `UI-02`.

## Work orders

- [x] [Task 01: Show archive progress feedback](task-01-show-archive-progress.md)

## Verification results

- Focused `useTaskActions` unit suite passed with resolve, reject, and
  concurrent-request coverage.
- Web typecheck and the complete i18n catalog checks passed.
- Desktop Chromium and phone `mobile-chrome` archive scenarios passed and
  produced reviewed, compressed PR screenshots.
- Specification, public-doc, and diff validation passed.

## Risks

- Bulk archives can accidentally create one toast per task unless the shared
  action hook counts concurrent requests from the same operation.
- A missing `finally` dismissal can leave a permanent loading toast after a
  rejection or navigation transition.
- The fixed-width desktop toast must stay inside the configured phone viewport;
  rendered geometry and horizontal-overflow checks guard that contract.
