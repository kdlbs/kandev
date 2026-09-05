---
created: 2026-08-26
status: done
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation Plan: Stop Chat Scroll Pagination Flicker

## Overview

The transcript will limit each upward load cycle to the current visible top boundary.

One upward action can skip tool-only pages that do not add a visible entry. The cycle stops when a page adds a visible entry.

## Scope

### In scope

- Require an upward transcript action before automatic older-page loading starts.
- Stop a load cycle after a page changes the visible top-boundary key.
- Continue the same cycle when a page only extends one collapsed activity group.
- Preserve the current reading position after each prepend.
- Add debug events for the trigger, geometry, boundary change, and stop reason.
- Add desktop and mobile browser evidence for request count and scroll geometry.

### Out of scope

- Backend pagination queries, cursors, page size, persistence, or API fields.
- Transcript virtualization or changes to message grouping.
- Removal of the explicit older-page recovery control.
- Changes to the prompt `#1` visible-start boundary.
- Loading the complete transcript before an upward user action.

## Confirmed root cause

The native transcript enables `rearmWhileIntersecting` in `message-list-native-scroll.ts`.

After each positive request, `use-lazy-load-sentinel.ts` schedules a continuation from `intersectingRef`. This value can describe the intersection before the prepend.

The continuation does not wait for the committed visible boundary. It can start another request after a page adds visible rows and moves the sentinel away.

A temporary Chromium test seeded 180 visible agent messages behind the newest window. One `scrollTop = 0` action caused five requests in 277 ms.

The probe recorded these changes:

- `scrollHeight`: 5,864 px to 10,434 px.
- `scrollTop`: 0 px to 1,143 px, 2,286 px, 3,429 px, and 4,572 px.
- Older-page request count: 5. The expected count was 1.

The repeated range growth and scroll restoration produced the shrinking scrollbar and visible flicker. The temporary test and browser probe were removed after diagnosis.

## Technical approach

### Transcript load-cycle state

- Record the oldest standalone message key for each older-page request.
- Continue automatically while tool pages leave that visible boundary unchanged.
- Disarm the observer when a page adds a new visible top entry.
- Re-arm from an observed sentinel exit/re-entry or a later actual upward scroll. This covers short pages that remain inside the 200 px preload margin.
- Keep the request generation as a diagnostic correlation label only. It does not control pagination.
- Do not treat prepend restoration or guarded programmatic scroll changes as new user intent.

### Sentinel continuation

- Update `use-lazy-load-sentinel.ts` to use a caller-supplied continuation decision.
- Make the decision after the prepended React state commits.
- Do not use a stale `intersectingRef` value as sufficient continuation evidence.
- Keep Prompt History pinned-bottom behavior unchanged.
- Keep request serialization, failure disarm, and explicit retry behavior unchanged.

### Diagnostics

- Add `messages:pagination` to the namespace list in `lib/debug/log.ts`.
- Emit one start event and one settle event for each transcript older-page request.
- Include primitive fields for session, trigger, boundary keys, request count, geometry, and stop reason.
- Do not log message content or every scroll event.

### Browser regression

- Add a shared seed with several older pages of visible agent messages.
- Record older-page requests after the watcher starts.
- Perform one upward action and wait for the first request to settle.
- Prove that no second request occurs when the page adds visible entries.
- Prove that the anchor row remains within the existing 8 px tolerance.
- Add a collapsed-tool scenario that continues until one new visible entry appears.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4` | Sentinel and native-scroll unit tests require upward intent at the oldest boundary. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5` | A browser scenario continues through collapsed tool-only pages. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6` | The collapsed scenario reaches the next visible entry without another action. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.8` | Desktop and mobile tests prove that one action does not cascade visible pages. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9` | Existing prompt `#1` boundary tests remain green. |

## E2E tests

- Desktop: `apps/web/e2e/tests/chat/message-pagination.spec.ts` in the `chromium` project.
- Mobile: `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts` in the `mobile-chrome` project.
- Shared cues: request count, `scrollTop`, `scrollHeight`, the oldest visible row, and the anchor-row position.

## Mobile design contract

- Desktop outcome: one upward boundary action loads only the required older pages.
- Mobile entry point: the existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the current mobile pagination test.
- Hierarchy and surface: the transcript remains the only vertical scroll owner.
- Shared logic: desktop and mobile use one pagination cycle, sentinel, store, and cursor coordinator.
- Mobile proof: the mobile test uses the same visible-page and collapsed-tool scenarios.

## Work orders

- [x] [Task 01: Constrain transcript pagination cycles](task-01-constrain-transcript-pagination-cycles.md)

## Verification results

- Red evidence: the new desktop cascade test timed out because one top-edge action issued more than one older-page request on the previous implementation.
- Unit tests: 43 passed across the shared sentinel and native transcript scroll suites.
- TypeScript typecheck: passed.
- Targeted ESLint: passed with no warnings.
- Specification lint: passed.
- Desktop Chromium E2E: 4 passed, including one-request visible pagination and automatic collapsed-tool continuation.
- Mobile Chrome E2E: 4 passed with the same pagination and anchor contract.

## Risks

- A continuation decision that runs before React commits can read the old boundary key.
- Loading-state rows can change scroll geometry without adding transcript content.
- Gesture tracking must not treat scroll restoration as user intent.
- The shared sentinel also serves Prompt History. Transcript rules must not change its pinned-bottom behavior.
- Touch, keyboard, wheel, and scrollbar navigation must produce the same boundary result.
