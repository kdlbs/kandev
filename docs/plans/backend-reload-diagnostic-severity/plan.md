---
created: 2026-09-05
status: done
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
  - ../../specs/platform/system-design/expected-runtime-log-severity.md
legacy_specs: []
---

# Implementation Plan: Backend Reload Diagnostic Severity

## Overview

This fix keeps the reload-required behavior and records its report at info
level. One vertical work order changes the browser report contract and the
backend severity mapping together.

## Root cause

The backend-reload coordinator sends its expected lifecycle signal through the
error-toast report source. The backend treats every accepted report as an error
and uses the fixed message `frontend error toast`.

## Scope

### In scope

- Give backend-reload reports a distinct, allow-listed source.
- Record valid backend-reload reports at info level with a fixed message.
- Keep actual Sonner and legacy error-toast reports at error level.
- Remove synthetic error-toast data from backend-reload reports.

### Out of scope

- Backend restart detection and boot ID generation.
- The reload-required state, alert, copy, and action.
- The report endpoint path, rate limits, field limits, or storage policy.
- General log-severity changes for other frontend events.

## Technical approach

### Browser report contract

`apps/web/lib/api/domains/frontend-error-log-api.ts` adds a dedicated
backend-reload report path. The path uses `backend-reload` as its source and
does not create an error-toast stack or error object.

`apps/web/src/main.tsx` uses the dedicated path. The title remains the
stable recovery signal from `BackendReloadSignal`.

### Backend classification

`apps/backend/internal/system/frontenderrors/handler.go` accepts
`backend-reload` only with `boot_id_changed` or
`settings_interlock_rejected`. It records these reports at info level with
the fixed message `frontend backend reload required`.

The existing `sonner` and `toast-provider` sources keep the fixed
`frontend error toast` error entry. Client fields remain structured fields
and cannot select the severity.

## Tests

- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.17` maps to a new observer
  test in `apps/backend/internal/system/frontenderrors/handler_test.go`.
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.18` maps to the existing
  error-toast observer test and an explicit unchanged-severity assertion.
- Frontend report tests cover the new source and the absence of synthetic
  toast-error data.
- Existing generation-guard and coordinator tests confirm that restart
  detection remains unchanged.

The backend observer test is the regression test. Before the correction, a
backend-reload report is rejected or represented as an error-toast entry.

## E2E tests

No E2E change is required. The existing restart-recovery E2E behavior and user
interface remain unchanged.

## Work orders

- [x] [Task 01: Classify backend-reload reports](task-01-classify-backend-reload-reports.md)

## Verification results

- `go test ./internal/system/frontenderrors -count=1`: 17 tests passed.
- Focused Vitest command: 3 files and 20 tests passed.
- `pnpm run typecheck`: passed.
- Specification lint and `git diff --check`: passed.

## Risks

- An untrusted browser can try to use the new source to reduce severity. The
  backend accepts only the two stable recovery titles.
- Info entries do not appear on default warning-only stdout. They remain in the
  retained backend log and diagnostic bundles.
