---
created: 2026-08-30
status: implemented
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation plan: Continuous conversation history pagination

## Overview

Make the transcript distinguish initialized history from default empty state,
then use that state to suppress the task-description fallback until the true
conversation start is reached. After that boundary is trustworthy, replace
visible-row identity pagination stops with committed sentinel geometry and
show the manual loader only for recoverable failures.

## Scope

### In scope

- Record whether authoritative message pagination metadata initialized.
- Gate the synthetic task description on initialized, exhausted history.
- Continue upward pagination while the committed sentinel remains in preload.
- Keep prompt `#1` as the visible start.
- Show the explicit older-page control only after error or no progress.
- Preserve scroll anchors and raw search pagination.
- Prove the same outcome on desktop and mobile with tool-heavy history.

### Out of scope

- Backend queries, cursor fields, persistence, or message ordering.
- Eagerly loading history when a task opens.
- Rendering internal rows before prompt `#1`.
- Changing prompt-history panel behavior beyond shared state compatibility.
- New user-facing copy or a new mobile composition.

## Confirmed root cause

`shouldShowTaskDescriptionFallback` checks only the currently loaded visible
rows. A bounded newest window containing no user row therefore synthesizes the
task description while raw `has_more` is still true.

The native transcript stops a load cycle whenever an older page changes the
oldest visible boundary. Its retry path requires `scrollTop` to decrease, which
cannot happen while the reader is pinned at the top. The always-visible manual
loader becomes the only reliable continuation.

The reported running session placed the latest user prompt behind 516 newer
rows. The initial 100-row window plus 20-row older pages required 21 requests,
which made both defects visible.

## Technical approach

### Trustworthy transcript start

- Add an initialization bit to per-session message metadata and its merge,
  hydration, default, reconciliation, and purge paths.
- Expose initialized and raw-history state from `useSessionMessages`.
- Pass an explicit history-start condition into `useProcessedMessages`.
- Require initialized, exhausted history before synthesizing the task
  description when no stored user prompt is visible.
- Keep prompt ordinals and the visible `hasMore` boundary unchanged.

### Geometry-driven pagination and recovery

- Re-evaluate the sentinel against the scroll root and configured preload
  margin after page commit and prepend restoration.
- Continue positive loads while current geometry remains eligible, regardless
  of whether a standalone boundary key changed.
- Retain boundary keys for scroll anchoring and diagnostics, not continuation.
- Track a transcript-local recovery state for rejected or zero-progress loads.
- Render the existing localized older-page button only in recovery state and
  clear that state after progress, exhaustion, prompt `#1`, or session change.
- Give the recovery control a coarse-pointer 44-pixel hit area while retaining
  existing desktop density.

## Tests

| Acceptance criteria                                     | Evidence                                                                                         |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.1` to `.4` | `use-processed-messages-fallback.test.ts`, session slice/fetch tests                             |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5` to `.8` | `use-lazy-load-sentinel.test.ts`, `message-list-native.test.tsx`, `message-list-shared.test.tsx` |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9`         | Existing native prepend-anchor unit and browser assertions                                       |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10`        | Existing bounded-opening browser request watcher                                                 |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.12`        | Existing inactive-session reconciliation coverage                                                |

The TDD regression starts with the current tool-heavy case: initialized
history has `hasMore: true`, contains agent/tool rows, and has no loaded user
row. The expected synthetic fallback count is zero, which fails against the
current implementation.

## E2E tests

- Desktop Chromium: update
  `apps/web/e2e/tests/chat/message-pagination.spec.ts` to prove one upward
  navigation crosses more than twenty tool-heavy pages, reveals the stored
  latest prompt, and never shows the task-description fallback or normal retry
  control.
- Mobile Chrome: update
  `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts` with the same user
  outcome in the full-height mobile Chat surface.
- Shared seed: extend
  `apps/web/e2e/tests/chat/message-pagination-helpers.ts` with an efficient
  long-history fixture and retain prompt-`#1`, hidden pre-prompt, and anchor
  scenarios.

## Mobile design contract

- Desktop outcome: continuous upward history loading reaches the stored prompt
  without a misleading synthetic prompt or routine manual loader.
- Mobile entry point: the existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the current
  mobile pagination spec.
- Hierarchy and surface: no composition change; the transcript remains the
  single vertical scroll owner.
- Shared logic: desktop and mobile use the same store, hooks, cursor
  coordinator, sentinel, and recovery state.
- Mobile-specific detail: the recovery control keeps a 44-pixel coarse-pointer
  hit area.
- Mobile proof: the `mobile-chrome` flow reaches the stored prompt with one
  upward navigation and exercises recovery without horizontal or nested
  scrolling.

## Work orders

- [x] [Task 01: Establish the true transcript start](task-01-establish-transcript-start.md) (`done`)
- [x] [Task 02: Continue pagination through the preload region](task-02-continue-preload-pagination.md) (`done`)

## Verification results

- Focused Task 01 Vitest suite: passed (5 files, 87 tests).
- Focused Task 02 Vitest suite: passed (4 files, 115 tests).
- `pnpm run typecheck`: passed.
- `make build-web`: passed; Vite emitted existing chunk-size and dynamic-import
  warnings only.
- Desktop Chromium pagination: passed (6 tests).
- Mobile Chrome pagination: passed (6 tests).
- `git diff --check`: passed.

## Risks

- Default `hasMore: false` must not be mistaken for an authoritative exhausted
  response before history initializes.
- Boot hydration, WebSocket-only rows, refetch reconciliation, and deletion
  must not leave stale initialization metadata.
- Geometry checks must use committed layout and current scroll-root bounds so
  stale observer entries cannot cascade through off-screen history.
- A zero-result response and a real request error currently both surface as no
  progress; both require recovery without an automatic retry loop.
- The long-history browser seed must remain deterministic and fast enough for
  desktop and mobile projects.
