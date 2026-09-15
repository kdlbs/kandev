---
created: 2026-09-01
status: implemented
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation plan: Restored transcript pagination recovery

## Overview

Repair older-history pagination when a persistent Dockview chat is initialized
while inactive and later becomes visible at its restored top edge. The fix will
turn the existing panel-visibility signal and hard-top input into guarded
current-geometry checks, without changing backend cursors, eager-opening
history, or explicit failure recovery.

## Scope

### In scope

- Pass real chat-host visibility into the native transcript lifecycle.
- Re-evaluate the oldest-page sentinel once after a hidden Dockview transcript
  becomes visible and its saved scroll offset is restored.
- Accept upward input at `scrollTop === 0` as fresh pagination intent even when
  the browser emits no `scroll` event.
- Reuse current sentinel geometry, serialization, session-epoch, and recovery
  guards for both paths.
- Add focused unit regressions and a desktop restored-secondary-tab browser
  regression.
- Retain mobile pagination, anchor, and recovery behavior through the shared
  native transcript path.

### Out of scope

- Backend message queries, pagination cursors, ordering, persistence, or page
  sizes.
- Eagerly loading older history when a task opens away from the oldest loaded
  edge.
- Retrying rejected or zero-progress requests on panel activation.
- Changes to Prompt History pagination, Dockview layout persistence, or chat
  scroll ownership.
- New user-facing copy or a new mobile composition.

## Confirmed root cause

The affected secondary session is restored as an inactive Dockview panel. Its
message request succeeds with 100 newest rows, `has_more: true`, and a valid
oldest cursor. The backend also returns older rows when that cursor is supplied.

The inactive transcript mounts behind a zero-size panel. When it becomes
visible already at `scrollTop = 0`, its `IntersectionObserver` does not deliver
a fresh eligible entry. The fallback in
`useRetryPaginationOnUpwardScroll` requires
`nextScrollTop < previousScrollTop`; at the hard top that comparison is
`0 < 0`, so another upward gesture cannot invoke the sentinel retry. The
background newest-window fetch then repeats the same 100 rows and cursor while
the older-page path is never called.

## Technical approach

### Visibility-aware sentinel recovery

- Extend the message-list contract with the existing `TaskChatPanel`
  visibility state and carry it into native scroll management.
- On a hidden-to-visible transition, wait for the panel's existing saved-offset
  restoration to commit before issuing one lifecycle recheck.
- Make the shared sentinel recheck current root/sentinel geometry instead of
  trusting an intersection sample captured while hidden.
- Fire only when visible pagination remains, initial/refetch loading is clear,
  the sentinel is inside preload, no request is active, the session is current,
  and transcript recovery is not active.
- Keep the first visible mount bounded: a lifecycle recheck does not paginate
  unless the restored viewport is already at the oldest loaded edge.

### Hard-top user intent

- Observe directional wheel, keyboard, touch, and scroll movement at the
  transcript scroll owner.
- Treat an upward action at the hard top as fresh intent even when scroll
  position does not change.
- Route the action through the same current-geometry sentinel recheck and
  programmatic-scroll guard.
- Coalesce one physical gesture so it cannot start duplicate cursor requests.

### Diagnostics and failure behavior

- Retain the existing bounded pagination diagnostics so the cursor-based older
  request remains distinguishable from newest-window running backfill.
- Preserve explicit retry for rejected or zero-progress requests; activation
  must not create an automatic retry loop.
- Preserve anchor restoration and continuation stop reasons after a positive
  older page commits.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5` to `.9` | Existing sentinel and native transcript suites plus new current-geometry and hard-top regressions |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10` | Existing no-eager-open browser assertion remains green |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.11` | Existing mobile pagination suite remains green after the shared-path change |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.12` | Existing inactive-session reconciliation coverage remains green |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.14` | New hidden-to-visible unit regression and restored secondary-session desktop browser flow |

The red-first unit case mounts a transcript hidden with `hasMore: true`, makes
it visible at restored `scrollTop = 0`, and expects exactly one older-page
request after the restore frame. A second case dispatches upward input at hard
top without changing `scrollTop` and expects the guarded retry path to run.

## E2E tests

- Desktop Chromium: extend
  `apps/web/e2e/tests/chat/message-pagination.spec.ts` with two seeded sessions
  in one restored Dockview group. Keep the target session inactive during
  hydration, suppress the fresh observer entry that can be missed during
  restoration, activate its tab, supply upward input at the hard top, and
  assert that the next request includes the previous oldest cursor and reveals
  older content.
- Keep the watcher capable of distinguishing newest-window refreshes from
  `before=<cursor>` older-page requests. Assert that activation starts one
  cursor advance, not repeated newest-window backfill.
- Mobile Chrome: rerun
  `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts` as regression
  evidence for shared sentinel, anchor, and recovery behavior. No new mobile
  layout is required because the defect depends on inactive Dockview tabs.

## Mobile design contract

- Desktop outcome: activating a restored secondary chat at its saved top edge
  resumes older-history pagination without scroll-away and scroll-back.
- Mobile entry point: the existing full-height Chat tab.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the existing
  mobile message-pagination specification.
- Hierarchy and surface: the transcript remains the only vertical scroll
  owner; no sheet, drawer, or compact desktop tab is added.
- Shared logic: visibility and hard-top intent use the native transcript's
  sentinel guards. Mobile supplies mounted visibility rather than Dockview
  activation.
- Mobile proof: the existing mobile long-history, prompt boundary, recovery,
  and prepend-anchor cases remain green.

## Work orders

- [x] [Task 01: Recover pagination after transcript activation](task-01-recover-pagination-after-activation.md) (`done`)

## Verification results

- Focused Vitest: 87 tests passed across the shared sentinel, native transcript,
  and lazy-message pagination suites.
- TypeScript typecheck, targeted ESLint, targeted Prettier, and the complete spec
  linter passed.
- Desktop Chromium: all eight message-pagination cases passed, including the
  restored inactive-session hard-top regression.
- Mobile Chrome: all eight mobile message-pagination cases passed, including
  the hard-top touch recovery regression.
- A fresh desktop PR screenshot was captured after the restored secondary chat
  loaded its older marker.

## Risks

- Running the lifecycle check before `SessionPanelContent` restores scroll can
  read zero-size or transient geometry and either miss or spuriously load.
- Treating every resize or active render as intent can duplicate requests or
  create an automatic loop.
- Direct changes to the shared sentinel can regress Prompt History's
  bottom-pinned append behavior.
- Wheel, keyboard, scrollbar, and touch signals need coalescing while retaining
  true hard-top intent.
- The browser fixture must reproduce restored inactive tabs, not only switch
  between two already-active panels.
