---
created: 2026-08-30
status: complete
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
depends_on: []
---

# Task 01: Reconcile the Latest Transcript Window

## Outcome

A revisited session stores one contiguous newest message interval, uses its
true oldest cursor, and can paginate to an attributed peer prompt that was
persisted while the session was inactive.

## In scope

- Add a pure latest-window reconciliation helper for session messages.
- Track which cached IDs existed when a latest fetch began.
- Join overlapping cached and fetched windows without duplicates.
- Replace a disjoint cached prefix while retaining live rows received during
  the fetch.
- Apply the helper and its cursor to `fetchAndStoreMessages`.
- Add focused unit tests before the implementation.
- Add a deterministic desktop Playwright regression for same-task sibling
  session history.

## Exclusions

- Backend, database, MCP, and WebSocket routing changes.
- New API fields, pagination modes, or eager gap filling.
- Responsive layout or interaction changes.
- New user-facing text or localization keys.

## Implementation acceptance conditions

1. A disjoint stale cache and newest response produce only the contiguous
   fetched suffix plus rows proven to have arrived during the request, with
   `oldestCursor` equal to the fetched suffix boundary.
2. An overlapping response preserves cached older pages, deduplicates shared
   rows, retains live additions, and keeps one chronological interval.
3. In a browser flow with two sessions on one task, a receiver cached before it
   becomes inactive can later paginate to exactly one attributed sibling prompt
   after more than 100 newer rows have been persisted.

## TDD sequence

1. Add helper tests for an overlapping window, a disjoint stale window, and a
   live row that appears after the fetch begins. Run them and record the RED
   failure.
2. Add the same-task sibling Playwright scenario and record that the prompt is
   unreachable with the stale cursor.
3. Implement the smallest pure reconciliation helper and wire it into the
   latest-fetch path.
4. Run the focused unit and browser tests to GREEN, then refactor without
   widening the store or backend contracts.

## Likely files

- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/message-window-reconciliation.ts`
- `apps/web/hooks/domains/session/message-window-reconciliation.test.ts`
- `apps/web/e2e/tests/chat/inactive-session-transcript-reconciliation.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if the existing message seed helpers
  cannot express the deterministic same-task scenario.

## Exact verification commands

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/session/message-window-reconciliation.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/inactive-session-transcript-reconciliation.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:ratchet)
```

## Results

- RED unit evidence: the pre-fix hook retained `stale-1` and `stale-2` around a
  disjoint newest response and derived `oldestCursor` from `stale-1`.
- RED browser evidence: the newest receiver message loaded, but upward
  pagination could not reach the persisted attributed peer prompt.
- GREEN implementation: `reconcileLatestMessageWindow` joins overlapping
  windows, replaces disjoint prefixes, preserves in-flight live additions, and
  returns the cursor for the actual contiguous boundary.
- PR fixup: the deduplicated request owns the first cache baseline, timestamp
  ordering preserves the backend's normalized-microsecond precision, and older
  out-of-order live rows remain for the next page instead of preceding the
  cursor boundary.
- GREEN verification:
  - `pnpm --filter @kandev/web exec vitest run hooks/domains/session/message-window-reconciliation.test.ts hooks/domains/session/use-session-messages.test.ts` (27 passed)
  - `pnpm e2e:run --project chromium tests/chat/inactive-session-transcript-reconciliation.spec.ts` (1 passed)
  - `pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts` (5 passed)
  - `pnpm run typecheck`
  - `pnpm run lint`
  - `pnpm run i18n:ratchet`
  - `python3 scripts/lint-spec-files.py --all`
