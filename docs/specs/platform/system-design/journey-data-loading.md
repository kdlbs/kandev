---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-006
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
  - REQ-PLATFORM-JOURNEY-LOADING-001
  - REQ-PLATFORM-JOURNEY-LOADING-002
  - REQ-PLATFORM-JOURNEY-LOADING-003
---

# Journey Data Loading System Design

## Boundary and requirement mapping

Platform owns bounded delivery and shared read availability. Domain repositories retain authorization, authoritative records, and mutation guards.
This design applies the existing reader and task-summary contracts to homepage and task-detail loading.
It also defines the initial boot graph and incremental turn-context extension.
The [investigation](../../../plans/journey-data-efficiency/evidence.md) records the current excess work and controlled lock evidence.

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-INTERACTIVE-READS-006 | Reader snapshots; Batched summary observations |
| REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001 | Visible detail demand; Compact status continuity |
| REQ-PLATFORM-JOURNEY-LOADING-001 | Route boot graph; Responsive behavior; Compatibility |
| REQ-PLATFORM-JOURNEY-LOADING-002 | Message-window turn context |
| REQ-PLATFORM-JOURNEY-LOADING-003 | Shared read ownership |

## Reader snapshots

`Repository.GetTaskCompletionGate` currently starts `r.db.BeginTx`. The configured SQLite writer reserves at BEGIN, even when the body only reads.
Move this standalone inspection to the existing `r.ro` pool. Use a native read transaction and the existing gate reader with `lockEvidence=false`.
SQLite uses its deferred reader transaction. PostgreSQL uses a read-only repeatable-read snapshot for this standalone multi-query observation.
Close rows before the next statement, settle the transaction on every exit, and preserve missing-task errors.

Mutation paths remain on the writer. `guardTaskCompletionTransitionTx`, criterion edits, verification receipts, and exact-command replay retain their supplied transaction and evidence locking.
A read snapshot never authorizes completion. The mutation rechecks current evidence inside its existing atomic boundary.
No writer DSN change, new connection pool, manual BEGIN facade, busy retry loop, or timeout increase is needed.
This follows the [writer admission ADR](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).

A factory-created, file-backed test must hold the writer while standalone gate, homepage, and workflow snapshot reads finish.
Alias fixtures that use one handle for reader and writer cannot establish this property.
Cancellation and SQL errors must release reader capacity. Mutation concurrency tests must still prove stale evidence cannot complete a task.

## Batched summary observations

Add a typed batch gate-summary read to the task repository contract. The proposed name is `GetTaskCompletionGateSummaries`.
It returns keyed completion-summary observations and explicit missing-task information, not full gate histories.
Use chunks of at most 100 task IDs. Within each chunk, one reader snapshot loads task identity, set revisions, criteria, and supported evidence sources in sets.
Use at most six data queries per chunk: identity, sets, criteria, task-revision evidence, PR-head evidence, and execution evidence.
Artifact evidence remains an immutable host-issued revision. Criteria evaluation reuses the existing pure validity rules.
Do not issue a query per criterion or silently treat a failed evidence read as an unblocked gate.

`ReconcileTaskStatusSummaries` receives this keyed observation once, before its per-task semantic comparison.
Both existing-summary repair and missing-summary rebuild consume it. Standalone full gate inspection retains its public shape.
Service event paths can continue to observe one task through the same batch contract.
For 1,000 tasks, gate work is at most ten read snapshots and sixty data queries, independent of session count.

Replace list enrichment's `BatchGetSessionsByTaskIDs` dependency with narrow summary observations.
The proposed `BatchGetTaskSessionSummaryObservations` returns session identity, primary identity, state, foreground activity, profile/executor labels, and fields actually consumed by summary logic.
Project required error fields and the selected model ID/name through existing dialect helpers. Inactive session tabs retain their current model label. Exclude other model choices, descriptions, configuration options, and runtime settings. Do not decode or return entire session metadata or configuration snapshots.
Preserve session counts, pending-action precedence, primary fallback, error clearing, queue state, and runner mutability.
Batch primary information with these observations instead of rereading full primary sessions.

Compact `GET /tasks/:id/sessions` receives a narrow list projection with its existing response fields.
Full `GetTaskSession` and mutation callers keep full models. A compact row must not overwrite an already loaded rich field with an omitted value.
Audit consumers before migrating a repository method. New narrow methods coexist with full-model methods until every list caller uses the correct projection.

Persisted status summaries remain rebuildable and revision guarded. Missing/stale records still repair through existing bounded compare-and-update logic.
No-op repair writes nothing. Repeated failures preserve the last valid summary and existing error handling.
Do not collect a whole board's repairs into a single long writer transaction or turn repair into an unbounded background queue.
This package removes unnecessary inspection reservations; necessary repair writes can still wait behind a writer.

## Visible detail demand

Desktop `ensureSiblingPanels` creates tabs for all siblings. `defaultRenderer="always"` preserves their React trees.
Keep panel and draft retention, but give each detail consumer explicit demand independent of mounting.
Add a `detailActive` input through `TaskChatPanel` and `useChatPanelState` to session/message/turn loaders and detail-dependent hooks.
For Dockview, derive it from panel visibility in its group, not keyboard focus. Two visible groups can own two sessions even if only one group has focus.
Reuse `usePanelActive`: despite its name, it already reads `api.isVisible` and observes `onDidVisibilityChange`. Do not replace it with Dockview global focus.

Do not reuse `isVisible` blindly. Existing previews use that property to suppress read acknowledgement while still displaying a conversation.
A visible preview has `detailActive=true` independently of unread/read tracking. Embedded Office and Threads surfaces retain their own explicit viewport demand.
Hidden sibling tabs have no session/conversation registration, session snapshot polling, transcript loading, MCP/model hydration, usage reads, or task-level CI/repository fetches.
Retained cache and local composer state remain available without active effects.

Keep the WebSocket client's reference counting. Same-session duplicate views share transport membership.
On reveal, acquire readiness once, reconcile the newest message window and relevant detail, then resume live delivery.
A retained transcript can remain visible with existing refresh state. It is not newly verified until recovery succeeds.
On hide or final release, unsubscribe and cancel owned requests/timers. This releases browser demand only: it never stops an agent, dispatches a prompt, changes task ownership, or acknowledges unread content. One visible consumer must not lose its request because another consumer hides.
Newer keyed events win over late fetches. Results never change selected task or reopen a dismissed panel.

## Compact status continuity

Use existing `task.status_summary.updated`, `session.state_changed`, and `session.pending_action_changed` delivery.
The pending-action event and revision-aware frontend handler already exist. This package does not add another event family.
Loaded sibling selectors receive lifecycle, foreground, and attention state without rich subscriptions.
Reconnect refreshes the selected task's compact membership once. Offscreen/unselected task rows keep workspace task summaries only.
Never repair an unavailable compact field by subscribing to every sibling.

[Bounded task status](bounded-task-status-delivery.md) owns badge semantics.
[Viewport session delivery](viewport-bounded-session-delivery.md) owns the Threads preload/detail window.
Quick Chat's closed-modal completion/unseen behavior is deliberately excluded until its compact completion contract has its own evidence and design.

## Route boot graph

`boot_state.go` currently serializes overlapping Zustand slices directly. Introduce a version-2 boot graph at `internal/webapp` and `src/boot-payload.ts`.
The proposed wire groups are `entities.tasksById`, `entities.sessionsById`, and route memberships containing IDs. Use existing TaskDTO and TaskSessionDTO field names in those entries. Preserve field presence when decoding compact rows; route membership identifies which session entries contain full detail.
The graph stores each task/session once. A selected full record extends its canonical entry instead of adding another wire copy.
Board membership contains task IDs, workflow metadata, steps, and explicit coverage. Sidebar membership contains its existing page projection with task IDs.
Selected task membership contains task ID, selected session ID, and ordered sibling IDs.
Other independent boot fields, plugins, auth, runtime, settings, and security interlock retain their contracts.

A frontend decoder converts this graph to current hydration inputs before route components mount.
Reuse `taskOverview`, `withTaskOverviewNormalization`, and current session merge actions. Do not introduce a second mutable task store.
Compatibility arrays can exist in memory as selectors over canonical objects. They do not reappear in the wire representation.
Selected task descriptions remain complete. Board card content and status precedence do not change merely to meet a fixture budget.

### Homepage

Resolve workspace, selected workflow, and saved view through existing precedence.
For single-board mode, load only that workflow snapshot. Keep workflow identities and lightweight coverage metadata for navigation.
For multi-board mode, seed the first displayed workflow. Additional board containers request snapshots when visible or in the existing adjacent preload region.
Do not mount `useAllWorkflowSnapshots` solely to populate a sidebar. Nonselected boards show their existing loading state until their snapshot arrives.
A missing selected workflow uses the established authorized fallback, not another workspace's cache.

### Task detail

Load task identity, compact sibling membership, and the authorized requested session first.
Pass the request query into `taskDetailRouteData` and share the existing primary/eligible-sibling selection rule with client navigation.
An invalid or foreign-task session ID supplies no detail and falls back to the authorized task's primary/eligible sibling. No sessions means a valid task with no active session.
Do not load the primary transcript and then fetch the requested transcript.

Load one sidebar page through the existing saved-view query, with its 100-row limit, account/workspace identity, filters, ordering, and page metadata.
Do not load a complete workflow snapshot solely to populate task navigation.
If an optional sidebar read fails or is cancelled, omit its membership and preserve the detail outcome. The sidebar's existing controller owns later recovery.
Initial task/session membership is independent from repository scripts, CI options, model settings, and other panel-owned enrichment.
Task actions use the bounded sidebar row or canonical task record. A workflow move keeps the selected detail record authoritative after it leaves a loaded board.
Step filters and destination menus request lightweight workflow steps on explicit demand. These reads share flights with route and detail metadata. They do not request destination board tasks.
Those resources load when a mounted active consumer needs them.

[Shared sidebar state](../../ui/system-design/sidebar-shared-task-state.md) remains authoritative for coverage, local evaluation, retention, tombstones, and authorization barriers.
A partial boot page never establishes workflow or workspace completeness. Already complete compatible board data can still satisfy a sidebar locally.
Cold archived views remain bounded server queries. This package changes neither sidebar filters nor saved-view semantics.

## Message-window turn context

Add optional `include_turns=true` to the existing HTTP message-list request and `include_turns` to `message.list`.
When requested, the response adds `turns` and `turn_coverage` beside existing messages and pagination fields.
The proposed coverage shape identifies the returned message IDs and current active turn ID, scoped to the session.
The repository reads messages, their distinct turn IDs, corresponding turn metadata, and current active turn through one reader snapshot.
At most one turn row per distinct returned message turn plus one active turn is needed. Boot retains its 50-message window, so at most 51 turn rows. Client cold/reconnect reads retain their current 100-message window, so at most 101 turn rows.
Do not use an arbitrary last-N-turn limit, which can miss context for sparse or long-running turns.

Boot uses the same window builder. Older-message pagination and search-result windows request their own turn context.
Preserve author ordering, timestamp precision, deletion behavior, lifecycle-turn exclusions, usage associations, and messages with no turn ID.
The existing full `GET /task-sessions/:id/turns` remains unchanged for callers that need it.

The store records coverage per accepted message window/readiness generation. It does not set the global full-history `loadedBySession` flag for partial data.
Replace the unconditional full-history call in `useSessionMessages` with window coverage checks.
Boot coverage satisfies initial consumers. A reconnect gap invalidates live completeness and refreshes the newest window without discarding safely cached older windows.
Turn completion events merge by existing freshness rules. A late snapshot cannot resurrect an active turn after a newer accepted completion.
Bound bookkeeping by retained message windows rather than retaining a new unbounded coverage ledger.

## Shared read ownership

Use store-instance scoped resource owners, following `agent-list-resource.ts` and `TaskNavigationReads`.
Do not add process-global maps keyed only by task or session ID. Different stores/accounts must not share private data.
Migrate existing request paths rather than adding a cache beside them.

| Resource | Key beyond store/auth generation | Consumers |
| --- | --- | --- |
| Compact task sessions | task ID and membership generation | route identity, tabs, sidebar expanded selected task |
| Workflow snapshot | workspace, workflow, coverage/options | visible boards and route consumers that need the board |
| Repositories | workspace and `includeScripts` | active task/panel consumers |
| Workflows/workspaces | workspace where applicable and include-hidden option | navigation and route consumers |
| User settings / agents | authenticated user and response options | app shell and active controls |
| Task CI options | task and integration scope | visible task controls |
| MCP configuration | profile and selected session where applicable | visible chat controls |

Keep rich-session polling under its existing shared reconciler. This package does not introduce a new cheap-revision endpoint or alter polling cadence.
Use existing owners where present, including agent list and integration health. Avoid a broad data-library migration.

Each owner exposes current value, error, freshness, subscribers, and one in-flight attempt.
Concurrent ensures reuse the promise. A refresh during execution marks one trailing request. Repeated invalidation cannot reset a failure cooldown.
Retain the existing endpoint-specific retry rules. Task navigation still follows the interactive-read recovery budget.
A final release cancels unneeded work, while scope invalidation cancels all obsolete work. Late responses must pass generation and authorization checks.

Hydration seeds the owner with value and freshness. Initial connection does not itself invalidate successful boot data.
For workspace data, open the workspace/user feed before one authoritative gap-closing repair, so events around boot cannot be lost.
Coalesce that necessary repair with all initial consumers. Distinguish it from unconditional repeated per-hook refreshes.
Resource-count tests allow one such documented repair per generation, not five identical requests.
A later reconnect or explicit mutation still invalidates the relevant resource and repairs its state.

## Responsive behavior and visible states

The existing layouts remain. Desktop keeps sidebar, sibling tabs, visible split chats, and other task panels.
Phone keeps direct task navigation and `SessionTaskSwitcherSheet` / `MobilePickerSheet`, with one focused chat.
These shipped surfaces are the mobile exemplars. Task selection is temporary drawer navigation; conversation content keeps its full-height dedicated surface.
Existing scroll owners, safe-area clearance, keyboard focus, and touch controls remain. No new two-axis phone layout or hidden desktop mount is introduced.

A hidden desktop tab can show compact activity/permission state. Revealing it retains its draft and uses existing loading/refresh/error UI for detail.
Cold sidebar loading, a successful empty page, stale retained data, and failure remain distinct.
Phone-to-desktop transitions do not overwrite saved desktop layout or drafts. All new copy, if required, uses seven locale catalogs.

## Compatibility, persistence, and security

| Boundary | Compatibility and failure behavior |
| --- | --- |
| SQLite | Existing WAL reader/writer pools. Reader isolation tested with real separate factory handles. No migration required. |
| PostgreSQL | Standalone coherent read snapshot; mutation isolation and lock order unchanged. Real conformance execution required, not a skipped test claim. |
| Boot | New frontend accepts version 1 and version 2. Static HTML references its matching asset build. Unsupported future versions use bounded ordinary route recovery, not a reload loop. |
| HTTP/WS messages | Optional request field and additive response fields. Existing message and full-turn clients keep their shapes. If an older backend omits coverage, use the existing legacy turn read once per scoped readiness generation. |
| Compact session lists | Preserve existing response fields. Missing rich fields are omission, not explicit clearing. |
| Authorization | Resolve account/workspace/task membership before projection. Requested session IDs are untrusted. Denied results never enter shared caches. |
| Plugins | Public host APIs keep their established DTOs. Boot graph is internal. No plugin depends on private hydration slices. |

No new runtime flag, public setting, schema cache, or durable read model is required.
If implementation reveals a necessary migration or new public capability, revise the package before changing that boundary.
Public documentation needs an audit for the additive message option and backend read guidance. Tests and work orders own any required documentation updates.

## Measurement and acceptance

Use the retained synthetic fixture from `docs/plans/journey-data-efficiency`, with 10 and 1,000 tasks and the same selected board.
Add a 10,000-task variant by appending tasks outside that selected board. Keep selected task histories and metadata constant for payload comparisons.
Separately increase older selected-session turns from 200 to 2,000 to prove initial window independence. Keep the newest messages and fixture clock fixed.
Add active mock sessions, stale/missing summaries, completion criteria for all evidence kinds, archived tasks, permissions, and a second unauthorized workspace for correctness tests.

Structural gates are deterministic: zero writer reservations for warm inspection, bounded gate batches, narrow session projections, unique boot records, subscription sets, and per-generation request counts.
Reference payload gates are 512 KiB for task detail and 1.25 MiB for a single board. Count compact serialized JSON before compression.
New unrelated tasks must add zero records. Changes in workflow counts or metadata can change a few bytes and must be reported separately.

Retain exact source revision, fixture manifest, engine, machine, mode, request options, response bytes, request counts, and subscription lifecycle traces.
Collect reader pool waits and writer pool wait/occupancy separately through test instrumentation and the existing diagnostic observer.
Do not add task/session identifiers as metric labels. Existing writer pool counters alone cannot identify transaction ownership.

Run ten warm baseline/candidate cycles after one warmup on the same host, sequentially and with identical data.
Report median, p95, and errors for homepage, one-session detail, and eight-session detail under idle and the existing writer workload.
Do not claim the historical 503 cause solved. Structural gates must pass even when host noise prevents a reliable latency comparison.
A repeatable latency or health regression blocks completion until explained and corrected. Record remaining writer-workload health failures honestly.

## Decisions and related delivery

The accepted reader/writer boundary and existing normalized task ownership supply the architectural rationale.
No new ADR is necessary for these applications of existing rules. An extra pool, generic cache, or removal of all consistency transactions is not selected.
Retaining every hidden rich subscription trades avoidable work for cache warmth; explicit demand preserves drafts and cache without that traffic.

- [Writer admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
- [Normalized task ownership](../../../decisions/2026-09-29-shared-sidebar-task-state.md).
- [Interactive read recovery](interactive-read-availability.md).
- [Bounded task status](bounded-task-status-delivery.md).
- [Implementation package](../../../plans/journey-data-efficiency/plan.md).
