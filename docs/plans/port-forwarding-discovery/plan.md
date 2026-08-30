---
spec: docs/specs/ui/requirements/port-forwarding-discovery.md
created: 2026-08-07
status: complete
---

# Implementation Plan: Task-scoped port-forwarding discovery

## Overview

Add a merge-safe, task-scoped preference mutation first, then carry the preference through the
frontend task snapshots and wire one shared visibility/open controller into the desktop and
mobile/tablet task surfaces. The existing port dialog and session-scoped tunnel APIs remain the
runtime implementation; focused component, backend, and Playwright coverage will update the current
remote-only assumptions and prove local, remote, persistence, failure, and mobile behavior.

---

## Backend

### Task metadata contract

- Add a task metadata key constant for `port_forwarding_enabled` in
  `apps/backend/internal/task/models/models.go`.
- Add an authorized task-service mutation (for example,
  `SetTaskPortForwardingEnabled(ctx context.Context, id string, enabled bool)`) that reads the task,
  merges only the preference key, preserves all other metadata, persists the task, and publishes the
  normal `task.updated` event after the write.
- Add `PATCH /api/v1/tasks/:id/port-forwarding` to
  `apps/backend/internal/task/handlers/task_handlers.go` and implement its strict boolean request
  parsing and `TaskDTO` response in `task_http_handlers.go`. Reuse the task service's existing
  visibility authorization and error-to-status conventions.
- Leave `port.list`, `port.tunnel.list`, `port.tunnel.start`, `port.tunnel.stop`, and the existing
  `/port-proxy` routes unchanged.

### Backend tests

- Add service coverage for enabling/disabling the key, preserving unrelated metadata, authorization,
  and publishing an updated task event.
- Add handler coverage for valid true/false requests, malformed/non-boolean bodies, not-found access,
  and the returned merged `TaskDTO`.

---

## Frontend

### Task preference propagation and API client

- Add `updateTaskPortForwarding(taskId, enabled)` to
  `apps/web/lib/api/domains/kanban-api.ts` for the new HTTP route.
- Carry `metadata` through the canonical task state in
  `apps/web/lib/state/slices/kanban/types.ts`, `apps/web/lib/kanban/map-task.ts`,
  `apps/web/lib/ssr/mapper.ts`, and `apps/web/components/task/task-page-content-helpers.ts`.
  Preserve cached metadata when a partial `task.updated` event omits it, while accepting an explicit
  metadata object as authoritative.
- Add a shared task-scoped visibility controller/hook under `apps/web/components/task/` that derives
  the preference from task metadata, calls the new mutation, reconciles server events, exposes
  `canToggle` from active-session/agentctl readiness, and coordinates the existing dialog's open
  state. Failed mutations retain the previous state and surface translated feedback.

### Desktop launcher and top bar

- Extend `apps/web/components/task/dockview-add-panel-items.tsx` with the checkable Port forwarding
  row and its shared action props. Thread the active task preference/readiness and mutation handler
  through `dockview-header-actions.tsx` and `dockview-watermark.tsx` so both `+` entry points behave
  identically.
- Refactor `apps/web/components/task/port-forward-dialog.tsx` and
  `apps/web/components/task/task-top-bar.tsx` so the top-bar control is gated by the persisted
  preference plus session/agentctl readiness, not `isRemoteExecutor`. Keep the current port list,
  tunnel actions, active-tunnel indicator, and dialog internals intact.
- Keep enable-from-menu behavior as “toggle and open”: persist successfully, close the menu, and open
  the existing dialog through the shared controller. Disabling only changes visibility.

### Mobile and tablet parity

- Add the shared active-task Port forwarding action to
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`; use the existing Drawer on phone
  and Sheet on tablet, with an explicit labeled row and a minimum 44px touch target.
- Add the enabled top-bar control to
  `apps/web/components/task/mobile/session-mobile-top-bar.tsx`, passing the shared controller through
  `task-layout.tsx` and `session-mobile-layout.tsx`. When enabling from the Drawer, close the surface
  before opening the dialog.
- Adapt the existing dialog presentation for phone viewport height, safe-area padding, keyboard/focus
  dismissal, and one internal scroll owner. Do not add a second tunnel implementation or rely on a
  hidden desktop menu as the mobile path.

### Translations

- Add all new launcher, readiness, mutation-error, and accessibility labels to the task namespace in
  `apps/web/src/locales/en/task.json`, `pt-pt/task.json`, and `pseudo/task.json`; use `t()`/accessible
  labels for every new user-facing string.

---

## Tests

- **What:** the backend stores the preference without replacing unrelated task metadata and emits a
  task update.
  **File:** `apps/backend/internal/task/service/service_port_forwarding_test.go` (or the adjacent
  service test file selected by existing conventions).
  **How:** use the task service/repository test fixture with existing metadata, toggle both values,
  reload the task, and assert the event payload contains the merged metadata.
- **What:** the preference HTTP boundary accepts only booleans and returns the updated task.
  **File:** `apps/backend/internal/task/handlers/task_port_forwarding_test.go` (or the existing HTTP
  handler test file).
  **How:** invoke the Gin handler with valid/invalid JSON and scoped missing-task contexts; assert
  status, response metadata, and no write on invalid input.
- **What:** task snapshot and WS update mapping preserves the preference and does not clobber it when
  metadata is omitted.
  **File:** `apps/web/lib/kanban/map-task.test.ts` and `apps/web/lib/ws/handlers/tasks.test.ts`.
  **How:** map HTTP/WS task shapes with true/false metadata, then apply an unrelated partial update
  and an explicit metadata update.
- **What:** the shared preference controller rolls back on mutation failure and opens the dialog only
  after a successful enable.
  **File:** `apps/web/components/task/port-forwarding-visibility.test.tsx`.
  **How:** mock the API and task readiness, exercise enable/disable, assert calls, checked state,
  dialog-open callback, and translated error feedback.
- **What:** the desktop launcher row is checkable and appears for local and remote ready sessions,
  while the top-bar control follows the saved preference.
  **Files:** `apps/web/components/task/dockview-add-panel-items.test.tsx`,
  `apps/web/components/task/task-top-bar.test.tsx`.
  **How:** render checked/unchecked/disabled states and verify the remote executor flag does not
  change eligibility.
- **What:** mobile/tablet task-switcher actions retain touch reachability and use the same controller.
  **File:** `apps/web/components/task/mobile/session-mobile-layout.test.tsx` and the task-switcher
  component test if one is added by the implementation.
  **How:** render the active-task action, invoke it, and assert the drawer/sheet close plus dialog
  callback path.

---

## E2E Tests

- **Scenario:** local task starts unchecked, then enabling from the desktop `+` menu opens the dialog
  and reveals the top-bar button.
  **File:** update `apps/web/e2e/tests/session/port-forward-dialog.spec.ts` and add launcher locators
  to `apps/web/e2e/pages/session-page.ts`.
  **What to verify:** the local button is hidden by default; the checked row appears after selection,
  the dialog opens, and a reload retains the preference.
- **Scenario:** remote task uses the same preference flow instead of receiving a default visible
  button solely because it is remote.
  **File:** `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`.
  **What to verify:** remote starts unchecked, enabling from the menu reveals the button, and the
  existing detected/manual-port cases still pass through the unchanged dialog.
- **Scenario:** disabling leaves an active tunnel running.
  **File:** `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`.
  **What to verify:** start a tunnel, disable the launcher preference, assert the top-bar button is
  hidden without a stop request, re-enable, and assert the tunnel is listed.
- **Scenario:** phone users can enable/disable from the task-switcher Drawer and use the top-bar
  control without overflow.
  **File:** `apps/web/e2e/tests/session/mobile-port-forwarding.spec.ts`.
  **What to verify:** the action has a 44px-class touch row, the drawer closes before the dialog
  opens, the dialog stays inside the Pixel 5 viewport, and document width does not exceed viewport
  width.
- **Scenario:** a not-ready agentctl session fails closed.
  **File:** `apps/web/e2e/tests/session/port-forward-dialog.spec.ts` or a focused readiness fixture.
  **What to verify:** the launcher entry is disabled with its accessible explanation and the top-bar
  control is absent.

---

## Verification Results

- Backend focused tests: `rtk go test ./internal/task/service ./internal/task/handlers -run
  'Test.*PortForward'` — passed across both packages.
- Backend suite: `rtk make -C apps/backend test` — passed.
- Frontend focused Vitest coverage for the API, mapping, WS merge, visibility controller, launcher,
  top bar, and mobile layout — 65 tests passed across 7 files, plus 43 surface tests across 5 files.
- Frontend typecheck: `rtk pnpm run typecheck` — passed.
- Frontend lint: `rtk pnpm --filter @kandev/web lint` — passed.
- i18n checks: `rtk pnpm run i18n:check` and `rtk pnpm run i18n:ratchet` — passed; the catalog
  checker retained its existing advisory parity warnings for incomplete locale catalogs.
- Formatting: `rtk git diff --check` — passed.
- Production web build: `rtk make build-web` — passed.
- E2E prerequisites: `rtk make build-backend` and `rtk make -C apps/backend e2e-plugin-package` —
  passed.
- Desktop E2E: `rtk pnpm exec playwright test --config e2e/playwright.config.ts
  e2e/tests/session/port-forward-dialog.spec.ts --project=chromium` — 12 passed.
- Mobile E2E: `rtk pnpm exec playwright test --config e2e/playwright.config.ts
  e2e/tests/session/mobile-port-forwarding.spec.ts --project=mobile-chrome` — 1 passed.
- Review-fixup validation: backend port-forward tests — 9 passed across 2 packages; frontend
  focused and surface coverage — 103 tests passed across 11 files; typecheck, lint, formatting,
  and diff checks passed.

All implementation task files and this plan are synchronized to completed status. The E2E fixture
managed temporary repositories and exited cleanly. The not-ready agentctl transition remains an
explicit E2E coverage risk recorded in Task 04; the shared readiness gate is covered by focused
provider and launcher tests.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Backend preference contract](task-01-backend-preference-contract.md)

Wave 2:

- [x] [Task 02: Frontend preference state](task-02-frontend-preference-state.md) (depends on Task 01)

Wave 3:

- [x] [Task 03: Desktop and mobile surfaces](task-03-task-surfaces.md) (depends on Task 02)

Wave 4:

- [x] [Task 04: Port-forwarding coverage](task-04-port-forwarding-coverage.md) (depends on Task 03)

Execution is sequential in the primary conversation; these waves do not authorize subagents.

---

## Open Questions

None. The interaction is confirmed as “toggle and open,” with a preference persisted per task.
