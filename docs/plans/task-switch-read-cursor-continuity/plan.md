---
created: 2026-09-07
status: complete
requirements:
  - ../../specs/office/requirements/unread-divider.md
  - ../../specs/ui/requirements/transcript-auto-scroll.md
system_design:
  - ../../specs/office/system-design/unread-divider.md
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Task-switch Read-cursor Continuity

## Overview

Keep completed task transcripts at the reader's intended bottom position after
switching tasks. Correlate mark-read responses per session so another task
cannot leave the returning task's local cursor stale, and add bounded debug
events that identify cursor freshness and the scroll owner that performed each
placement.

The repair is one sequential work order because the cursor race, divider
eligibility, scroll diagnostics, and browser regression describe one user flow.

## Root cause

`useSessionReadTracking` stores the latest mark-read dispatch in one ref for the
currently rendered hook. When task A's response is delayed and task B dispatches
another request through the same hook instance, B replaces the ref. A's
successful response is then discarded even though it is still the newest
request for session A. The backend cursor advances, but the in-memory session A
cursor stays stale.

Returning to completed task A can therefore recreate its prior unread-divider
anchor. That divider wins initial scroll placement and moves the transcript
away from the bottom. A running agent hides the defect because a later
work-start or streamed-message update performs another bottom write.

A focused throwaway hook test reproduced the race: session A's delayed response
produced no `updateSessionReadCursor("session-1", "m2")` call after session B
dispatched. The temporary test was removed after recording the red evidence.

## Scope

### In scope

- Per-session freshness for frontend mark-read responses across task switches,
  hides, and unmounts.
- Existing same-session out-of-order response protection.
- Persistent debug events for read tracking and transcript placement ownership.
- Unit and desktop/mobile Playwright regressions using completed tasks.
- An Office unread-divider system design for the existing requirement.

### Out of scope

- Changing unread-divider product behavior, default, persistence schema, or
  backend endpoint.
- Changing transcript auto-scroll preference semantics or saved reader offsets.
- Changing task navigation, layout, touch behavior, or user-facing copy.
- Relying on running-agent updates to correct initial placement.

## Technical approach

### Correlate mark-read responses by session

Replace the hook-local single-session freshness ref in
`apps/web/components/task/chat/use-session-read-tracking.ts` with a bounded
module coordinator keyed by session ID. Give every dispatch a generation. Apply
a response only when that generation is still latest for its session; clear the
entry only when the latest request settles. Requests for other sessions never
invalidate it.

Keep the existing narrow `updateSessionReadCursor` store action and backend
monotonic write. Do not optimistically advance the cursor before the HTTP
request succeeds.

### Add bounded diagnostics

Register `messages:read-tracking` in `apps/web/lib/debug/log.ts`. Log visit
capture, dispatch, applied response, stale discard, and failure with flat
primitive fields. Extend `messages:scroll-placement` logging to expose
delegation to unread-divider/layout/navigation/programmatic owners and actual
bottom writes caused by work start or message updates. Do not log message
content, saved offsets, every render, or every native scroll event.

### Prove completed-task switching

Add the permanent unit regression first and observe the same failure as the
throwaway test. Extend the existing unread-divider Playwright coverage with two
completed overflowing tasks. Hold task A's successful mark-read response,
switch to task B, release A after B dispatches, return to A, and assert that A
has no repeated divider and is at the bottom. Share the response-hold helper
between desktop and mobile specs.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-OFFICE-UNREAD-DIVIDER-001.3` and `.4` | `use-session-read-tracking.test.ts` proves unrelated sessions do not supersede each other's responses and retains same-session stale-response rejection. |
| `AC-OFFICE-UNREAD-DIVIDER-001.7` | `message-list-native.test.tsx` proves placement delegates only to a current divider and distinguishes later running-agent bottom writes. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11` and `.14` | Desktop and mobile task-switch scenarios prove a completed enabled transcript returns to its current bottom without a running update. |

## E2E tests

- `apps/web/e2e/tests/chat/unread-divider.spec.ts` covers the desktop sidebar
  task switch between completed tasks.
- `apps/web/e2e/tests/chat/mobile-unread-divider.spec.ts` covers the same user
  value through the existing mobile task switcher.
- A shared helper under `apps/web/e2e/helpers/` delays delivery of one
  mark-read response after the backend has accepted it, then exposes an
  explicit release signal. The tests use causal state and response gates, not
  wall-clock sleeps.

Mobile uses the existing full-height task Chat tab and task-switcher sheet. The
transcript remains the only scroll owner; no composition or touch target
changes.

## Work orders

- [x] [Task 01: Preserve read cursor across task switches](task-01-preserve-read-cursor-across-task-switches.md)

## Verification results

- TDD regression: the cross-session response test failed before the fix because
  task A received no local cursor update after task B dispatched; it passed
  after session-scoped correlation was added.
- Focused Vitest: 2 files passed, 74 tests passed.
- TypeScript typecheck passed.
- Scoped ESLint passed with no warnings or errors.
- Chromium completed-task switch E2E passed (1 test).
- Mobile Chrome completed-task switch E2E passed (1 test).
- Specification linter tests passed (30 tests); all specification files passed.
- `git diff --check` passed.

## Risks

- A module coordinator must stay bounded to in-flight requests and must not
  leak one entry per opened session.
- Clearing a generation too early can let an older response look current;
  clearing another session's generation recreates this defect.
- Browser task switches can trigger session hydration that masks a stale local
  cursor. The response hold and assertions must prove the in-memory path is the
  one under test.
- Debug events can become noisy during streaming. Emit only lifecycle decisions
  and actual placement writes.
