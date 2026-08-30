---
spec: docs/specs/ui/requirements/message-queue-merge.md
created: 2026-07-31
status: completed
---

# Implementation Plan: Merge Enqueued Messages Individually

## Overview

Add a per-entry **Merge with above** control to the queued-message panel. The
control folds a queued message into the message directly above it (same sender
kind only: user↔user, agent↔agent from the same sender task), concatenating
content, attachments, and entity references, then removing the folded entry.
The merge is implemented as a new atomic WebSocket action `message.queue.merge`
so a concurrent drain can never observe a half-merged queue.

Order: repository contract and transaction first, then the service + WS handler
that expose it, then the frontend API/hook/components, then E2E.

---

## Backend

### Contract (`apps/backend/pkg/websocket/actions.go`)

- Add `ActionMessageQueueMerge = "message.queue.merge"` beside the other
  `ActionMessageQueue*` constants (line ~120).

### Message-queue package (`apps/backend/internal/orchestrator/messagequeue/`)

- `types.go`
  - Add `MetadataSenderTaskID = "sender_task_id"` constant next to the other
    `Metadata*` constants (reused by the merge sender-task equality check).
  - Add sentinel error `ErrNoMergeTarget = errors.New("no mergeable message above")`
    next to `ErrEntryNotFound` — signals the source exists but has no valid
    target above it (head, different sender kind, agent sender-task mismatch,
    or a reserved/in-flight target).
- `repository.go`
  - Add to the `Repository` interface:
    `MergeIntoAbove(ctx context.Context, sessionID, sourceID string, queuedBy string) (*QueuedMessage, error)`.
- `repository_sqlite.go`
  - Implement `MergeIntoAbove` inside one transaction:
    1. SELECT the source row by `(id, session_id)` → `ErrEntryNotFound` when
       missing.
    2. SELECT the target = the entry in the session with the greatest
       `position` strictly below the source's `position` → `ErrNoMergeTarget`
       when none (source is head).
    3. Kind/ownership gate (reject with `ErrNoMergeTarget` unless all hold):
       - source `queued_by == QueuedByUser`: target `queued_by == queuedBy`
         (caller, non-empty, non-reserved) — user merges into user.
       - source `queued_by == QueuedByAgent`: target `queued_by == QueuedByAgent`
         and both `metadata[sender_task_id]` values are equal — agent merges
         into agent from the same sender task.
       - otherwise (workflow/server/system, mismatched kinds, agent sender
         mismatch) → `ErrNoMergeTarget`.
       - a target that `IsReservedInFlight()` is not a valid target →
         `ErrNoMergeTarget`.
    4. Merged fields:
       - content = `joinMergeContent(target.Content, source.Content)` — join
         with `"\n\n"`, but use `source.Content` verbatim when the target
         content is empty (no leading blank line).
       - attachments = `append(target.Attachments, source.Attachments...)`.
       - metadata = copy of target metadata with `MetadataEntityReferences`
         replaced by the union of both entries' reference lists, deduplicated
         by canonical `ref` (`normalizeEntityReferences`-style dedupe — reuse
         the existing `MetadataEntityReferences` key; see `types.go` and the
         memory repo's `applyMetadataUpdates`). The deduplicated union must
         not exceed the shared per-message cap
         (`entityrefs.MaxReferencesPerMessage`): an over-cap union fails with
         `ErrMergeReferenceOverflow` before either row is written, leaving
         both entries unchanged (atomic rejection, sqlite and memory alike).
    5. `UPDATE` the target row (content/attachments/metadata, scoped by
       `id` + `session_id`), then `DELETE` the source row (scoped the same),
       then commit. Return the merged target entry.
  - Keep the function ≤80 lines/≤50 statements; extract the kind gate and the
    reference-union helper if needed.
- `repository_memory.go`
  - Mirror the same logic under the repository mutex, returning the same errors
    and merged semantics (used by service/handler tests and the memory repo
    tests).
- `service.go`
  - Add `MergeIntoAbove(ctx, sessionID, entryID, queuedBy string) (*QueuedMessage, error)`
    delegating to `s.repo.MergeIntoAbove` with the same `s.logger.Info` log
    pattern as `RemoveEntry`.

### WS handler (`apps/backend/internal/orchestrator/handlers/queue_handlers.go`)

- Extend the `QueueService` interface with `MergeIntoAbove`.
- Register `ws.ActionMessageQueueMerge` → `wsMergeIntoAbove` in
  `RegisterHandlers`.
- New request type `wsMergeIntoAboveRequest{SessionID, EntryID, UserID}`.
- `wsMergeIntoAbove`:
  - validate `session_id` / `entry_id` required (validation error), reject
    reserved `user_id` via `IsReservedQueuedBy` + `reservedIdentityError`
    (mirrors `wsUpdateMessage`), default empty `user_id` to `QueuedByUser`.
  - call `queueService.MergeIntoAbove`; map `ErrEntryNotFound` →
    `entry_not_found`, `ErrNoMergeTarget` → `ws.ErrorCodeValidation`, else
    internal error.
  - `publishStatus(ctx, sessionID)` on success; respond `{entry_id}`.
- Add a `queueErrorCodeEntryNotFound`-style mapping reuse — the existing
  constants at the top of the file already cover both codes.

## Frontend

### API client (`apps/web/lib/api/domains/queue-api.ts`)

- Add `mergeQueuedEntry(params: { session_id: string; entry_id: string }): Promise<{ entry_id: string }>`
  sending `message.queue.merge` through `client.request`, wrapped in
  `rethrowQueueError` like `removeQueuedEntry`.

### Hook (`apps/web/hooks/domains/session/use-queue.ts`)

- Add `mergeEntry(entryId: string)` action in `useQueueActions`: call
  `mergeQueuedEntry({ session_id, entry_id })`, then `refetch(sessionId)`; on
  `QueueEntryNotFoundError` refetch (drain race) and rethrow so the caller can
  toast, matching the `editEntry` pattern. No optimistic store mutation (the
  merged entry's content isn't computed client-side). Return it from `useQueue`.

### Components (`apps/web/components/task/chat/`)

- `queued-ghost-message.tsx`
  - Export a mergeability helper:
    `canMergeEntry(entry)` → sender kind is `"user"` or `"agent"`, plus — for
    agent entries — a non-empty `metadata[sender_task_id]`; `canMergeWithAbove(entry,
    above)` → both mergeable and same sender kind **and** equal non-empty
    `sender_task_id` values for agent entries (mirrors the backend
    `mergeAllowed` rule so the UI never offers a merge the backend will
    reject). Reuse the existing `senderKindOf`.
  - Add optional `canMerge: boolean` + `onMerge: () => void | Promise<void>`
    props to `QueuedGhostMessage`; pass through to `DisplayView`.
  - Render a **Merge with above** ghost button (title `t("chat:mergeWithAbove")`,
    `data-testid="queue-entry-merge"`, `IconArrowMerge` from
    `@tabler/icons-react`, same styling as Edit/Remove) before the Edit
    button, only when `canMerge` and the row is not in edit mode.
- `queued-ghost-list.tsx`
  - In `QueuePanel`, compute each entry's `canMerge` as
    `canMergeWithAbove(entry, entries[index - 1])` and pass
    `onMerge={() => onMerge(entry.id)}`.
  - Thread `onMerge` through `QueuePanelProps` and `useQueuePanelHandlers` (new
    `handleMerge` → `mergeEntry`; on failure, `toast.error(t("chat:failedToMergeQueuedMessages"))`
    or, for an over-cap reference union, `toast.error(t("chat:mergeReferenceOverflow"))`,
    matching `handleRemove`).
  - Pull `mergeEntry` from `useQueue(sessionId)` in `QueueAffordance`.

## Tests

- **Repository — user↔user happy path** (`repository_sqlite_merge_test.go`,
  `service_test.go`): merge a user entry into the user entry above it →
  combined content joined with `\n\n`, attachments concatenated, entity
  references unioned/deduped, target identity (id/position/queued_by/queued_at)
  preserved, source row gone, queue count drops by one.
- **Repository — empty target content** (`repository_sqlite_merge_test.go`): target
  with empty content → merged content is exactly the source content (no leading
  blank line).
- **Repository — head rejected** (`repository_sqlite_merge_test.go`): merging the
  head entry → `ErrNoMergeTarget`, queue unchanged.
- **Repository — mismatched kinds rejected** (`repository_sqlite_merge_test.go`):
  user source above agent target → `ErrNoMergeTarget`; agent source above user
  target → `ErrNoMergeTarget`; workflow/system source → `ErrNoMergeTarget`.
- **Repository — agent↔agent** (`repository_sqlite_merge_test.go`): same
  `sender_task_id` → allowed, merged entry keeps target's agent identity;
  different `sender_task_id` → `ErrNoMergeTarget`.
- **Repository — ownership guard** (`repository_sqlite_merge_test.go`): user merge
  with caller `queuedBy` not equal to both rows → `ErrNoMergeTarget`.
- **Repository — reference overflow rejected atomically**
  (`repository_sqlite_merge_test.go`, `repository_postgres_merge_test.go`,
  `service_test.go`): a deduplicated reference union over
  `entityrefs.MaxReferencesPerMessage` → `ErrMergeReferenceOverflow`, both
  rows' content/attachments/references unchanged, queue count unchanged
  (sqlite, postgres, and in-memory repositories all assert the target is
  untouched on the rejected path).
- **Repository — missing source** (`repository_sqlite_merge_test.go`): drained source
  id → `ErrEntryNotFound`.
- **Repository — reserved in-flight target** (`repository_sqlite_merge_test.go`): a
  durable lifecycle target with the reservation marker → `ErrNoMergeTarget`.
- **Repository — three-entry chain** (`repository_sqlite_merge_test.go`): merge `C`
  into `B`, then `B` into `A` → one entry with `A+B+C` in order.
- **Service** (`service_test.go`): `MergeIntoAbove` delegates and returns the
  merged entry (or the mapped error) through the memory repo.
- **WS handler** (`queue_handlers_merge_test.go`): happy path returns `{entry_id}` and
  publishes status; missing `session_id` / `entry_id` → validation; head merge →
  validation; reserved `user_id` rejected; drained source → `entry_not_found`.
- **Frontend hook** (`use-queue` via `queued-ghost-list.test.tsx` or a hook
  test): `mergeEntry` calls the API and refetches; refetch on
  `QueueEntryNotFoundError`.
- **Frontend gating** (`queued-ghost-message.test.tsx`): merge button hidden for
  the first entry, hidden for mismatched sender kinds, hidden for
  workflow/system entries, shown for a user entry behind a user entry and for
  an agent entry behind an agent entry, and clicking it calls `onMerge`.
- **Frontend list wiring** (`queued-ghost-list.test.tsx`): `onMerge` passes the
  clicked entry id to `mergeEntry`.

## E2E Tests

- **Scenario** (spec): two queued messages `A` and `B` with `B` behind `A`; user
  clicks **Merge with above** on `B` → queue contains one message `A + B`, count
  drops by one.
  - **File:** `apps/web/e2e/tests/chat/message-queue.spec.ts`
  - **What to verify:** queue two messages while the agent is busy, open the
    panel, click `queue-entry-merge` on the second row, assert the panel now
    shows a single entry whose text contains both messages and the chip/label
    reflects one queued message. Agent-kind merging is covered by backend + unit
    tests (inter-task dispatch is not practical in this E2E harness).

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-backend-repository](task-01-backend-repository.md)

Wave 2:
- [x] [task-02-backend-service-handler](task-02-backend-service-handler.md)

Wave 3:
- [x] [task-03-frontend-merge-ui](task-03-frontend-merge-ui.md)

Wave 4:
- [x] [task-04-e2e](task-04-e2e.md)
```

All four tasks execute sequentially in the primary conversation. The default is
sequential; `task-01` and `task-03` touch disjoint files and are parallel-safe
candidates **only with a frozen backend contract** — task-03 consumes the
`message.queue.merge` WS action and `mergeEntry` hook from task-02, so it must
not merge before task-02 lands. No subagents run unless the user explicitly
asks.

## Verification commands

All commands pass on the final head (`2b657e61d`, 2026-08-02); CI run
[#30742525141](https://github.com/kdlbs/kandev/actions/runs/30742525141)
(Frontend: lint, i18n checks + ratchet, tests, build — ✓) and
[#30742525164](https://github.com/kdlbs/kandev/actions/runs/30742525164)
(Backend: gofmt, golangci-lint, `go test -race` incl. Postgres — ✓), E2E
[#30742525129](https://github.com/kdlbs/kandev/actions/runs/30742525129) (all
14 shards + containers + desktop smoke — ✓).

- `make fmt` — ✓ 2026-08-02 (gofmt clean on changed Go files)
- `make typecheck test lint` (from repo root per AGENTS.md) — ✓ via CI runs
  above
- Targeted Go: `cd apps/backend && go test -race ./internal/orchestrator/messagequeue/... ./internal/orchestrator/handlers/...` — ✓ 2026-08-02
- Targeted frontend: `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/queued-ghost-message.test.tsx components/task/chat/queued-ghost-list.test.tsx` — ✓ 2026-08-02
- E2E: `cd apps/web && pnpm e2e:run --host tests/chat/message-queue.spec.ts --project=chromium` — ✓ 2026-08-02 (in E2E CI shards above)
