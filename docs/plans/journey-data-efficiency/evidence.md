# Journey data efficiency investigation

Status: investigation complete. Production changes remain proposals.

Date: 2026-10-09. Source: `b8b740f128993b804745704591374f5a9fc7b0ab`, with the existing diagnostic worktree changes.

## Findings and priority

### 1. Completion-gate reads acquire the writer for every task

This is the strongest new database finding. `GetTaskCompletionGate` starts a transaction through `r.db`, the writer pool. Its read path performs SELECTs. The SQLite writer uses `BEGIN IMMEDIATE`, even for this read operation.

Task-summary reconciliation calls this method for each task, including unchanged summaries. A homepage with 1,000 tasks therefore traverses 1,000 individual gate transactions. This count follows the source path, rather than a SQL trace counter. Each gate without criteria still reads the task, completion-set revision, and criteria.

The isolated lock control confirms the dependency:

- A warm homepage normally completed in about 181–199 ms during the control sequence.
- With an external writer held for one second, the homepage took 1,203 ms. It did not finish before release.
- A second control held the writer while requesting the homepage, board snapshot, and session detail concurrently.
- Homepage and board requests waited. Session detail completed in 9.4 ms before release.
- Captured goroutine stacks place both blocked requests inside `GetTaskCompletionGate`. One waited for SQLite admission. The other waited for the shared writer connection.

The proposed change moves standalone gate reads to a consistent reader snapshot. A batched gate-summary read then removes the per-task transaction and query loop. Actual completion transitions must retain their writer transaction, evidence locks, revision checks, and atomic decision.

Sources: `apps/backend/internal/task/repository/sqlite/completion_gates.go:273`, `apps/backend/internal/task/service/service_status_summary_rebuild.go:355`, `apps/backend/internal/db/sqlite.go:40`.

Evidence: [lock control](evidence/lock-control.json), [blocked stacks](evidence/blocked-completion-gate-stacks.txt), [database probe](evidence/db-probe.json).

### 2. Hidden desktop chat tabs load full session detail

The production frontend subscribed to all eight sessions on the multi-agent task. All eight remained subscribed after the initial capture. Only one chat tab was active. Each session also fetched its complete turn history.

Desktop creates inactive sibling panels and uses `defaultRenderer="always"`. `ChatContent` passes visibility to `TaskChatPanel`, but `useChatPanelState` does not receive it. Session and message hooks therefore remain active in hidden tabs.

The phone layout subscribed to one session. Desktop navigation to a different task released all eight old session subscriptions. Ordinary sidebar tasks did not create rich session subscriptions in either capture. This is excess desktop demand, not evidence of a general unsubscribe leak.

The proposed change keeps tab identity, drafts, and cached history separate from live data demand. Only visible chat panels acquire conversation subscriptions and detail loaders. Multiple visible split panels remain supported. Hidden tabs use compact session status and attention events.

Sources: `apps/web/components/task/dockview-session-tabs.ts:363`, `dockview-desktop-layout.tsx:422`, `dockview-panel-content.tsx:96`, `task-chat-panel.tsx:1158`, `chat/use-chat-panel-state.ts:477`.

Evidence: [production desktop captures](evidence/browser-production-1000.json), [navigation and phone capture](evidence/navigation-production.json), [settled membership](evidence/settled-subscriptions.json).

### 3. Initial boot data grows with unrelated navigation data

Kandev serves a Go-generated JSON boot payload inside the SPA HTML. React does not perform server-side rendering here. The `lib/ssr` directory name describes older loading code.

Homepage boot loads every workflow snapshot. It serializes the selected board again in `kanban.tasks`. Task-detail boot loads the selected workflow snapshot and serializes its tasks in both `kanban` and `kanbanMulti`.

| Journey | 10 tasks: boot bytes | 1,000 tasks: boot bytes | 1,000-task contents |
| --- | ---: | ---: | --- |
| Homepage, one board selected | 43,038 | 2,993,184 | 1,000 snapshot tasks plus 495 selected-board copies |
| Task with one session | 153,485 | 2,106,627 | 495 board tasks serialized twice |
| Task with eight sessions | 219,957 | 2,173,099 | Same board duplication plus all eight full session records |

These are compact JSON byte counts, before compression. They exclude JavaScript assets. They are not browser memory measurements.

Task boot also stores its full session rows in two state collections. It loads 50 messages but all 200 turns for the chosen session. The client then requests those turns again because boot omits the `turns.loadedBySession` marker.

A direct link with `?sessionId=...session-3` still boots primary session 0. The client fetches session 3 afterward. The server route-data builder does not receive the query parameter. Any correction must validate membership before selecting the requested session.

The proposed change uses one canonical task collection, compact navigation projections, and explicit coverage metadata. Single-board views load that board. Multi-board views retain the data their visible boards require. Task pages load a bounded sidebar page and selected detail. An incomplete sidebar scope must never claim complete local coverage.

Sources: `apps/backend/internal/backendapp/boot_state.go:138`, `:596`, `:680`, `:1033`, `:1118`, `:1216`.

Evidence: [10-task boot samples](evidence/boot-10.json), [1,000-task boot samples](evidence/boot-1000.json).

### 4. Compact responses still require broad database reads

Both boot and workflow snapshot enrichment call `BatchGetSessionsByTaskIDs`. This loads full session rows, metadata, configuration snapshots, execution joins, and worktrees. Another query loads primary-session information.

The 1,000-task fixture has 2,005 sessions and 8,258,595 bytes of session metadata alone. Board summaries do not need that metadata. The fixture intentionally makes this cost visible. It does not represent a measured production metadata distribution.

The proposed change uses dedicated summary projections for board and sidebar reads. Compact task-session lists need their own narrow repository query. Full session hydration remains available for selected detail and command paths.

Summary reconciliation also scans task activity sources and can repair persisted summaries. Removing all fixture summaries caused one homepage request to insert 1,000 summary rows. The next warm request caused zero summary mutations. Zero mutations does not mean zero writer occupancy: completion-gate reads still acquired the writer.

Repair deduplication and bounded repair batches are candidates after the reader fix. Stale permission, clarification, completion, and launch-queue state must remain correct. A bulk repair must not become one long transaction that blocks all writers.

Sources: `apps/backend/internal/backendapp/boot_state.go:734`, `apps/backend/internal/task/handlers/task_http_handlers.go:274`, `apps/backend/internal/task/repository/sqlite/session.go:3888`, `task_status_summary.go:69`.

### 5. Initial hydration repeats shared requests

The production build still issued repeated requests during the four-second observation window:

| Journey | Fetch calls | Session subscriptions | Full turn-history reads |
| --- | ---: | ---: | ---: |
| Homepage | 71 | 0 | 0 |
| One-session task | 52 | 1 | 1 |
| Eight-session task | 116 | 8 | 8 |
| Explicit sibling-session link | 127 | 8 | 9 |

Fetch counts include agent logos and integration probes. They are observations from one browser run, not stable request budgets. The one-session task fetched its session list five times. The eight-session task fetched repository data nine times and task CI options eight times.

Shared hooks need one request owner per resource and refresh generation. `useTaskSessions` has a per-hook request reference, while multiple consumers force refresh on connection. Workflow snapshot effects also react to connection changes after boot hydration. Route enrichment fetches shared navigation resources again.

The proposed change deduplicates simultaneous reads by resource key and uses boot freshness markers. Reconnect repair remains necessary. It must merge revisions safely and cancel obsolete navigation work.

Sources: `apps/web/hooks/use-task-sessions.ts`, `apps/web/hooks/use-workflow-snapshot.ts:128`, `apps/web/lib/ssr/session-page-state.ts:450`, `:630`.

## Additional candidates and existing protections

- Quick Chat deliberately subscribes to every persisted quick-chat session while its modal is closed. Compact completion and unseen-state delivery can replace this demand. The fixture has no quick chats, so this finding is source-based.
- Busy-session reconciliation polls every 750 ms for up to 30 seconds. Its ETag prevents response-body transfer, but the backend reads and serializes the full DTO before returning 304. A cheap revision or status endpoint can reduce that cost. This fixture uses idle sessions and does not measure active polling.
- Sidebar queries already use the reader pool and a consistent transaction. SQLite temporary candidate tables, indexes, and ANALYZE consume reader time, not the main writer lock. Large filtered pages can occupy one of four reader connections.
- Task, message, and turn reads already use `r.ro`. Increasing the writer pool does not remove SQLite's single-writer limit.
- Representative query-plan checks used workflow and session indexes. Some sort operations still used temporary B-trees. These simplified checks do not justify a new index without a complete query benchmark.
- The session WebSocket client reference-counts subscriptions. The gateway avoids repeating initial data for duplicate membership. Those protections do not help when hidden panels request different sessions.

Sources: `apps/web/components/quick-chat/quick-chat-provider.tsx:75`, `apps/web/hooks/domains/session/session-state-reconciler.ts:9`, `apps/backend/internal/task/handlers/task_http_handlers.go:659`, `apps/backend/internal/task/repository/sqlite/sidebar_task_query_snapshot.go:26`, `apps/web/lib/ws/client.ts:535`, `apps/backend/internal/gateway/websocket/client.go:394`.

## Measurement gates for the proposed changes

1. **Reader isolation:** warm homepage and snapshot reads complete while a separate connection holds the writer. Gate reads use zero writer transactions. Completion-transition race tests still pass.
2. **Batching:** gate query and transaction counts grow with bounded batches, not task count. Metadata size and unrelated session history do not increase summary-query payload.
3. **Subscription scope:** desktop with eight sibling tabs and one visible chat has one rich subscription. Two visible split chats have two. Phone has one. Sidebar-only tasks have none.
4. **Lifecycle:** tab switches, task switches, reconnects, and unmounts release obsolete subscriptions. Drafts survive. Compact status, permission, clarification, and unseen indicators remain live.
5. **Boot scope:** task detail size stays bounded when unrelated tasks increase. Primary and explicit-session links hydrate the intended session once. Task records appear once in the wire payload.
6. **Request ownership:** one session-list request per task and refresh generation. One shared repository/settings request per scope. A successful boot does not immediately repeat the same turn-history read.
7. **Contention:** repeat the existing writer workload with concurrent journey requests. Compare writer wait duration, occupancy, route p50/p95, error counts, and health deadlines.

New instrumentation needs reader-pool wait statistics and operation-level writer attribution. The existing `db_writer_pool_stats` counter cannot identify the operation responsible for a wait.

## Test setup and limits

The isolated instance used synthetic data only. The final database contains 1,000 tasks, three fixture workflows, 2,005 sessions, 1,800 turns, and 3,600 messages. An empty default workflow also exists. The two detail targets have one and eight sessions. Each target session has 200 completed turns.

The seed expanded from 10 tasks to 1,000 without moving existing rows. The selected board therefore contains 495 tasks. Session metadata contains 4 KiB of synthetic padding. The fixture has no repositories, live executors, completion criteria, or quick chats.

Desktop captures used 1440 × 1000. Phone used 390 × 844. The frontend came from `pnpm --dir apps --filter @kandev/web build:vite`. The backend served static production assets with isolated mock providers. Development StrictMode captures were exploratory and are excluded from the final request table.

HTTP 404 responses for missing live environments are expected fixture limitations. No 503 occurred in the retained journey captures. A large development capture and one phone locator check timed out because their target text was hidden or outside the visible sidebar page. Corrected captures completed.

Three direct requests per route supplied the boot-size samples. Timing medians ranged from 31–116 ms with 10 tasks and 111–185 ms with 1,000 tasks. Shared-host load, cache state, and tiny sample sizes prevent a performance SLA or p95 claim.

The lock experiment holds an external SQLite writer deliberately. It establishes a blocking dependency, not the historical incident's writer identity. The earlier 503 incident and separate agentctl file failures retain their previous unresolved attribution.

The retained seed passed `PRAGMA integrity_check` and `PRAGMA foreign_key_check`. Diagnostic triggers were removed before backup. The owned browser and isolated backend were stopped after capture. No production source or permanent application test changed.

Related context: [writer contention investigation](../database-writer-contention/evidence.md), [pending message-update optimization](../message-update-writer-occupancy/plan.md), [viewport session design](../../specs/platform/system-design/viewport-bounded-session-delivery.md), [sidebar shared state](../../specs/ui/system-design/sidebar-shared-task-state.md).
