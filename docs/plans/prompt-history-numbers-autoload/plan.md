---
spec: docs/specs/ui/requirements/prompt-history-panel.md
created: 2026-08-18
status: draft
---

# Implementation Plan: Prompt History Numbers and Auto-Load

## Overview

Two increments on the shipped prompt-history panel buildout (`docs/plans/prompt-history-panel/`): (1) each prompt row shows a small `#N` label in front of the prompt text, where `N` is the prompt's absolute 1-based ordinal among all user messages of its session (`#1` = very first prompt of the whole session); (2) scrolling the panel down to the last rendered prompt auto-loads the next older page, exactly like the transcript's scroll-up sentinel, with no load-more button.

Build order: backend message contract first (`prompt_index`, persisted from a durable per-session sequence with an additive migration), then the frontend numbering display, then the panel auto-load sentinel, then E2E coverage plus the test-harness `author_type` passthrough that E2E seeding needs. All four land in one PR; tasks are sequential because each frontend step consumes the previous step's contract.

## Backend

### 1. Message contract: `prompt_index` (Task 01)

The message JSON gains an optional `prompt_index` (1-based ordinal among the session's user messages, ordered by normalized microsecond `created_at` asc, ties by `id` asc — identical to the panel's entry ordering). The web store receives it from paginated list/boot responses and indexed WS replay responses; ordinary single-message/full-list consumers remain on legacy scans. The ordinal is a durable per-session sequence: `prompt_seq` is allocated from a per-session counter inside the user-message create boundary (serialized by the advisory session lock on PostgreSQL and the single-writer pool on SQLite), persisted on the row, and read back directly by the paginated list and the single-row replay/update read. Because the ordinal never derives from the remaining rows, deleting an earlier prompt cannot renumber later ones and a backward clock correction cannot block new prompts; the migration adds the column and backfills historical user rows with their previously derived ordinal, seeding each session's counter at its backfilled maximum. Live user-message creation assigns the same normalized ordering key inside its…

- `apps/backend/internal/task/models/models.go` — `models.Message` gains `PromptIndex int` (json tag `prompt_index,omitempty`); `ToAPI()` copies it into `v1.Message`.
- `apps/backend/pkg/api/v1/task.go` — `v1.Message` gains `PromptIndex int` + `json:"prompt_index,omitempty"`.
- `apps/backend/internal/task/repository/sqlite/message.go` — keep hot `GetMessage` and full `ListMessages` 12-column; add dedicated `GetMessageWithPromptIndex` (reads persisted `prompt_seq`) and the `prompt_index`/`scanPromptIndexedMessageRows` projection to the paginated list and the single-row read. Keep all other legacy scanner/projection callers unchanged.
- `apps/backend/internal/db/dialect/` — provide dialect-specific normalized-microsecond expressions for the list `ORDER BY`, the outer cursor predicate/ordering, and the migration backfill, normalizing any SQLite `±TZ` offset to UTC (see Task 01).

The indexed query reads the persisted ordinal:

```sql
CASE WHEN author_type = 'user' THEN prompt_seq ELSE 0 END AS prompt_index
```

The list query selects the column directly (no window pass); the cursor/`id` predicate, ORDER BY, and `LIMIT (limit+1)` use the same normalized-microsecond key as the rest of the package, so pages are contiguous in ordinal order and the sentinel `promptNumber === 1` stop cannot skip a higher ordinal. `GetMessageWithPromptIndex` reads the same persisted column, so live, replay, and read ordinals agree by construction, and deletion of an earlier prompt leaves later ordinals unchanged. The cursor bound is pre-normalized to the exact `YYYY-MM-DD HH:MM:SS.ffffff` UTC string form (space separator, 6-digit fraction, no `Z`) before binding — never a serialized R…

- `apps/backend/internal/task/service/service_messages.go` / `message_handlers.go` — do not rely on a post-insert `GetMessage` read for live ordinals; use `GetMessageWithPromptIndex` for both idempotent replay reads and user `UpdateMessage` event publication. Keep agent streaming updates on the hot 12-column loaded model. Live zero-timestamp users use the atomic repository boundary; explicit user imports preserve only valid strictly-after-max timestamps.
- `apps/backend/internal/task/repository/sqlite/message.go` — atomic live-create transaction as specified in Task 01.



The boot payload (`backendapp/boot_state.go` `addTaskDetailActiveTaskState`) and the paginated HTTP handler need no code change: they serialize whatever the enriched `ListMessagesPaginated`/`ToAPI` produce.

## Frontend

### 2. Numbering display (Task 02)

- `apps/web/lib/types/http.ts` — `Message` gains `prompt_index?: number`.
- `apps/web/lib/types/session-events.ts` — the `MessageAddedPayload` declaration gains `prompt_index?: number`; `apps/web/lib/types/backend.ts` continues to re-export it unchanged.
- `apps/web/lib/ws/handlers/messages.ts` — `toMessage` maps `prompt_index: payload.prompt_index`.
- `apps/web/lib/prompt-history.ts` — `PromptHistoryEntry.promptNumber: number | null`; parse RFC3339/RFC3339Nano into a full BigInt epoch-nanosecond key; use a separate ordering key `epochNanoseconds / 1000n` (UTC microsecond storage precision) with `id` as the tie-break, and use the full nanosecond key only for duration subtraction/bounds. Normalize offsets, remove variable-width fractions before whole-second parsing, and pad fractions to nine digits. Duration next-prompt bounds evaluate over the loaded contiguous newest suffix per the spec; an unloaded edge prompt uses the turn-completion bound until older pages arrive.
- `apps/web/lib/state/slices/session/message-signature.ts` and reconciliation — HTTP/boot snapshot merging must treat a newly present `prompt_index` as a meaningful change even when `updated_at` is unchanged, preserve a previous defined index when the incoming payload omits it, and never clear a known index from an older/transient payload.
- `apps/web/components/task/prompt-history-panel-content.tsx` — `PromptHistoryRow` renders `#N` at the start of the prompt bubble (before the robot icon and the truncated text), small type (`text-[10px]`-scale, muted), `data-testid="prompt-history-number-<index>"`, only when `entry.promptNumber !== null`. It is a sibling of the text/expanded box, so it stays visible in the expanded state. No i18n keys: `#N` is not translatable copy (precedent: `#${pr.pr_number}`, `queuePositionLabel`).
### 3. Auto-load sentinel (Task 03)

- Extract the transcript's private `useLazyLoadSentinel` (`apps/web/components/task/chat/message-list-native-scroll.ts`) into `apps/web/hooks/use-lazy-load-sentinel.ts` with options `rootMargin`, `rearmWhileIntersecting`, and `joinInFlightWhileLoading` (all default to transcript behavior, with re-arm/join disabled). The shared hook returns `{ sentinelRef, onUserGesture }` and takes separate `blocked` (hard initial/refetch loading; never fires or joins) and `isLoadingMore` (older-page in flight; join-eligible) inputs, so `joinInFlightWhileLoading` bypasses only the `isLoadingMore` term and can never fire during a refetch. It explicitly unobserves the current panel sentinel before awaiting a load, then re-observes after a positive result while still mounted/current. After rejection or zero result, it re-observes disarmed, ignores the current intersection, arms on an observed exit, and permits retry on the next true transition or a panel wheel/touch gesture when the sentinel cannot exit. This preserves transcript behavior, avoids immediate retry loops, and enables user-driven recovery.
- `apps/web/lib/state/slices/session/types.ts`, `session-slice.ts`, `use-session-messages.ts`, `use-lazy-load-messages.ts`, `message-backfill.ts`, `older-message-pagination.ts`, and every existing `useLazyLoadMessages` caller — add distinct `isLoadingMore` metadata, keep initial/refetch `isLoading` untouched by older-page merges, migrate all listed consumers and fakes, and route automatic backfill/last-prompt preload through the same first-request-wins coordinator. The native transcript passes `blocked = messagesLoading` and `isLoadingMore` with join disabled; the panel passes the same `blocked` and `isLoadingMore` with `joinInFlightWhileLoading: true`; `useDrainOlderMessages` tracks cumulative `fetchedMessageCount` returned by `loadMore`, stops at the first batch whose cumulative count reaches or exceeds `MAX_DRAIN_MESSAGES = 1000`, and also stops immediately after a zero-result batch. The coordinator retains zero-row cursors and clears only `isLoadingMore` on success/error.
- `apps/web/components/task/prompt-history-panel-content.tsx` — preserve `session?.is_passthrough` as the first unconditional early return with no rows/arrows/sentinel/loading controls; consume `useSessionMessages(sessionId)` for `messages` plus `messagesLoading`, and `useLazyLoadMessages(sessionId)` for `loadMore`, `hasMore`, and `isLoadingMore`; derive `shouldPaginate = hasMore && !entries.some((entry) => entry.promptNumber === 1)`; render the sentinel only while `shouldPaginate`, render the older-page loading row as `{t("task:loadingOlderMessages")}` only while `shouldPaginate && isLoadingMore`, and a neutral initial loading state as `{t("task:loading")}` when `entries.length === 0 && messagesLoading && !isLoadingMore`; older-page loading takes precedence if both flags are true; attach `onWheel`/`onTouchMove` to invoke `onUserGesture` for any short/disarmed panel, including non-empty rows; show the definitive empty state only when `entries.length === 0 && !hasMore && !messagesLoading`. If ordinals are absent, fall back to `hasMore` exhaustion. Wire the shared hook with `{ rootMargin: "0px 0px 200px 0px", rearmWhileIntersecting: true, joinInFlightWhileLoading: true }`; no load-more button.


## Tests

Every plan must include tests. Per-behavior:

- **Create-time enrichment + broadcast** — extend `apps/backend/internal/task/service/service_messages_test.go` (or new file): `CreateMessage` and fresh user `CreateMessageWithID` return `PromptIndex == N` from the atomic repository create path; the published `message.added` event data contains `prompt_index: N`; idempotent replay returns the same indexed message without a duplicate add event; a user update publishes `prompt_index: N` in `message.updated`; an agent create/update publishes no `prompt_index`. With a fixed clock and reverse-ordered UUIDs, assert SQLite and PostgreSQL normalize/advance timestamps so reread order and WebSocket indexes are distinct and consistent, a backward clock correction clamps the ordering timestamp forward instead of blocking (the ordinal comes from the durable sequence). Add a deterministic concurrent-create test that delays the earlier-created insert, asserts distinct final ordinals, and verifies both events carry those distinct indexes rather than duplicate `#1`. Add a failed-first-attempt/concurrent-winner retry test proving a failed repository attempt does not leave `CreatedAt`/`PromptIndex` on the retried model.
- **Read-path and durability regression** — `GetMessage` and full `ListMessages` remain legacy 12-column paths; `GetMessageWithPromptIndex` (persisted `prompt_seq`) and `ListMessagesPaginated` (direct column projection) agree on ordinals; the migration is additive (one column + one counter table + backfill, idempotent) and the fixture tests exercise the backfill exactly as a migrated database. Deleting an earlier prompt after a later ordinal was published leaves the later ordinal unchanged and never reuses the deleted ordinal; a backward clock correction clamps the ordering timestamp instead of blocking the create; SQLite legacy rows with nanosecond-distinct timestamps inside one microsecond verify the normalized ordering by microsecond then id, matching frontend labels; PostgreSQL repeats the assertions plus a concurrent delete/create regression. A tied-microsecond fixture verifies pagination pages are contiguous in ordinal order (loading a page containing `#1` never leaves a lower ordinal unloaded), that the SQLite normalized key equals the frontend `epochNanoseconds / 1000n` floor division, that an offset-carrying seed row (`+02:00`) matches the frontend UTC key, that an offset-carrying cursor yields contiguous pages with no dup/skip, and that non-first pages return absolute (not page-relative) `prompt_index` values.
- **Clarification scanner regression** — existing clarification repository tests (or a new focused test file) exercise `message_clarification_delivery.go` and all `message_clarification_response.go` paths that use the legacy 12-column `scanMessageRows`; assert they still scan and return messages after the enriched scanner is introduced.
- **Entry derivation** — `apps/web/lib/prompt-history.test.ts` (exists): entries carry `promptNumber` from `prompt_index`; absent/0 → null; ordering pairs identical through fractional digits 1–6 and differing only in digits 7–9 plus reverse-ordered IDs order by microsecond-truncated timestamp then id; mixed-width fractions (`.1Z`, `.123456789Z`, and no fraction), timezone offsets, and a legacy pair tied after microsecond truncation but distinct in full nanoseconds verify `(microseconds,id)` list order versus full-nanosecond duration subtraction, including expected clamp/overlap; all use normalized BigInt keys.
- **Panel numbering** — `apps/web/components/task/prompt-history-panel-content.test.tsx` (exists): rows render `#1`, `#2`, … newest = highest; no label when `prompt_index` absent; label visible in expanded state.
- **Sentinel hook** — `apps/web/hooks/use-lazy-load-sentinel.test.ts` (new; IntersectionObserver stub): fires `loadMore` on intersect when eligible; no-options defaults preserve transcript margin and disabled re-arm/join; panel re-arm unobserves before loading and re-observes after positive progress; after rejection/zero result, disarms until an observed exit then permits the next re-entry or `onUserGesture` while still intersecting; stale completions do not re-arm.
- **Panel auto-load** — extend `prompt-history-panel-content.test.tsx`: passthrough remains an unconditional no-controls state even when entries/hasMore exist; follower intersection after propagated `isLoadingMore=true` joins the shared promise; agent/tool-only pages re-arm only after positive progress; errors/zero results do not loop and support scroll-out/scroll-back or wheel/touch retry; zero-row `has_more=true` retains its cursor; empty `hasMore=true` remains paginatable; empty `hasMore=false` with `messagesLoading=true` and `isLoadingMore=false` shows neutral loading, not empty; prompt `#1` plus older non-user rows has neither sentinel nor loading; no button; loading row only while `shouldPaginate && isLoadingMore`.
- **Shared older-message pagination** — `apps/web/hooks/domains/session/older-message-pagination.test.ts` (new): invoke the shared coordinator concurrently from panel, native transcript, automatic backfill, and last-prompt preload consumers for one session/cursor with different requested limits; assert one request/merge using first-request-wins page size, shared positive result, monotonic cursor metadata, `isLoadingMore` clears on success/error without changing initial `isLoading`, no cross-session interference, and zero-row retry cursor retention/progress. Also assert a small-page backfill still reaches the 1000-message budget, a large-page drain stops at the first batch reaching/exceeding 1000 messages (mixed first-request-wins page sizes may overshoot by at most one batch), and a retained-cursor zero-row drain terminates after one no-progress attempt.

- `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts` (exists) — assert the single seeded prompt's row shows `#1`.
- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts` (exists, desktop) — assert row-0 (the second, UI-sent prompt) shows `#2` and row-1 (the seeded first prompt) shows `#1`; before sending the second prompt, assert the settled session contains exactly one user message (the seeded first prompt) so the ordinals stay deterministic; no extra seeding.
- `apps/web/e2e/tests/task/prompt-history-auto-load.spec.ts` (new) — call `test.setTimeout(180_000)`; task description = first prompt (`#1`); after the boot turn settles, assert exactly one pre-seed user message exists, fetch all settled session messages, compute `seedBase = max(created_at) + 1 second`, assign the marker the earliest seed timestamp (`seedBase`), and seed the remaining 119 prompts at one-microsecond-aligned timestamps at least 1ms apart; refetch and assert strict stored timestamp order (the pre-seed count pins the marker's absolute ordinal `#2`). Register a route handler that ignores older-page requests until `panelScrollTriggered`; open the panel, set that flag immediately before scrolling, hold the next panel-triggered request, assert loading/marker absence, release, and loop until `#1`, then assert marker `#2`; no clicks or load-more button. Add the same deterministic seed/hold flow to Pixel.
- Harness support (Task 04): `seedMessageRequest` accepts optional `author_type` and RFC3339 `created_at`; omitted author type remains agent, explicit user seeds use deterministic timestamps, `seedMessageHandler` reads seeded user rows through `GetMessageWithPromptIndex`, and `seedMessageAdded` serializes `prompt_index` plus `created_at` with `time.RFC3339Nano`. `seedSessionMessage` passes both fields through. The specs seed before page load, so the panel also validates the HTTP list path.
- E2E verification must use two runner invocations rooted at `apps/web`: first `cd apps/web && pnpm e2e:run -- e2e/tests/task/prompt-history-auto-load.spec.ts e2e/tests/task/prompt-history-panel.spec.ts`, then `cd apps/web && pnpm e2e:run -- --project mobile-chrome --no-build -- e2e/tests/task/mobile-prompt-history-panel.spec.ts`. The first builds backend and `build:e2e` frontend; the second reuses those fresh artifacts and explicitly collects the Pixel project.

## Verification Results

All four tasks implemented and verified 2026-08-19. Per-task commands/results are in each task file's `## Results`; the full suite passed:
- Backend: `go build ./...`; `go test ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers` (all ok); `go test ./internal/office/testharness/...` (ok); Postgres tests compile and skip without `KANDEV_TEST_POSTGRES_DSN` (CI postgres job supplies it).
- Frontend: Task 02 unit set (75 tests), Task 03 unit set (164 tests) and `pnpm run typecheck` all pass; eslint clean on every touched file.
- E2E: `pnpm e2e:run -- e2e/tests/task/prompt-history-auto-load.spec.ts e2e/tests/task/prompt-history-panel.spec.ts` (2 passed), `pnpm e2e:run -- --project mobile-chrome --no-build -- e2e/tests/task/mobile-prompt-history-panel.spec.ts` (2 passed).

## Implementation Waves And Parallel Candidates

All sequential — each task consumes the prior task's contract (backend field → frontend type/display → auto-load → E2E).

```
Wave 1:
- [x] [task-01-backend-prompt-index](task-01-backend-prompt-index.md)

Wave 2:
- [x] [task-02-frontend-numbering](task-02-frontend-numbering.md)

Wave 3:
- [x] [task-03-frontend-auto-load](task-03-frontend-auto-load.md)

Wave 4:
- [x] [task-04-e2e-and-harness](task-04-e2e-and-harness.md)
```

## Open Questions

None.
