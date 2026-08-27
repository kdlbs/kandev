---
id: "01-coalesce-browser-diagnostic-work"
title: "Coalesce browser diagnostic work"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.9
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.5
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.8
  - AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.9
system_design:
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/browser-console-retention.md
---

# Task 01: Coalesce Browser Diagnostic Work

## Summary

Collect browser logs for the full 250 ms window. Prepare each accepted entry
once, then reuse it through memory and IndexedDB storage. Bound the growing
message-pipeline debug summary to the same cadence.

## Failing regression first

Add a fake-time test named `collects an idle log burst for 250 ms before one
bounded append`. Stage several entries, run an idle callback before 250 ms, and
prove that `IndexedDBLogStore.append` has not run. Advance to 250 ms and prove
that one append receives the burst.

Add a hook regression named `emits the latest processed-message debug sample
at most once per 250 ms`. Re-render the hook several times inside one window
and prove that only the latest counts are formatted and emitted.

## In scope

- Add a cancellable 250 ms collection gate to the logger runtime.
- Make snapshot flush bypass the collection wait.
- Keep one in-flight append and the current batch and staging limits.
- Introduce one prepared-entry shape with detached data and exact bytes.
- Reuse prepared entries in the ring buffer and IndexedDB store.
- Coalesce `messages:process` derived debug work to one latest sample per
  window.
- Preserve capture levels, identity partitioning, loss counts, and fallback.

## Out of scope

- Removing debug logs or changing bundle contents.
- Changing retention age, entry count, byte limits, or database name.
- Changing backend diagnostics or bundle transport.

## Acceptance

- An idle callback cannot split one collection window into one-entry writes.
- A snapshot starts and joins the serialized drain without waiting 250 ms.
- One accepted entry has one canonical exact byte count across all stores.
- A continuous processed-message stream creates at most four derived debug
  samples per second and retains the latest sample.
- Persistence failure still switches to the bounded memory fallback.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/logger/buffer.test.ts lib/logger/intercept.test.ts lib/logger/runtime.test.ts lib/logger/indexeddb-store.test.ts hooks/use-processed-messages.test.ts
cd apps && pnpm --filter @kandev/web run typecheck
cd apps/web && pnpm exec eslint lib/logger/buffer.ts lib/logger/buffer.test.ts lib/logger/intercept.ts lib/logger/intercept.test.ts lib/logger/runtime.ts lib/logger/runtime.test.ts lib/logger/indexeddb-store.ts lib/logger/indexeddb-store.test.ts hooks/use-processed-messages.ts hooks/use-processed-messages.test.ts
```

## Files likely touched

- `apps/web/lib/logger/buffer.ts`
- `apps/web/lib/logger/buffer.test.ts`
- `apps/web/lib/logger/intercept.ts`
- `apps/web/lib/logger/intercept.test.ts`
- `apps/web/lib/logger/runtime.ts`
- `apps/web/lib/logger/runtime.test.ts`
- `apps/web/lib/logger/indexeddb-store.ts`
- `apps/web/lib/logger/indexeddb-store.test.ts`
- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages.test.ts`

## Dependencies

None.

## Risks

- A stale timer can start a second drain after a snapshot.
- A prepared entry can become mutable if the memory buffer exposes its object.
- Byte totals can drift if the identity scope changes after preparation.
- A trailing debug sample can outlive its session unless cleanup cancels it.

## Parallelism

`sequential`

## Inputs

- Browser CPU trace findings in `plan.md`.
- Browser console retention requirements and system design.
- Diagnostic logging performance contract.

## Results

Implemented the 250 ms collection gate, prepared-entry reuse, serialized
IndexedDB append path, and latest-sample coalescing for the processed-message
debug summary. Snapshot flush still bypasses the collection wait, persistence
failure still degrades to the bounded memory buffer, and retention limits are
unchanged.

Targeted verification passed: 6 files and 83 tests across the logger buffer,
interceptor, runtime, IndexedDB store, and processed-message hook suites.
