---
id: "04-build-conversation-host-facade"
title: "Build browser conversation Host facade"
status: done
wave: 3
depends_on:
  - "01-publish-browser-conversation-contract"
  - "02-add-plugin-conversation-reads"
  - "03-extend-task-panel-capabilities"
plan: "plan.md"
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-003
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-004
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.1 (scope revision, CAS, and atomic page lifecycle)
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.12
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.14
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.15 (facade rebind and projection)
  - AC-PLUGINS-PROMPT-HISTORY-HOST-004.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-004.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-004.3
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
---

# Task 04: Build Browser Conversation Host Facade
## Scope
Implement `host.conversation` and the curated prompt-mention renderer behind the published SDK types. Own request/subscription races, pagination, lifecycle cleanup, DTO mapping, favorite reactivity, and private custom-prompt access inside the host.

## Acceptance

- Nullable-session hooks return stable empty state, authorized snapshots, bounded older pages, and `PluginConversationError` values whose required `retryable` field follows the 400/401/404 versus authorized-5xx mapping. Each panel scope owns a monotonic `continuation_revision` from sentinel `0`, incremented on query change, retry, and close, over cursor/token/cutoff/fingerprint/generation; renewal and `loadMore` serialize dispatch per scope and CAS-install only matching revisions, atomically commit page plus next cursor/`hasMore`, discard stale responses, and leave state unchanged on failed renewal.
- `loadMore()` resolves to the number of newly projected messages. Concurrent
  calls for one committed continuation join and resolve to the same count;
  exhausted, closed, and terminal handles resolve `0` without network activity.
  Retryable transport or renewal failures reject with `PluginConversationError`
  while preserving committed state; non-retryable query failures also reject
  and remain in `error`. `retry()` starts a fresh snapshot only for a live
  retryable handle.
- Each mounted `PluginTaskPanel` has a Host-owned scope provider with independent cache, consumer identity, and abort controller. `conversation.history` is authoritative; global `host.conversation` resolves only the nearest scope. Unmount or plugin/panel/task/session/generation change aborts work, releases the consumer, and makes the handle return stable empty state without network activity. Two concurrent panels cannot share scope state; wrong IDs are rejected. The loader's per-generation isolated registry/resource ledger is staged without global visibility, rejects duplicate or foreign ownership, atomically swaps contributions at commit, and discards failed stages without partial notifications.
- Subscription readiness precedes snapshot resolution; message and turn start/completion events reconcile deterministically; reconnect reconciles; deletes and session removal cannot be resurrected by queued updates.
- `session.removed` retains projected rows, sets the hook's `removed` state and message `hasMore` to `false`, and makes later reads/retries no-ops without network activity.
- The facade does not initialize or mutate the chat transcript cache and exposes no Kandev HTTP/event/store types.
- `useMessageFavorite` reacts to the existing session-storage store without exposing toggle/store access.
- `host.ui.PromptMentionText` owns custom-prompt loading and native fine/coarse-pointer previews through a minimal public prop shape.
- The facade filters every buffered/live event by exact task/session identity before timestamp or sequence merging; focused tests cover wrong-session and wrong-task payloads. Omitted `taskId` resolves to the panel task before sending, explicit `null` sends as absent `task_id`, and mismatched strings fail without network activity.
- Snapshot tests block the HTTP response while add/update/delete/session-removal events arrive, then assert ordered replay and no resurrection through reconnect.
- Facade integration tests consume the production ordered WebSocket envelope and `session.removed` terminal event; unit tests may use fixtures only for reducer edge cases.
- Every snapshot/live DTO has required `updatedAt`; the facade applies the documented timestamp-less-add/update policy.

- Reconciliation uses event sequence as ordering and fencing authority; `updatedAt` is freshness metadata only, equal or missing timestamps defer to sequence, and buffered events are released only after the snapshot-cutoff commit.

## TDD sequence

1. RED: add hook/runtime tests for null state, authorization errors, readiness race, pagination joining, concurrent renewal/expiry-between-pages, update/delete ordering, turn completion updates, reconnect, stale generations, two concurrent panels, unmount races, failed/overlapping successor staging with no partial notifications, duplicate ownership, lifecycle cleanup, favorites, prompt mentions, and concrete Host-to-SDK assignability.
2. GREEN: implement the plugin-scoped client, hook cache/reconciler, and UI wrapper in the Host builder.
3. REFACTOR: share existing message timestamp/merge helpers where their contracts match; keep the public DTO independent from private state.

## Likely files

- `apps/web/lib/plugins/host-api.ts`
- new focused conversation client/hook modules under `apps/web/lib/plugins/`
- `apps/web/lib/plugins/types.ts`
- `apps/web/components/task/chat/messages/prompt-mention-components.tsx` only for a narrow wrapper if required
- `apps/web/lib/plugins/host.ts` lifecycle resource tracking and staged generation registration
- `apps/web/lib/plugins/registry.ts` isolated contribution/resource ledgers and atomic commit
- focused tests beside the implementation

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/plugins components/task/chat/messages/prompt-mention-components
cd apps/web && pnpm run typecheck
```

Do not copy the core prompt-history component into Kandev or expose `host.store` fields.

## Current result

Implemented the panel-scoped browser facade with subscription-before-snapshot
readiness, hydration, joined continuation paging, renewal, ordered
reconciliation, reconnect, poison fencing, atomic replacement-cursor rebind,
terminal removal, lifecycle abort, generation refresh, read-only favorites,
curated prompt mentions, and scoped desktop/mobile navigation. The production
core adapter validates the same versioned envelope and preserves
projection-before-ACK behavior. Concurrent panel isolation and registry
successor staging are covered by focused tests.

Verified:

- Focused browser Host, scope, registry, and task-panel suites
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run lint`
