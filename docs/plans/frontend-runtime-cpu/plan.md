---
created: 2026-08-27
status: completed
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
system_design:
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/browser-console-retention.md
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Reduce Frontend Runtime CPU

## Overview

Reduce avoidable browser CPU work during active Kandev sessions. Keep the
current diagnostic content, transcript data, event ordering, and scroll
behavior.

The package targets three app-owned costs from the attached 5.779 second Chrome
performance capture:

- Repeated debug-log preparation and IndexedDB drain work.
- One Zustand transaction for each message key in one animation frame.
- A forced content-size layout read for each pinned transcript update.

## Scope

### In scope

- Coalesce browser-log persistence over the documented 250 ms window.
- Reuse one detached entry and exact byte count through browser-log storage.
- Bound the growing `messages:process` debug summary to one latest sample per
  250 ms.
- Apply one frame of replacement message updates in one store transaction.
- Keep message add, delete, and turn-settle events as ordered barriers.
- Place an enabled pinned transcript at the bottom without reading its content
  size in the message-commit path.
- Add deterministic work-count tests and desktop and mobile scroll evidence.

### Out of scope

- Removing any captured console level or changing diagnostic retention limits.
- Changing backend stream coalescing, WebSocket protocol payloads, or database
  persistence.
- Transcript virtualization, message-grouping changes, or a new state library.
- Changes to transcript controls, copy, layout, or persisted preferences.
- Optimizing React DevTools or the Kandy plugin. They are separate runtime
  contributors outside the Kandev host implementation.
- A fixed CPU-time CI threshold. Shared runners make wall-clock samples too
  variable for a stable correctness gate.

## Confirmed root cause

The renderer main thread was idle for 4.963 seconds of the 5.767 second renderer
window. App work was bursty, not a continuous busy loop. After excluding the
236.434 ms profiler-start task, renderer CPU was about 498 ms, or 8.6% of one
core during the capture.

The trace contained 47 `V8Console::Debug` calls. Samples attributed about 35 to
40 ms to browser-log append, drain scheduling, callback, and encoding work.
The current path can encode one entry in `buildLogEntry`, `RingBuffer.push`,
`stageLogEntry`, and `IndexedDBLogStore.append`. An idle callback can also run
before the documented 250 ms collection opportunity, which turns a burst into
small transactions.

The WebSocket scheduler already keeps the newest update per message for one
animation frame. Its `flush()` loop still calls `updateMessage()` once for each
message key. Every call opens a separate Zustand and Immer transaction and can
notify transcript selectors. Derived message processing then repeats for those
notifications.

Ten message commits caused ten `scrollHeight` reads followed by `scrollTop`
writes in the pinned transcript path. These reads initiated about 5.643 ms of
style recalculation and 2.893 ms of layout in the capture.

React DevTools added about 48 ms of sampled JavaScript and initiated separate
style and layout work. Kandy celebration animation callbacks added about 6 ms.
These values remain measurement context, not Kandev host fixes.

## Requirement conformance

The fixes preserve accepted product behavior. They refine resource boundaries
that the current implementation does not fully enforce.

- Diagnostic bundles still retain `console.debug`, `console.info`,
  `console.warn`, and `console.error` with the same identity, size, and age
  limits.
- Final transcript content remains lossless. Replacement updates can still
  skip only intermediate render states.
- Message add, delete, and turn-settle actions keep their ordering guarantees.
- Enabled auto-scroll stays pinned. Disabled auto-scroll stays frozen.
- Desktop and mobile use the same store and native transcript behavior.

No ADR is required. The package keeps the accepted privacy, persistence,
transport, and UI boundaries. It changes internal scheduling and mutation
granularity only.

## Technical approach

### Browser diagnostics

- Replace the idle-first drain trigger with one collection gate followed by
  bounded idle scheduling. The first staged entry starts a 250 ms timer. When
  it closes, the runtime requests browser idle time with the remaining
  one-second deadline and keeps a cancellable timer fallback. Snapshot
  collection cancels both waits and joins that drain.
- Prepare an accepted log entry once after adding `identity_scope`. Reuse its
  detached object and exact UTF-8 byte count in the memory buffer, staging
  limits, IndexedDB record, and retention totals.
- Keep the 50-entry, 256 KiB transaction bounds and the 500-entry, 2 MiB
  staging bounds.
- Make `useDebugProcessedPipeline` keep the latest inputs in a ref. Compute and
  emit its growing collection summary at most once per 250 ms, flushing the
  pending latest sample on session change and unmount.

### Message frame transaction

- Add a `updateMessages(messages)` session-store action.
- Convert and settle all accepted frame payloads before the store mutation.
- Apply the array of replacements in one Immer callback. Preserve first-key
  insertion order from the scheduler map.
- Keep `updateMessage(message)` as the single-message API for existing callers.
  Share one internal merge helper so both actions keep partial-field behavior.
- Flush the batch before add and delete barriers, as the current scheduler
  does.

### Layout-free bottom placement

- Add one native transcript helper that writes a maximum scroll offset.
- Use it for work-start and pinned message updates.
- Do not read `scrollHeight`, `clientHeight`, rectangles, or computed style in
  those common paths.
- Keep geometry reads for user scroll detection, catch-up decisions,
  pagination, and prepend restoration. Those reads answer different questions
  and do not run for every streamed commit.

## Test mapping

| Acceptance criterion                             | Test evidence                                                                                                         |
| ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| `AC-PLATFORM-DIAGNOSTIC-LOGGING-001.9`           | Fake-time hook test proves that continuous message updates emit no more than one latest debug summary per 250 ms.     |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.5`    | Runtime test holds one append and proves maximum append concurrency remains one.                                      |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.8`    | Fake-time runtime test stages a burst and proves idle availability cannot create early one-entry transactions.        |
| `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.9`    | Runtime test proves a snapshot bypasses the collection timer and waits for the shared drain.                          |
| `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.7` | Existing scheduler tests prove newest-payload replacement, stale rejection through the bulk path, and semantic barriers. |
| `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.9` | Scheduler and real-store tests prove that several message keys cause one bulk action and one subscriber notification. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.1`             | Existing unit and browser tests prove that disabled auto-scroll keeps its captured position.                          |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.2`             | Existing unit and browser tests prove catch-up after re-enable.                                                       |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.5`             | Desktop and mobile browser tests prove enabled transcripts remain at the bottom.                                      |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.7`             | Unit test fails on any content-size read in the pinned append path and records one maximum-offset write.              |

## E2E tests

- Desktop: extend `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` in the
  `chromium` project.
- Mobile: extend
  `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts` in the
  `mobile-chrome` project.
- Shared cues: enabled bottom distance, disabled `scrollTop`, incoming live
  message visibility, and the existing native transcript scroll owner.

Logger and store work use deterministic Vitest counters. They do not require a
browser timing threshold.

## Mobile design contract

- Desktop outcome: the native transcript follows live content with fewer
  layout and store operations.
- Mobile entry point: the existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the shipped
  mobile auto-scroll tests.
- Hierarchy and surface: the transcript remains the only vertical scroll owner.
- Shared logic: desktop and mobile use one frame scheduler, session store, and
  bottom-placement helper.
- Mobile proof: the mobile test covers enabled bottom pinning and disabled
  position freezing after a live message arrives.

## Work orders

- [completed] [Task 01: Coalesce Browser Diagnostic Work](task-01-coalesce-browser-diagnostic-work.md)
- [completed] [Task 02: Batch Message Frame Mutations](task-02-batch-message-frame-mutations.md)
- [completed] [Task 03: Remove Pinned-scroll Layout Reads](task-03-remove-pinned-scroll-layout-reads.md)

## Verification protocol

Each work order starts with its named failing regression and runs its targeted
tests before broader checks.

After all work orders, capture the same interaction twice with browser
extensions disabled:

1. Use a production-like build to measure app-owned streaming and scroll work.
2. Use debug mode to measure the bounded diagnostics path.

Compare app-originated console work, IndexedDB append count, Zustand subscriber
notifications, forced layout initiators, and renderer CPU. Keep React DevTools
and Kandy plugin samples separate from host results.

The deterministic acceptance budgets are:

- One browser-log drain start for entries inside one 250 ms collection window.
- One store transaction and subscriber notification per message frame.
- Zero content-size reads in the pinned message-commit path.
- Unchanged final logs, transcript content, event order, and scroll outcomes.

## Risks

- Timer cancellation can lose a pending browser-log drain during test resets or
  snapshot capture.
- Reusing a byte count before identity scoping can make retention totals wrong.
- Bulk store mutation can change barrier order if payload conversion moves
  across the barrier.
- A shared message merge helper can clear fields that an omitted partial field
  must preserve.
- A maximum scroll write can run while another scroll owner is active if the
  existing guards are bypassed.
- Browser extensions can hide improvements or create false regressions in a
  comparison profile.
