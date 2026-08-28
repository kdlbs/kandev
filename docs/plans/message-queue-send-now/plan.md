---
spec: docs/specs/ui/requirements/message-queue-send-now.md
created: 2026-08-05
status: implemented
---

# Implementation Plan: Send Queued Messages Now

## Overview

Add one explicit WebSocket operation that asks the orchestrator to replace the
active turn with either one queued entry or the click-time snapshot of the
whole visible queue. Build the exact-entry/batch claim and restoration contract
first, then compose it with the existing cancellation coordinator and expose it
through the frontend queue hook and inline panel. Finish with desktop/mobile
Playwright coverage and the public queue-control guides.

Decision inputs:

- [Queue Send Now Replaces the Active Turn](../../decisions/2026-08-05-queue-send-now-replaces-turn.md)
- [Keep Cancellation Progress Backend Owned](../../decisions/2026-08-03-backend-owned-cancellation-progress.md)
- [Explicit User Cancellation May Complete a Workflow Step](../../decisions/2026-08-02-explicit-user-cancel-completion.md)
- [Version Agent-Ready Events by Prompt Generation](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

---

## Backend

### Exact queue dispatch claims

Files:

- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- new focused helpers/tests under `apps/backend/internal/orchestrator/messagequeue/`

Add a `SendNowClaim` value that retains the ordered original entries plus the
synthetic dispatch envelope. Add repository/service operations to atomically
claim an exact ordered list of entry IDs, restore the claim, and acknowledge
its durable lifecycle rows.

The claim transaction must fail without partial mutation when any requested row
is missing, belongs to another session, or is already reserved. Ordinary rows
are removed; durable lifecycle rows receive the existing in-flight reservation
and remain until acknowledgement. Restoration reinserts ordinary rows at their
original positions and clears durable reservations.

Build the bulk envelope from the claimed rows: FIFO content joined by `\n\n`
between non-empty bodies, FIFO attachments, canonical reference union, oldest
entry model/plan mode/attribution, and source identity/provenance metadata.
Validate attachment count/bytes and reference count before cancellation using
the same constants as ordinary message admission.

### Non-completing cancel-and-dispatch

Files:

- new `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`

Add `Service.SendQueuedNow(ctx, sessionID, scope, entryID)` and a distinct
`cancellationKindQueueSendNow`. Authorize the session first, snapshot the active
turn and exact queue selection, and validate the aggregate before cancellation.

Reuse `cancelInFlightGuard`, the backend cancellation projection, captured turn
identity, and silent cancellation reconciliation. Do not call the explicit
`CancelAgent` path: Send Now must not write the ordinary cancellation message,
move the task to review, or evaluate cancelled-turn completion. Reject an
existing explicit cancellation or Send Now owner. If the observed turn changes,
leave the successor and queue untouched; if the session is already promptable,
claim and dispatch without cancellation.

Refactor the queued execution handoff only as far as necessary to accept a
single- or multi-entry `SendNowClaim`. A successful prompt acknowledges every
durable source; retryable admission/execution failures restore every original
source and publish authoritative queue status. Never fall back to the FIFO head
when the selected entry or bulk snapshot cannot be claimed.

### WebSocket action

Files:

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/backendapp/gateway.go` only if constructor wiring must
  name a broader dispatcher interface

Register `message.queue.send_now` with request fields `session_id`, `scope`, and
conditional `entry_id`. Extend the existing drainer dependency into a queue
dispatcher interface implemented by `orchestrator.Service`.

Authorize before all queue reads/cancellation, validate the scope combination,
call `SendQueuedNow`, map the spec's stable error codes, publish status, and
return `{session_id, dispatched: true, sent_count}`. Add the action to the
gateway authorization pinning tests so `session_id` receives the existing
backstop.

---

## Frontend

### Queue API and hook

Files:

- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/lib/api/domains/queue-api.test.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts`

Add `sendQueuedNow` plus typed error mapping for the stable send-now codes. Add
`sendEntryNow(entryId)` and `sendAllNow()` to `useQueue`; both set queue loading,
invoke the action, and refetch authoritative status on success or failure. A
missing/raced entry is a reconciled result plus focused feedback, not a stale
optimistic removal.

### Queue controls

Files:

- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/components/task/chat/queued-ghost-list.test.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/web/src/locales/en/chat.json`
- generated pseudo-locale output if the catalog workflow changes it

Add a per-row **Send Now** button to the existing `RowActions` hover/focus
group and thread the selected entry ID through the queue hook. Add header
**Send Now** immediately before **Clear all** and bind it to `sendAllNow`.
Disable both controls while the queue request or authoritative session
`cancellation_pending` state is true, use the existing spinner/toast patterns,
and localize all labels and failures.

Keep **Run next** unchanged for promptable FIFO-head dispatch. When present, the
header order is **Run next**, **Send Now**, **Clear all**, collapse. Let the
action group wrap on narrow viewports rather than shrink or overflow.

---

## Responsive and Mobile Contract

- **Desktop outcome and mobile entry point:** desktop opens the existing queue
  chip and uses hover/focus row actions or the header; mobile opens the same
  chip and sees both Send Now controls without hover.
- **Nearest shipped exemplar:**
  `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts` supplies the
  inline panel, single scroll owner, visible touch controls, and 44px geometry.
- **Hierarchy and primary action:** queue content remains primary; per-row Send
  Now sits with row actions, while bulk Send Now is the first bulk mutation
  immediately before Clear all.
- **Presentation choice:** retain the inline queue panel because the action is a
  frequent, short mutation of already visible content; no drawer or navigation
  layer is introduced.
- **Geometry and scrolling:** `queue-scroll-region` remains the only internal
  scroll owner. The header action group may wrap, controls are at least 44px on
  coarse pointers, the composer remains visible, and document horizontal
  overflow stays zero.
- **Shared logic:** API calls, loading/cancellation state, selection, aggregation,
  and errors are shared. Only the existing pointer-responsive action visibility
  differs.
- **Mobile proof:** a `mobile-chrome` test taps row/header Send Now, asserts the
  replacement prompt outcome, verifies touch geometry, and checks horizontal
  containment.

---

## Tests

- **What:** exact-entry and exact-snapshot batch claims are atomic, preserve FIFO
  source order, reserve durable rows, restore originals, and acknowledge every
  durable source.
  **Files:** new `repository_*_send_now_test.go` coverage for memory, SQLite,
  and env-gated PostgreSQL plus `service_test.go`.
  **How:** table-driven repository tests for selected/all claims, missing or
  changed snapshots, mixed ordinary/durable rows, restoration, and no
  cross-session mutation.
- **What:** bulk aggregation joins content correctly, preserves FIFO
  attachments, deduplicates references, uses the oldest envelope, and rejects
  attachment/reference overflow before mutation.
  **Files:** new messagequeue send-now helper tests and orchestrator tests where
  task-model attachment limits are applied.
  **How:** deterministic unit tests with attachment-only messages, repeated
  references, mixed provenance, and over-limit inputs.
- **What:** busy Send Now cancels the captured turn and dispatches the exact
  selection without explicit-cancel workflow side effects; promptable Send Now
  skips cancellation.
  **File:** `apps/backend/internal/orchestrator/queue_send_now_test.go`.
  **How:** channel-synchronized lifecycle mocks assert one cancel, no review or
  cancelled-turn completion, exact prompt content, and one successor turn.
- **What:** successor-turn, explicit-cancel, duplicate-click, queue-change, and
  prompt-failure races fail closed or restore the original queue.
  **Files:** `queue_send_now_test.go` and focused existing cancellation tests if
  shared coordinator behavior changes.
  **How:** captured-turn replacement and cancellation-owner race tests without
  sleeps; assert no substitute FIFO dispatch.
- **What:** the WS action validates scope, authorizes before dependencies, maps
  stable errors, and publishes the post-action status.
  **File:** `apps/backend/internal/orchestrator/handlers/queue_handlers_send_now_test.go`.
- **What:** frontend API/hook calls and both controls use the correct scope,
  refetch after every outcome, show localized failures, and disable during
  loading/cancellation.
  **Files:** existing queue API, hook, list, and row component test files.

---

## E2E Tests

- **Scenario:** while a desktop task agent runs a cancellable slow turn, queue
  three distinct messages and activate row Send Now on the second. Verify the
  replacement transcript contains the second prompt, the cancelled turn does
  not advance the workflow, and the first/third rows remain ordered.
  **File:** `apps/web/e2e/tests/chat/message-queue.spec.ts`.
- **Scenario:** while a desktop task agent is busy, queue several messages and
  activate header Send Now. Verify one replacement prompt contains all bodies
  in FIFO order and the queue disappears.
  **File:** `apps/web/e2e/tests/chat/message-queue.spec.ts`.
- **Scenario:** on Pixel 5, open the busy queue panel and tap Send Now. Verify
  the row and header controls are 44px, require no hover, remain inside the
  viewport with no document horizontal overflow, and complete a replacement
  turn.
  **File:** `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`.

---

## Public Documentation

Update the how-to/explanation sections in:

- `docs/public/coordination.md`
- `docs/public/sessions-and-review.md`

Explain the difference between **Run next** (promptable FIFO head), **Send Now**
(interrupt and replace without workflow completion), **Clear all**, and the
normal Cancel control. No new page or navigation entry is required.

---

## Verification Results

- Backend queue claims and envelope aggregation: `cd apps/backend && go test -race ./internal/orchestrator/messagequeue -count=1 -v` — 116 tests passed. The memory and SQLite claim suites cover exact atomic selection, click-time snapshot changes, FIFO restoration, durable reservation, acknowledgement, aggregate attachments, and reference limits. The env-gated PostgreSQL cases were included but skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- Backend replacement orchestration: `cd apps/backend && go test -race ./internal/orchestrator -run 'SendNow|ExplicitCancellationDoesNotJoin' -count=1 -v` — 6 tests passed; `cd apps/backend && go test -race ./internal/orchestrator/handlers -run 'SendNow|QueueHandler' -count=1 -v` — 24 tests passed; existing cancellation/peer-interrupt regressions — 56 tests passed. The retry regression also proves restored ordinary and durable sources retain the recorded-user-message marker.
- Backend compile sweep: `cd apps/backend && go test ./... -run '^$'` — completed with no test bodies selected, compiling all packages successfully.
- Backend lint: `make -C apps/backend lint` — completed with 0 issues after extracting the repository, handler, and orchestration validation helpers.
- Frontend queue API, hook, and component suite: 4 files, 97 tests passed. `cd apps/web && pnpm run typecheck` passed; full web lint passed with `pnpm --filter @kandev/web run lint`; `i18n:pseudo`, `i18n:check`, and `i18n:ratchet` all passed.
- Public documentation: `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed; `node scripts/validate-public-docs.mjs` — 41 published docs pages validated; `git diff --check` passed.
- E2E discovery found 12 desktop queue tests and 2 mobile queue-management tests. After rebuilding and syncing the embedded web assets with `make build-web` and `make sync-embedded-web && make build-backend`, `cd apps/web && pnpm e2e --project=chromium e2e/tests/chat/message-queue.spec.ts --grep 'Send Now' --retries=0` — 2 passed in 33.4s; the matching `mobile-chrome` command for `mobile-message-queue-management.spec.ts` — 1 passed in 13.1s. The desktop row scenario verifies workflow-step preservation and neighboring FIFO entries; the mobile scenario verifies touch geometry and zero horizontal overflow.

---

## Implementation Waves and Parallel Candidates

The default is sequential execution in the primary conversation. The files are
ordered by dependency; these waves do not authorize subagents.

Wave 1:

- [x] [Task 01: Add exact send-now queue claims](task-01-queue-send-now-claims.md)

Wave 2:

- [x] [Task 02: Orchestrate replacement-turn dispatch](task-02-replacement-turn-dispatch.md)

Wave 3:

- [x] [Task 03: Add Send Now queue controls](task-03-send-now-queue-controls.md)

Wave 4:

- [x] [Task 04: Prove Send Now end to end](task-04-send-now-e2e.md)
- [x] [Task 05: Document Send Now behavior](task-05-document-send-now.md)

Tasks 04 and 05 own disjoint E2E and public-doc files, but the final plan/status
and verification-results update is serialized after both tasks complete so the
durable implementation record cannot be overwritten by parallel edits.

---

## Risks and Boundaries

- `task_operations.go` and cancellation coordination are high-contention
  concurrency surfaces. Keep new orchestration in a focused file and preserve
  captured-turn and prompt-generation checks.
- Explicit Cancel can complete/advance a workflow step; Send Now must never
  reuse or join that semantic path.
- Bulk delivery can mix provenance and durable lifecycle rows. The batch claim
  must preserve originals for retry and acknowledge every durable source only
  after prompt acceptance.
- Aggregate attachment/reference limits must be checked before cancellation so
  a validation error cannot stop useful active work.
- New queue entries accepted after the click-time snapshot remain pending and
  must not leak into the replacement prompt.
- No schema migration, feature flag, queue reordering, arbitrary subset
  selection, model picker, or change to ordinary FIFO drain is planned.
