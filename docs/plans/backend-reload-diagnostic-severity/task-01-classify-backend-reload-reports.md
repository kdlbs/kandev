---
id: "01-classify-backend-reload-reports"
title: "Classify backend-reload reports"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
acceptance_criteria:
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.3
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.5
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.17
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.18
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
  - ../../specs/platform/system-design/expected-runtime-log-severity.md
---

# Task 01: Classify Backend-Reload Reports

## Summary

Add a distinct source for expected backend-reload reports. Record this source
at info level while actual error-toast reports remain errors.

## In scope

- Add the `backend-reload` source to the frontend and backend contracts.
- Restrict the source to the two stable reload signals.
- Omit synthetic error-toast stacks and error objects from reload reports.
- Add backend observer tests and frontend payload tests.

## Out of scope

- Change the restart detector, reload coordinator, or reload alert.
- Rename the existing endpoint.
- Reclassify any other frontend or backend event.

## Acceptance

- A valid backend-reload report creates one info entry named
  `frontend backend reload required` and no error entry.
- Sonner and legacy error-toast reports keep their current error entry.
- The frontend sends one reload report with browser context and no synthetic
  toast-error data.

## Verification

```bash
cd apps/backend
go test ./internal/system/frontenderrors -count=1
```

```bash
cd apps/web
pnpm exec vitest run lib/api/domains/frontend-error-log-api.test.ts lib/platform/backend-reload-coordinator.test.ts hooks/domains/system/use-backend-generation-guard.test.ts
```

```bash
cd apps/web
pnpm run typecheck
```

## Files likely touched

- `apps/backend/internal/system/frontenderrors/handler.go`
- `apps/backend/internal/system/frontenderrors/handler_test.go`
- `apps/web/lib/api/domains/frontend-error-log-api.ts`
- `apps/web/lib/api/domains/frontend-error-log-api.test.ts`
- `apps/web/src/main.tsx`

## Dependencies

None.

## Risks

- The backend must validate the source and signal before it selects info level.
- Existing toast reports must keep their message, fields, and error severity.

## Parallelism

`sequential`

## Inputs

- The requirements and system designs in the frontmatter.
- The observer-backed frontend-error handler tests.
- The backend-reload coordinator and generation-guard tests.

## Results

- RED: the backend rejected `backend-reload` with HTTP 400.
- RED: the frontend added the synthetic toast stack and error object.
- RED: the incomplete allow-list accepted invalid recovery reports and rejected
  `settings_interlock_rejected`.
- GREEN: the backend package passed 17 tests.
- GREEN: the three focused frontend files passed 20 tests.
- GREEN: the web typecheck passed.
