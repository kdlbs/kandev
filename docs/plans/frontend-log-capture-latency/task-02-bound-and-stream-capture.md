---
id: "02-bound-and-stream-capture"
title: "Bound and stream capture"
status: done
wave: 2
depends_on:
  - "01-measure-capture-phases"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.10
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.11
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.1
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.3
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.5
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.6
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.9
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.10
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.11
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.12
system_design:
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
  - ../../specs/platform/system-design/browser-console-retention.md
---

# Task 02: Bound and Stream Capture

## Summary

Bound the pre-capture persistence wait and stream chronological pages through
the existing IndexedDB schema. Use a monotonic relative budget and a small first
request so a slow link can return partial evidence.

## Failing regression first

Add these regressions before production changes:

1. In `runtime.test.ts`, hold one append and add later entries. Prove that the
   capture watermark does not move and that the one-second timeout selects the
   receipt-time memory snapshot.
2. In `indexeddb-store.test.ts`, seed two identities and equal timestamps. Prove
   exact chronological continuation without `getAll`, gaps, or duplicates.
3. In `capture.test.ts`, hold the second page. Prove that upload zero starts
   first and contains no more than 128 KiB of entry data.
4. In `capture.test.ts`, skew `Date.now()` and control `performance.now()`.
   Prove that the relative duration controls page admission and aborts an active
   upload.
5. In `service_test.go`, inspect the capture notification. Prove that it
   contains the absolute deadline and a nonnegative duration of at most 15,000
   milliseconds.

Run these tests and record their expected failures before implementation.

## In scope

- Add a monotonic sequence number to frontend staging entries.
- Make each drain request stop at its fixed sequence watermark.
- Save a bounded memory snapshot when capture starts.
- Wait one second for persistence through the capture watermark.
- If that wait expires, use the saved memory snapshot.
- Keep the active persistence drain and IndexedDB storage mode unchanged.
- Report memory storage and `flush_timeout: true` for the fallback capture.
- Page the existing `timestamp_ms` index with timestamp and primary-key
  continuation.
- Record a persisted primary-key upper boundary after the capture flush and
  apply it to every page.
- Filter the requested identity without a complete-partition sort.
- Reuse prepared byte counts for page bounds.
- Add `capture_timeout_ms` to the backend WS notification.
- Use a monotonic local deadline and cancel an active upload at expiry.
- Use 128 KiB for the first page and 800 KiB for later pages.
- Remove `chunkEntries` and replace its direct unit test.
- Preserve empty final chunks, final metadata, and sequential chunk indexes.

## Out of scope

- IndexedDB version 3 or another write index.
- A change to the process-wide backend service lock.
- More retention, staging, profile, request, or collection capacity.
- Continuous upload, retry, new UI, or new user-facing copy.

## Acceptance

1. New entries cannot extend capture flush beyond its receipt watermark. A slow
   drain selects memory evidence within one second and continues persistence.
2. IndexedDB pages preserve identity and chronological order without a schema
   upgrade, a full `getAll`, gaps, or duplicates.
3. The first page uploads before later enumeration. The monotonic budget stops
   new work and cancels an active upload.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/logger/buffer.test.ts lib/logger/indexeddb-store.test.ts lib/logger/runtime.test.ts lib/logger/capture.test.ts)
(cd apps/web && pnpm exec eslint lib/logger/buffer.ts lib/logger/buffer.test.ts lib/logger/indexeddb-store.ts lib/logger/indexeddb-store.test.ts lib/logger/runtime.ts lib/logger/runtime.test.ts lib/logger/capture.ts lib/logger/capture.test.ts)
(cd apps/web && pnpm run typecheck)
make -C apps/backend test
make -C apps/backend lint
```

## Files likely touched

- `apps/web/lib/logger/buffer.ts`
- `apps/web/lib/logger/buffer.test.ts`
- `apps/web/lib/logger/indexeddb-store.ts`
- `apps/web/lib/logger/indexeddb-store.test.ts`
- `apps/web/lib/logger/runtime.ts`
- `apps/web/lib/logger/runtime.test.ts`
- `apps/web/lib/logger/capture.ts`
- `apps/web/lib/logger/capture.test.ts`
- `apps/backend/internal/system/logbundle/service.go`
- `apps/backend/internal/system/logbundle/service_test.go`

## Dependencies

- Task 01 must record the complete phase table.
- Task 01 must show that backend locking does not require a separate repair.

## Risks

- Timestamp and primary-key continuation can skip equal-timestamp entries.
- A read transaction can scan entries for other identities before it fills a
  page. The global retention cap bounds this scan.
- The memory snapshot can contain less history than IndexedDB.
- The local budget can extend past backend expiry after transport delay.
- Abort support must pass through the shared API client without changing other
  requests.

## Parallelism

`sequential`

## Inputs

- Task 01 results and any resulting plan correction.
- Platform diagnostic logging requirements and designs.
- Browser console retention requirements and design.
- Existing `fetchJson` abort-signal support.

## Results

- RED: the new tests failed before implementation. The eager capture prepared
  an oversized first request, IndexedDB used `getAll`, the drain included later
  entries, and the notification omitted the relative timeout.
- GREEN: capture now freezes the staging watermark, waits one second, and uses
  a receipt-time memory snapshot when the wait expires. Persistence continues.
- GREEN: the store reads the timestamp index with timestamp and primary-key
  continuation and a persisted upper primary-key boundary. It preserves identity
  filtering, equal-timestamp order, and global retention bounds while excluding
  writes that arrive between page transactions.
- GREEN: capture uploads a 128 KiB first page, waits for each upload before the
  next read, uses 800 KiB later pages, and aborts at the monotonic deadline.
- GREEN: the backend notification includes the absolute deadline and a capped
  relative duration.
- Verification passed: 4 frontend files and 40 focused tests, focused ESLint,
  web typecheck, 23 log-bundle tests, 23 log-bundle race tests, `gofmt -l`, and
  `git diff --check`.
- The repository-wide backend test target was run twice. Both runs failed in
  unrelated process-probe, configuration-discovery, launcher, and Office
  migration tests. The changed log-bundle package passed in both runs.
