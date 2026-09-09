---
created: 2026-09-08
updated: 2026-09-09
status: done
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
system_design:
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
  - ../../specs/platform/system-design/browser-console-retention.md
legacy_specs: []
---

# Implementation Plan: Reduce Frontend Log Capture Latency

## Overview

Review the available capture evidence before the implementation starts. Then
make capture work finite and incremental. The implementation freezes one
capture watermark and gives its persistence flush one second.

It then uploads one chronological page before it reads the next page. The
backend supplies a relative duration for a monotonic local deadline.

## Scope

### In scope

- Phase timing for the frontend capture and backend upload handler.
- A fixed staging watermark and a one-second persistence-flush limit.
- Receipt-time memory fallback that does not stop normal persistence.
- Chronological paging through the existing IndexedDB timestamp index.
- A 128 KiB first page and the existing 800 KiB target for later pages.
- A relative capture duration and an abort signal for the active upload.
- Regression tests for ordering, bounds, clock skew, timeout, and fallback.

### Out of scope

- An IndexedDB schema change or a new write index.
- Changes to console levels, producer frequency, retention, or staging limits.
- Continuous frontend telemetry or background upload retries.
- A longer backend collection window or a larger request-body limit.
- A backend lock refactor without timing evidence from Task 01.
- User-facing UI or copy changes.

## Evidence and attribution gap

The backend sent both observed capture notifications immediately. The first
frontend request reached the server 11.6 seconds later. The second request
reached the server 14.8 seconds later.

The server then spent 24.8 seconds and 16.2 seconds in those requests. The HTTP
timer includes request-body decoding. The handler decodes the body before it
enters the diagnostic bundle service lock.

An isolated Chromium benchmark used 10,000 entries with approximately 20 MiB
of data. The current `getAll`, sort, and chunk preparation took 1.08 seconds on
the first read. Four later reads took 0.20 to 0.54 seconds.

The eager snapshot is a bounded cost, but this benchmark did not reproduce the
reported delay. The current evidence does not separate these possible costs:

- The unbounded staging flush.
- IndexedDB contention in the active browser profile.
- Request-body serialization and transfer.
- Backend lock wait and file writes.

Task 01 records these phases before Task 02 starts. The phrase "confirmed root
cause" does not apply until this table is complete.

## Requirement conformance

This package amends the existing platform requirements. It does not create a
new capability owner.

- Console calls remain synchronous and do not wait for persistence or network
  work.
- A capture includes only entries that exist at notification receipt.
- Later console entries do not extend the capture flush.
- Identity isolation, chronological order, and all retention limits remain.
- The backend remains authoritative for job expiry.
- The relative duration removes dependence on the browser wall clock.
- A partial capture does not retry or write through the console interceptor.

No new ADR is necessary. The privacy, ownership, persistence, and transport
boundaries remain the same. The added relative duration is backward compatible.

## Technical approach

### Phase measurement

Task 01 adds temporary, non-console timing around each capture phase. Browser
timing uses `performance.now()`. Backend timing uses the existing structured
logger. The task removes this temporary code after it records the results.

The frontend phase table includes these intervals:

1. Notification receipt to flush start.
2. The current persistence flush.
3. The IndexedDB request.
4. Snapshot filtering and ordering.
5. Chunk preparation and request-body serialization.
6. Each upload request.

The backend phase table includes request-body decoding, stream claim, service
lock wait, validation, file writes, and response completion. Task 01 compares
an active profile with an isolated maximum-size profile.

The frontend intervals do not overlap. Their total covers notification receipt
through the fetch response. The backend intervals subdivide the server request
and do not add to the frontend total.

If backend lock wait or file writes exceed either threshold, Task 01 stops the
package. The thresholds are 250 milliseconds and 10 percent of request time.
Then update this package with a backend work order before Task 02 starts.

### Fixed capture watermark

In `apps/web/lib/logger/runtime.ts`, assign a local sequence number to each
staged entry. Each drain request owns a fixed maximum sequence number. Entries
that arrive later start a later drain.

At notification receipt, save the current sequence number and a bounded memory
snapshot. Join the active drain and persist only through that sequence number.
Wait at most one second. If the wait expires, use the saved memory snapshot and
let persistence continue. Report memory storage for this capture and include
`flush_timeout: true` in its final metadata.

### Existing-index snapshot pages

In `apps/web/lib/logger/indexeddb-store.ts`, keep database version 2. Read the
existing `timestamp_ms` index with a continuation pair of timestamp and primary
key. Filter the requested identity while the cursor scans the globally bounded
store.

Return one page and close its readonly transaction before the uploader asks for
the next page. Use stored prepared-byte counts for page limits. Do not use
`getAll` or sort the complete identity partition.

### Upload budget and page sizes

In `apps/backend/internal/system/logbundle/service.go`, add
`capture_timeout_ms` to `system.logs.capture_requested`. Calculate it from the
backend clock when the notification is created. Keep `capture_deadline` for
backend authority and compatibility.

In `apps/web/lib/logger/capture.ts`, use `performance.now()` to create the local
deadline. Use the absolute deadline only when an older backend omits the
duration. Check the local deadline before each page and upload.

Use 128 KiB of entry data for the first page. Use the existing 800 KiB target
for later pages. This change adds at most one request to a maximum-size capture.
Use an `AbortController` to cancel the active request when the local budget
expires.

If no production caller remains, remove `chunkEntries`. Replace its direct test
with tests for the streaming capture behavior.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-PLATFORM-DIAGNOSTIC-LOGGING-001.10` | `capture.test.ts` holds the second page and proves that the first upload starts first. |
| `AC-PLATFORM-DIAGNOSTIC-LOGGING-001.11` | Backend notification tests cover both deadline fields. Frontend fake-time tests prove monotonic behavior with wall-clock skew. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.1` | Existing store tests retain the three-day, 10,000-entry, and 20 MiB limits. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.3` | Existing concurrent-store tests retain shared transaction totals. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.5` | Runtime tests prove that persistence uses one active drain. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.6` | Runtime tests prove that capture fallback does not change persistence mode or erase staging. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.9` | Runtime tests freeze a sequence watermark and add later entries while the drain is held. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.10` | Store tests cover timestamp ties, identity filtering, continuation, page bounds, and bounded cursor visits. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.11` | Fake-time runtime tests prove the one-second memory fallback and continued persistence. |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.12` | Capture tests prove that budget expiry stops new pages and cancels an active upload. |

## Work orders

- [x] [Task 01: Measure Capture Phases](task-01-measure-capture-phases.md)
- [x] [Task 02: Bound and Stream Capture](task-02-bound-and-stream-capture.md)

Task 02 uses the available phase evidence. The active-profile phase table could
not be reproduced because this workspace had no attached browser session.

## Verification results

- `pnpm install --frozen-lockfile` completed from `apps`.
- The focused frontend logger suite passed: 4 files and 37 tests.
- `pnpm run typecheck` passed from `apps/web`.
- The log-bundle package tests passed: 23 tests.
- The log-bundle package race tests passed: 23 tests.
- The focused frontend ESLint and Prettier checks passed.
- `gofmt -l` reported no changed Go files, and `git diff --check` passed.
- The repository-wide backend test target was run twice. Both runs failed in
  unrelated process-probe, configuration-discovery, launcher, and Office
  migration tests. The first run also used the workspace home configuration.

## Risks

- A timestamp cursor can skip or duplicate entries when timestamps are equal.
- A slow active write can continue after capture selects its memory fallback.
- A delayed notification can leave less server time than the relative client
  budget. The backend can reject the late request.
- A 128 KiB first request can still exceed the budget on a very slow link.
- Request-body transfer can remain the largest cost after frontend paging.

## Mobile parity

This change does not alter layout, navigation, touch behavior, scrolling, or
viewport-specific controls. Desktop and mobile browsers use the same logger
runtime, capture protocol, and tests. Focused unit and protocol tests are
sufficient. No new mobile Playwright test is necessary.

## Documentation impact

No public documentation changes are necessary. The visible bundle workflow is
unchanged. The platform requirements and designs own the changed contract.
