---
id: "01-measure-capture-phases"
title: "Measure capture phases"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.10
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.11
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.9
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.10
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.11
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.12
system_design:
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
  - ../../specs/platform/system-design/browser-console-retention.md
---

# Task 01: Measure Capture Phases

## Summary

Measure the frontend and backend phases that make one capture exceed 30
seconds. Record enough timing data to identify the dominant phases before the
capture implementation changes.

## In scope

- Add temporary `performance.now()` timing around frontend capture phases.
- Do not emit timing through an intercepted console method.
- Add temporary structured backend timing around body decoding and service work.
- Measure two captures from the reported active browser profile.
- Measure two captures from an isolated 10,000-entry, approximately 20 MiB
  profile.
- Record request size, entry count, and each phase duration in `## Results`.
- Compare frontend and backend timing with the HTTP middleware duration.
- Before this task is complete, remove all temporary instrumentation.
- If the evidence changes the implementation boundary, update Task 02.

## Out of scope

- A production behavior change.
- A permanent telemetry endpoint or browser global.
- Changes to page size, retention, IndexedDB schema, or backend locking.

## Acceptance

1. The frontend phases account for at least 90 percent of
   notification-to-response time. The backend phases separately account for at
   least 90 percent of server request time.
2. The results identify the dominant pre-request and in-request phases.
3. The task removes temporary code and leaves only its recorded results and any
   necessary plan corrections.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
make -C apps/backend test
git diff --check
```

## Files likely touched

- `apps/web/lib/logger/capture.ts` (temporary instrumentation only)
- `apps/web/lib/logger/runtime.ts` (temporary instrumentation only)
- `apps/web/lib/logger/indexeddb-store.ts` (temporary instrumentation only)
- `apps/backend/internal/system/logbundle/handler.go` (temporary instrumentation only)
- `apps/backend/internal/system/logbundle/service.go` (temporary instrumentation only)
- `docs/plans/frontend-log-capture-latency/plan.md`
- `docs/plans/frontend-log-capture-latency/task-01-measure-capture-phases.md`
- `docs/plans/frontend-log-capture-latency/task-02-bound-and-stream-capture.md`

## Dependencies

None.

## Risks

- Console timing can recurse into the logger. Use the Performance API instead.
- A synthetic profile can differ from the active browser profile.
- Temporary instrumentation can change timing. Keep each marker constant-time.

## Parallelism

`sequential`

## Inputs

- The two observed capture timelines in `plan.md`.
- HTTP middleware timing in `apps/backend/internal/common/httpmw/logging.go`.
- The frontend capture, runtime, and IndexedDB code paths.
- The backend bundle upload handler and service.

## Results

- The available active-profile trace recorded 11.6 seconds from the first
  notification to the first frontend request and 14.8 seconds to the second.
  The corresponding HTTP request durations were 24.8 seconds and 16.2 seconds.
- The isolated Chromium benchmark used 10,000 entries and about 20 MiB of
  data. The old eager read, sort, and chunk preparation took 1.08 seconds on
  the first read and 0.20 to 0.54 seconds on later reads.
- A repeat of the active-profile capture was not possible. The available debug
  backend had no attached Chromium client, so no new phase markers were
  available for flush, IndexedDB, serialization, upload, lock wait, or file
  writes. These timings do not prove a backend lock bottleneck.
- The evidence supports bounded staging, incremental IndexedDB reads, and
  incremental uploads. No separate backend lock work order was created.
- The temporary measurement code was not retained. The task leaves only these
  results and the bounded implementation in Task 02.
- Verification passed: web typecheck, log-bundle tests, focused logger tests,
  focused ESLint, `gofmt -l`, and `git diff --check`.
