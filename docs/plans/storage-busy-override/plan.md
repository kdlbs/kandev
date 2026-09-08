---
spec: docs/specs/system-page/requirements/storage-maintenance.md
created: 2026-07-27
status: implemented
---

# Implementation Plan: Storage Busy Feedback and Override

## Overview

Make a blocked manual cleanup explain the Kandev activity categories that hold the gate, then let
the operator explicitly run the same cleanup alongside current task activity. Preserve the
install-wide mutual exclusion that prevents overlapping maintenance runs, and keep future task work
able to cancel maintenance as it does today. The work proceeds from backend activity/API semantics,
through the shared frontend controller and Storage page, to desktop and Pixel 5 coverage.

## Backend

### Activity-gate and manual-run contract

- Extend `apps/backend/internal/agent/runtime/activity/coordinator.go` with a typed public busy
  resource shape that retains the stable `Kind` and supplies the user-facing label. Preserve the
  existing kind values and sorting, but do not expose task prompts, paths, IDs, or names.
- Add an explicit manual-maintenance acquisition mode that may ignore already-active task work but
  still rejects `maintenance_running`. It must install the normal maintenance lease so a task that
  begins after the force run can cancel it through `AcquireTask`.
- Thread an explicit `force` flag through `apps/backend/internal/system/storage/operations.go` and
  `runner.go`. Normal manual and scheduled paths retain their current admission behavior; only a
  manual force request skips the current-activity check.
- Extend the `Mutations` interface and `runRequest` in
  `apps/backend/internal/system/storage/handler.go`. A busy response returns typed
  `busy_resources` and `force_available`; accepted forced runs retain the existing `202 { job_id }`
  response.

### Backend tests

- Update `apps/backend/internal/agent/runtime/activity/coordinator_test.go` to prove force admission
  coexists with active task work, refuses an existing maintenance lease, and remains preemptible by
  subsequently admitted task activity.
- Extend `apps/backend/internal/system/storage/operations_test.go`, `runner_test.go`,
  `handler_test.go`, and `storage_routes_test.go` for the typed 409 response, force=false rejection,
  force=true job start, and maintenance-running refusal.

## Frontend

### API, types, and domain controller

- Add the typed busy response/resource contracts to `apps/web/lib/types/system.ts` and let
  `runStorageMaintenance` in `apps/web/lib/api/domains/system-api.ts` send an optional `force` body
  field without changing callers that omit it.
- In `apps/web/hooks/domains/system/use-storage-maintenance.ts`, inspect `ApiError.body` for the
  Storage 409 shape. Keep normal failures as ordinary action errors, but expose a structured busy
  state and a `runAnyway` action that repeats the original resource selection with `force: true`.

### Storage page and mobile contract

- Update `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx` to replace
  the generic busy alert with a plain-language list of the reported activity labels, a disruption
  warning, and a distinct **Run anyway** button when `force_available` is true. An existing
  maintenance run remains informational with no override control.
- Desktop keeps the busy feedback directly below the existing Analyze/Run-now action bar. On phones,
  the existing one-column Storage actions layout remains the nearest shipped exemplar: the warning
  and full-width, 44px **Run anyway** action stack inline with the page, need no drawer or dialog,
  preserve the document as the single scroll owner, and retain zero horizontal overflow.

## Tests

- **Activity and operation semantics:** force cleanup runs beside existing task activity, cannot run
  beside maintenance, and later task work still cancels it. **Files:**
  `apps/backend/internal/agent/runtime/activity/coordinator_test.go`,
  `apps/backend/internal/system/storage/operations_test.go`, and `runner_test.go`.
  **How:** deterministic lease/channel tests plus existing tracked-job tests.
- **HTTP contract:** the Storage route returns labeled blockers and availability on 409, accepts
  `force: true`, and does not change the ordinary 202 job response. **Files:**
  `apps/backend/internal/system/storage/handler_test.go` and `storage_routes_test.go`.
- **Controller and rendered feedback:** the hook preserves resource selection after a structured
  409, sends force only from **Run anyway**, and the settings page renders warning/list/button only
  when supplied by the backend. **Files:**
  `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`,
  `apps/web/components/settings/system/storage/storage-maintenance-settings.test.tsx`, and
  `apps/web/lib/api/domains/system-api.test.ts`.

## E2E Tests

- **Scenario:** a running task blocks manual cleanup. **File:**
  `apps/web/e2e/tests/system/storage-maintenance.spec.ts`. **Verify:** the page identifies the
  active work, exposes **Run anyway**, sends `{ force: true }`, and receives a normal cleanup job.
- **Scenario:** the same busy feedback is usable on Pixel 5. **File:**
  `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`. **Verify:** the warning and
  override remain visible/tappable, start the cleanup, and leave document horizontal overflow at
  zero.

## Implementation Waves

Wave 1:

- [x] [Task 01: Backend activity and API contract](task-01-backend-activity-api.md)

Wave 2:

- [x] [Task 02: Storage busy feedback and override UI](task-02-frontend-busy-override.md)

Wave 3:

- [x] [Task 03: Desktop and mobile override coverage](task-03-e2e-busy-override.md)

## Risks

- A forced cleanup can disrupt the active work it names; the inline warning must remain explicit
  and the feature must never silently retry with force.
- Bypassing an active task must not become a bypass for an existing maintenance run; the latter
  remains an unavailable override.
- The activity coordinator currently knows categories rather than user/task identity. The API must
  stay at that privacy-safe category granularity unless a later, separately designed feature adds
  activity identity.

## Validation Results

- Backend activity/storage/system race tests: passed (107 tests).
- Backend backendapp/storage/system integration tests: passed (312 tests).
- Web Storage unit tests: passed (57 tests).
- Web typecheck: passed.
- Managed desktop/mobile Storage E2E: passed (7 tests).
- Commit hooks and GitHub PR checks: passed.
