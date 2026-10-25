---
spec: docs/specs/ui/requirements/port-proxy-browser-panel.md
created: 2026-08-07
status: complete
---

# Implementation Plan: Open proxy URLs in the browser panel

## Overview

Add a Dockview action that reuses an existing Browser panel or creates one in the central group.
Update the Browser panel when Dockview parameters change. Add the action to proxy URL rows only,
close the Port Forwarding dialog after a successful navigation, and keep the existing system
browser action as the phone/tablet fallback. No backend or port-forwarding protocol change is
needed.

The work is sequential: implement and test the Dockview navigation action, wire the dialog and
translations, then add desktop and mobile Playwright coverage.

---

## Backend

No backend changes. The feature uses the existing proxy URL and does not add an HTTP route,
WebSocket message, database field, or authorization rule.

---

## Frontend

### Dockview browser navigation

- `apps/web/lib/state/dockview-panel-actions.ts`: add an `openBrowserPanel(url)` action. Prefer the
  active panel when its component is `browser`; otherwise use the first Browser panel in
  `api.panels`. For an existing panel, call `updateParameters({ url })` and `setActive()`. If no
  panel exists, call `focusOrAddPanel` with a `browser:<url>` ID, the `browser` component, the URL
  parameter, and `centerGroupId` placement.
- `apps/web/lib/state/dockview-store.ts`: expose `openBrowserPanel` in the store type through the
  existing `buildPanelActions` spread.
- `apps/web/lib/state/dockview-panel-actions.test-utils.ts` and
  `apps/web/lib/state/dockview-panel-actions.test.ts`: extend the Dockview mock with component
  identity and test existing-panel reuse, active-panel preference, central placement, URL
  parameters, and no duplicate panel creation.

### Browser panel URL synchronization

- `apps/web/components/task/browser-panel.tsx`: synchronize the local address value and draft with
  a changed `params.url`. Keep the existing detected preview URL fallback and iframe loading rules.
  Add a stable `data-testid` to the Browser panel root for Playwright assertions if the existing
  panel primitives do not provide one.

### Port Forwarding dialog action

- `apps/web/components/task/port-forward-dialog.tsx`: add an optional browser-panel callback to
  `UrlActions`, pass it only to the proxy `PortUrlRow`, and render an accessible Browser-panel icon
  action beside copy and system-browser actions. Read the Dockview action from the store only when
  the desktop Dockview API is available. Invoke the callback and close the dialog after navigation.
  Keep the action out of tunnel rows.
- Keep URL action controls usable with coarse pointers. Use a 44px hit area for the shared mobile
  fallback controls and the compact existing desktop size through responsive classes. Keep one
  internal dialog scroll owner and preserve the current viewport containment.
- `apps/web/src/locales/en/task.json`, `apps/web/src/locales/pt-pt/task.json`,
  `apps/web/src/locales/zh-cn/task.json`, and `apps/web/src/locales/pseudo/task.json`: add the
  translated `openInBrowserPanel` label. All new accessible copy and tooltip text uses `t()`.

### Mobile and tablet behavior

- Do not mount or add a new Dockview group to phone/tablet layouts. The Dockview action is absent
  when the Dockview API is unavailable.
- Keep the existing system browser action visible and touch-sized in the shared Port Forwarding
  dialog. This is the mobile/tablet path to the same proxy URL.
- Use the existing Port Forwarding dialog, its internal scroll region, and the existing mobile task
  drawer entry. Do not add a second port or tunnel state model.

---

## Tests

- **What:** an existing Browser panel is updated and focused, and no new panel is created.
  **File:** `apps/web/lib/state/dockview-panel-actions.test.ts`.
  **How:** use the current Dockview test mock; seed Browser panels in custom groups, call
  `openBrowserPanel`, and assert URL parameters, active state, group identity, and panel count.
- **What:** no Browser panel creates one in the central group with the requested URL.
  **File:** `apps/web/lib/state/dockview-panel-actions.test.ts`.
  **How:** call the action on an empty mock API and assert the added panel's component, ID,
  `params.url`, group, and active state.
- **What:** a changed Dockview URL parameter updates the Browser panel address and navigation
  source without changing the detected-preview fallback.
  **File:** `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`.
  **How:** open a proxy URL in an existing Browser panel and assert the address input updates;
  open a proxy URL in a new Browser panel and assert the iframe source uses the proxy URL.
- **What:** the dialog exposes the action only for proxy rows and keeps the existing actions.
  **File:** `apps/web/components/task/port-forward-dialog.test.tsx` or the closest focused dialog
  test harness.
  **How:** render the row with a mocked Dockview action, invoke the Browser-panel action, and
  assert the callback and dialog close path; render a tunnel row and assert no Browser-panel action.
- **What:** new translation keys pass catalog checks and changed files pass frontend type and lint
  rules.
  **Files:** locale files and changed frontend files.
  **How:** run the focused Vitest commands, `cd apps/web && pnpm run typecheck`,
  `cd apps && pnpm --filter @kandev/web lint`, `cd apps && pnpm run i18n:check`, and
  `cd apps && pnpm run i18n:ratchet`.

---

## E2E Tests

- **Scenario:** GIVEN no Browser panel, WHEN a user adds a manual port and clicks the proxy row's
  Browser-panel action, THEN a Browser panel is active in the central group, its address input
  contains the proxy URL, its iframe has the proxy URL, and the dialog is closed.
  **File:** `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`.
  **Page object:** add proxy Browser-panel action and Browser panel locators to
  `apps/web/e2e/pages/session-page.ts`.
- **Scenario:** GIVEN an existing Browser panel in a group, WHEN the user opens a different proxy
  URL in the Browser panel, THEN the same Browser panel remains and its URL changes.
  **File:** `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`.
  **What to verify:** Dockview reports one Browser component, the panel is active, and the address
  input contains the new URL.
- **Scenario:** GIVEN a phone viewport, WHEN the user opens the Port Forwarding dialog, THEN the
  existing system-browser fallback remains reachable, the dialog stays within the viewport, and
  document horizontal overflow is zero.
  **File:** `apps/web/e2e/tests/session/mobile-port-forwarding.spec.ts`.
  **What to verify:** add a manual row, inspect its system-browser link, and run the existing
  containment assertions.

---

## Verification Results

- `cd apps && pnpm --filter @kandev/web test -- lib/state/dockview-panel-actions.test.ts components/task/port-forward-dialog.test.tsx` — 33 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web lint` — passed with zero warnings.
- `cd apps && pnpm --filter @kandev/web i18n:check` — passed; existing real-locale parity warnings remain advisory.
- `cd apps && pnpm --filter @kandev/web i18n:ratchet` — passed.
- `cd apps && pnpm --filter @kandev/web build:e2e` — passed.
- `cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/session/port-forward-dialog.spec.ts` — 14 tests passed.
- `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-port-forwarding.spec.ts` — 1 test passed.
- `git diff --check` — passed.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Dockview browser navigation](task-01-dockview-browser-navigation.md)

Wave 2:

- [x] [Task 02: Port dialog browser action](task-02-port-dialog-browser-action.md) (depends on
  Task 01)

Wave 3:

- [x] [Task 03: Browser navigation E2E coverage](task-03-browser-navigation-e2e.md) (depends on
  Task 02)

Execution is sequential in the primary conversation. These waves do not authorize subagents.

## Open Questions

None. The desktop group behavior and the phone/tablet fallback are defined in the spec.
