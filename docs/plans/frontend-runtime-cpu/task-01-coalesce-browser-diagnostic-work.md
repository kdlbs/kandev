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

Collect browser logs for the full 250 ms window, then schedule persistence
during browser idle time with a bounded deadline. Prepare each accepted entry
once, then reuse it through memory and IndexedDB storage. Bound the growing
message-pipeline debug summary to the same cadence and flush its final sample
when a session ends.

## Failing regression first

Add fake-time tests that stage a burst, prove that an idle callback cannot run
before 250 ms, then prove that post-window idle time starts one bounded append.
Also prove that the remaining deadline fallback drains when idle time does not
arrive, and that snapshot flush cancels both waits.

Add a hook regression named `emits the latest processed-message debug sample
at most once per 250 ms`. Re-render the hook several times inside one window
and prove that only the latest counts are formatted and emitted.

## In scope

- Add a cancellable 250 ms collection gate to the logger runtime.
- Schedule the serialized drain during browser idle time after the gate, with a
  one-second overall deadline and cancellable fallback.
- Make snapshot flush bypass and cancel the collection and idle waits.
- Keep one in-flight append and the current batch and staging limits.
- Introduce one prepared-entry shape with detached data and exact bytes.
- Reuse prepared entries in the ring buffer and IndexedDB store.
- Coalesce `messages:process` derived debug work to one latest sample per
  window.
- Flush a pending processed-message debug sample on session change and unmount.
- Preserve capture levels, identity partitioning, loss counts, and fallback.

## Out of scope

- Removing debug logs or changing bundle contents.
- Changing retention age, entry count, byte limits, or database name.
- Changing backend diagnostics or bundle transport.

## Acceptance

- An idle callback cannot split one collection window into one-entry writes.
- Post-window browser idle time starts persistence, and the bounded deadline
  fallback starts it when idle time does not arrive.
- A snapshot starts and joins the serialized drain without waiting 250 ms.
- One accepted entry has one canonical exact byte count across all stores.
- A continuous processed-message stream creates at most four derived debug
  samples per second and retains the latest sample.
- Persistence failure still switches to the bounded memory fallback.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/logger/buffer.test.ts lib/logger/intercept.test.ts lib/logger/runtime.test.ts lib/logger/indexeddb-store.test.ts hooks/use-processed-messages.test.ts hooks/use-processed-messages-fallback.test.ts
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
- A stale idle callback or fallback timer can start a second drain after a
  snapshot.
- A prepared entry can become mutable if the memory buffer exposes its object.
- Byte totals can drift if the identity scope changes after preparation.
- Cleanup can lose a trailing debug sample unless it emits the latest state
  before cancelling its timer.

## Parallelism

`sequential`

## Inputs

- Browser CPU trace findings in `plan.md`.
- Browser console retention requirements and system design.
- Diagnostic logging performance contract.

## Results

Implemented the 250 ms collection gate followed by browser-idle scheduling with
a bounded one-second overall deadline and a timer fallback. Snapshot flush
cancels both waits and joins the serialized drain. Prepared-entry reuse,
serialized IndexedDB append, and latest-sample coalescing remain in place; a
pending processed-message sample now flushes on session change and unmount.
Persistence failure still degrades to the bounded memory buffer, and retention
limits are unchanged.

Targeted verification passed across the logger buffer, interceptor, runtime,
IndexedDB store, and processed-message hook suites, including the idle-gate,
deadline, snapshot-cancellation, session-change, and unmount regressions.
