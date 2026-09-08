---
created: 2026-08-30
status: complete
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation Plan: Reconcile Inactive Session Transcript Windows

## Overview

Revisiting a previously opened agent session will restore one contiguous newest
message window. Older persisted rows will remain reachable through the existing
upward pagination path.

This fixes the observed case where one sibling agent sent a durable prompt to
another session, the receiver processed it while inactive, and the prompt later
could not be reached in the UI.

## Scope

### In scope

- Reconcile a bounded latest-message response with cached session rows as one
  contiguous window.
- Preserve cached older pages only when they overlap the fetched suffix.
- Preserve live rows that arrive while the latest fetch is in flight.
- Derive `oldestCursor` from the true contiguous boundary.
- Add a pure frontend regression test for overlapping, disjoint, and in-flight
  message cases.
- Add desktop browser evidence for a same-task sibling session that receives an
  attributed prompt while inactive and accumulates more than 100 later rows.

### Out of scope

- Message persistence, MCP delivery, sender attribution, or backend pagination
  queries.
- Global or task-wide delivery of session-detail WebSocket traffic.
- Subscribing to inactive sibling sessions.
- Eagerly loading a complete transcript when a task or session opens.
- Changes to transcript layout, grouping, navigation, copy, or touch behavior.

## Confirmed root cause

The receiver prompt was persisted and dispatched to the intended Luna session.
The receiver was inactive, so the scoped `BroadcastToSession` correctly had no
browser recipient. Luna still received the prompt at the agent runtime and
acted on it.

When the browser revisited Luna 34 minutes later, 156 rows had been persisted
after the prompt. The bounded latest fetch returned 100 rows whose oldest row
was newer than the prompt. `fetchAndStoreMessages` retained the much older
cached rows even though the two sets did not overlap, sorted both sets, and set
`oldestCursor` from the oldest cached row. The local array therefore contained
an unloaded middle gap while pagination started before that gap. The durable
peer prompt became unreachable from the transcript.

## Technical approach

### Contiguous-window reconciliation

- Extract a pure latest-window reconciliation helper beside
  `use-session-messages.ts`.
- Capture the cached message IDs before issuing `message.list`.
- After the response, detect whether the fetched suffix overlaps the current
  cached window.
- For an overlapping window, deduplicate and sort the complete joined interval.
- For a disjoint window, use the fetched suffix as the new base and retain only
  rows that were not present before the request and arrived through live
  delivery while it was in flight.
- Return both the reconciled messages and the correct oldest cursor. A disjoint
  prefix must not influence that cursor.
- Keep the existing monotonic fetch guard and Zustand identity reconciliation.

### Browser regression

- Launch one task with two normal agent sessions and open the receiver first so
  its initial rows enter the browser cache.
- Switch to the sibling so the receiver becomes inactive.
- Persist an attributed user prompt on the receiver, followed by enough agent
  or tool rows to push that prompt outside the newest 100-row fetch.
- Restore the receiver's earlier cache snapshot after test-only live broadcasts
  settle, then trigger the normal foreground-recovery fetch.
- Reopen the receiver, navigate upward, and assert the peer prompt and sender
  badge become visible exactly once.

## Test strategy

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4` | The browser flow reaches the attributed prompt through the existing upward pagination action. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6` | Collapsed activity pagination continues until the peer prompt becomes a standalone visible entry. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10` | Pure reconciliation tests and the same-task sibling browser scenario prove that a stale cache cannot skip the persisted middle range. |

## Mobile design contract

- Desktop outcome: switching back to a receiver session exposes one contiguous
  transcript window and makes the peer prompt reachable.
- Mobile outcome: the same shared session-message hook and native transcript
  preserve the identical durable-history result.
- Nearest mobile exemplar: `apps/web/components/task/task-layout.tsx` and the
  existing mobile message-pagination scenarios.
- This fix changes state normalization only. It adds no mobile interaction,
  layout, navigation, scroll-owner, or touch-target behavior.
- Focused helper coverage plus the desktop end-to-end regression owns the new
  behavior. Existing `mobile-message-pagination.spec.ts` remains the responsive
  proof for the shared cursor and upward-pagination path; no new
  viewport-specific scenario is required.

## Work orders

- [x] [Task 01: Reconcile the latest transcript window](task-01-reconcile-latest-transcript-window.md)

## Dependency order

Task 01 is one vertical repair. Its unit regression must fail before the helper
is implemented, and its browser regression must fail before the production
reconciliation is changed.

## Verification strategy

Run the focused unit test first, then the production-build Chromium scenario.
Finish with frontend type, lint, and localization ratchet checks. The backend is
unchanged. Use a subshell for each command so the documented paths are safe to
copy as one shell block.

## Results

- Added a pure reconciliation helper that keeps overlapping history, replaces
  disjoint stale prefixes, and retains rows received during an in-flight fetch.
- Wired the helper's contiguous boundary into the existing latest-message
  fetch without changing backend, WebSocket, or store contracts.
- Added unit coverage for all reconciliation branches and a two-session browser
  regression that reproduces the previously unreachable attributed prompt.
- The PR fixup stores the cache baseline on the deduplicated request, compares
  RFC3339 timestamps at the backend's normalized-microsecond precision, and
  excludes out-of-order older live rows from a disjoint window until pagination
  reaches them.
- Verified the focused unit suite (27 tests), desktop Chromium regression,
  existing mobile pagination suite (5 tests), frontend typecheck and lint,
  i18n ratchet, and specification lint.

## Risks

- Retaining every cached extra recreates the gap; discarding every extra can
  erase a live row that arrived while the fetch was pending.
- An overlapping cache can contain legitimately paginated older rows and must
  keep them.
- The cursor must follow the reconciled interval boundary, not simply the
  first row after sorting unrelated sets.
- The test must seed deterministic message order and confirm that the peer
  prompt is outside the newest response page.

## Rejected alternatives

- Broadcasting session-detail messages globally or subscribing to every
  sibling session conflicts with the bounded session-stream architecture.
- Automatically loading every missing page on revisit increases transcript
  traffic and conflicts with the bounded-opening contract.
- Keeping a disjoint prefix and introducing multiple cursors would add a new
  store model and pagination protocol for a repair that one contiguous window
  can solve.
