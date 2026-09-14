---
created: 2026-09-03
status: completed
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
legacy_specs: []
---

# Implementation Plan: Backend Restart Page Recovery

## Overview

Publish the backend process identity in each application boot payload. Detect a
changed process after WebSocket connection recovery. Then show one shared reload
alert across every authenticated application route.

Keep the typed agent settings interlock response as a race fallback. Coordinate
the new global state with existing restart and self-update flows.

## Scope

### In scope

- Add the current `boot_id` to HTML and `/api/v1/app-state` boot payloads.
- Check system identity after each successful WebSocket connection.
- Latch one document-local reload-required state after a proven change.
- Show one persistent in-flow alert on all authenticated application routes.
- Add a stable stale-interlock error code and route it to the same state.
- Coordinate intentional restart and self-update reload ownership.
- Add localized desktop, tablet, and phone behavior.
- Add unit, integration, and real-restart E2E coverage.

### Out of scope

- Automatic reload or unsaved-draft persistence.
- Timer-based identity polling.
- Changes to WebSocket retry policy or the public health endpoint.
- Changes to the interim settings interlock threat model.
- Alerts on login or setup screens outside the application shell.

## Technical approach

### Boot identity

- Add optional `bootId` to the backend and frontend boot runtime shapes.
- Source it from the existing process-scoped `info.Service`.
- Include it in both server-rendered HTML and `/api/v1/app-state` responses.
- Extend payload and parser tests before implementation.

### Detection and fallback

- Add a document-local reload coordinator with a one-way state transition.
- Observe shared WebSocket connection transitions in the application shell.
- On each `connected` transition, fetch system information with `no-store`.
- Compare the response with the original boot ID. Ignore failures, missing IDs,
  equal IDs, and responses from older connection checks.
- Add `interim_settings_interlock_required` to interlock rejections.
- Make the shared API client signal the coordinator for that exact code.
- Suppress only the local error that the coordinator handled.

### Global recovery UI

- Render the alert in `AppStatusSurfaceProvider` above route content.
- Use a persistent `role="alert"` surface with one explicit reload action.
- Keep the current route usable so the user can copy unsaved work.
- Register intentional restart ownership in `useKandevRestart` and
  `useSelfUpdate`. Preserve their current progress and error surfaces.
- Add all copy to the six locale catalogs. Generate the Traditional Chinese
  catalogs with `pnpm run i18n:zh-hant`.

## Tests

- **AC 001.1:** Backend payload tests and frontend parser tests cover valid,
  missing, and malformed boot IDs.
- **AC 001.2 through 001.4:** Guard tests cover initial connection, reconnect,
  failed requests, same IDs, changed IDs, and stale responses.
- **AC 001.5 through 001.8:** Coordinator and alert tests cover the one-way
  latch, client-side navigation, deduplication, explicit reload, and no
  automatic reload.
- **AC 001.9 and 001.11:** Middleware and API client tests cover the stable code,
  unrelated HTTP 403 responses, and local-error suppression.
- **AC 001.10:** Restart and self-update hook tests cover reload ownership and
  the single-surface rule.
- **AC 001.12:** Component and browser tests cover keyboard use, phone touch
  size, overflow, and navigation clearance.

## E2E tests

- `apps/web/e2e/tests/layout/backend-restart-page-recovery.spec.ts` opens a
  non-settings route in the `chromium` project. It restarts the real backend,
  waits for WebSocket recovery, and checks the alert before any user action.
  It selects **Reload page** and verifies the fresh document.
- `apps/web/e2e/tests/layout/mobile-backend-restart-page-recovery.spec.ts`
  repeats the flow in the `mobile-chrome` project. It also checks the action
  height, the single scroll owner, horizontal containment, and navigation
  clearance.

The E2E tests seed through API fixtures. They do not simulate a process change
with request interception.

## Work orders

- [completed] [Task 01: Publish page boot identity](task-01-publish-page-boot-identity.md)
- [completed] [Task 02: Detect changed backend generation](task-02-detect-backend-generation.md)
- [completed] [Task 03: Show the global reload alert](task-03-show-global-reload-alert.md)

## Verification results

Implementation and verification passed:

- `make -C apps/backend test`: all backend packages passed.
- Focused backend interlock tests: 154 tests passed.
- Focused frontend recovery, agent-settings, and lifecycle suites: 81 tests
  passed across 12 files.
- `pnpm run lint`, `pnpm run typecheck`, and `pnpm run i18n:check`: passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check`: passed.
- `make build-backend build-web`: passed.
- Managed desktop and mobile real-backend restart E2E tests: one test passed
  in each project, including explicit reload and mobile layout checks.

## Risks

- A network reconnect can look like a restart. The guard must require a changed
  boot ID.
- An older identity response can arrive after a newer connection. The guard
  must reject stale response sequences.
- Intentional restart dialogs can duplicate the global alert. Reload ownership
  must have one active surface and a global fallback.
- A broad HTTP 403 match can hide authorization errors. The API client must
  require the stable interlock code.
- A reload discards draft data. The product must warn the user and wait for an
  explicit action.
