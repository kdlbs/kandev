# User journey read-efficiency audit

Date: 2026-10-06. Status: source audit and focused reproductions complete.

This report extends the [availability repair plan](plan.md). It covers Home,
Kanban, task details, task navigation, and sending messages to agents.
The recommendations below are candidates for subsequent work orders.
They do not change the scope or completion status of the existing three orders.

## Evidence boundary

The incident bundle proves reader-pool saturation and a failed required-store
probe. It does not identify the operations occupying the four readers.
This audit identifies avoidable work in the current checkout. It does not claim
that each finding caused that production incident.

Home resolves to the Kanban route in `apps/web/src/spa-routes.tsx`.
Boot hydration can skip route bootstrap and initial snapshots. Cold routes,
recovery, and task switches remain relevant paths. Costs below describe those
conditions, rather than every visit.

## Priorities

| Priority | Journey | Finding | Next change to evaluate |
| --- | --- | --- | --- |
| High | Home, Kanban, sidebar | Completion-gate checks use one writer transaction per task | Batch current gate summaries on the read path |
| High | Home, Kanban | Active and all-workflow hooks duplicate snapshot requests | Share requests and bound workflow fan-out |
| High | Task details, navigation | Optional enrichment repeats workspace reads and full board reads | Reuse scoped resources and read only workflow context |
| High | Task switches | Deadline and generation guards discard results without stopping requests | Propagate cancellation through exclusive request owners |
| High | Repository pickers and task setup | All failures trigger up to five repository reads | Classify errors and share retry ownership |
| Medium | Opening conversations | Full turn history is read independently of the message window | Share hydration first; design bounded turn reads separately |
| Medium | Retrying agent messages | Recovery searches recent messages across task sessions | Look up the durable admission by its stable ID |
| Medium | Task details | Usage invalidations can overlap two summary reads | Coalesce invalidations and cancel obsolete reads |
| Measure first | Sidebar | A small response still requires a pinned, multi-stage query | Profile stages and concurrent tabs before SQL changes |
| Lower | Boards and integration views | Workspace PR loading has only component-local deduplication | Share the scoped request; preserve task-specific reads |

## 1. Completion-gate reads scale with every displayed task

Sources:

- `apps/backend/internal/task/handlers/task_http_handlers.go:215`,
  `buildTaskDTOsWithSessionInfo`.
- `apps/backend/internal/task/handlers/sidebar_task_http.go:178` routes page
  enrichment through the same DTO builder.
- `apps/backend/internal/task/service/service_status_summary_rebuild.go:116`,
  `reconcileExistingSummaries`, and the missing-summary rebuild loop.
- `apps/backend/internal/task/repository/sqlite/completion_gates.go:273`,
  `GetTaskCompletionGate` and `readTaskCompletionGateTx`.

DTO assembly batches many inputs, but summary reconciliation calls
`completionGateSummary` separately for each task. `GetTaskCompletionGate` starts
a transaction on `r.db`, the writer handle. An empty gate still reads the task's
workspace, gate revision, and criteria. Thus N empty gates produce N writer
transactions and 3N SELECT statements. Verified criteria add evidence reads.
This is a source-derived count, not a measured latency estimate.

Evaluate a batch read for overview gate summaries, including current evidence
validity. Chunk IDs for each driver and retain a coherent read snapshot.
Do not replace current evidence checks with stale cached verdicts. Keep terminal
transition guards and evidence locking inside their existing mutation transaction.
Summary repair may still write when a value changes; separate this necessary
repair from repeated per-task reads. A blanket ban on repair writes would change
the current recovery contract.

Acceptance evidence: count queries and writer transactions for 1, 100, and 1,000
tasks with empty, unverified, valid, and stale gates. Check SQLite and PostgreSQL
parity, concurrent evidence changes, and completion blocking. The overview read
count should depend on batches and evidence categories, rather than task count.

The same builder loads all sessions with worktrees and then performs a separate
primary-session query with executor metadata. This is already batched, not a
per-card session query. Profile a smaller overview session projection before
removing either read; their selected fields and joins differ.

## 2. Cold boards duplicate the active workflow snapshot

`apps/web/components/kanban-board.tsx:320` mounts `useAllWorkflowSnapshots`.
Its `useKanbanData` path also mounts `useWorkflowSnapshot` for the active workflow.
Both call `fetchWorkflowSnapshot` directly. Their guards are local to each hook.

A temporary test mounted both hooks with an empty store and a held response.
It observed two concurrent requests for the same workflow. The test was removed
after execution. Boot-hydrated snapshots can avoid this duplication.

`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts:581` also starts all
workflow reads with `Promise.all`. A workspace with many workflows can therefore
create concurrent full snapshots. Cleanup advances a generation but does not
abort their network requests.

Share network acquisition by store, API/auth identity, workspace generation,
workflow, and request shape. Retain each consumer's existing live-event merge
rules. Use one trailing refresh when invalidations arrive during a read.
Prioritize the visible workflow and bound other workflow starts. Choose the
capacity from mixed-workload measurements; independent limits for Inbox,
statistics, and snapshots do not reserve capacity when combined.

`httpGetWorkflowSnapshot` lists and enriches all non-archived tasks.
`task_limit` is applied after `Service.ListTasks` loads all tasks, repositories,
and folders. It currently limits part of response enrichment, not the initial
database work. Do not silently truncate boards. Any later SQL limit or paging
must preserve `TaskCoverage`, search, workflow placement, and completeness.

Acceptance evidence: the cold active workflow has one shared request; repeated
foreground events do not overlap it; switching workspace cancels unused reads;
live task moves and status revisions survive late snapshots. Test desktop and
phone on a workspace with many workflows.

## 3. Task enrichment reads more context than a task page needs

`apps/web/lib/state/task-navigation-reads.ts:93` already shares task identity
between route and page consumers. Preserve that resource and its identity fences.

`fetchTaskNavigationEnrichment` in `apps/web/lib/ssr/session-page-state.ts:449`
then starts optional reads. The session path includes workflow snapshot, agents,
repositories with scripts, workspaces, workflows, all turns, user settings,
terminals, messages, and the active session snapshot. Agents already use a shared
resource when a store is supplied. Other workspace collections are requested
again during task-to-task navigation.

The shell renders before enrichment, so this does not all block first paint.
It still creates database work. `useTaskWorkflowSnapshot` in
`apps/web/components/task/task-page-content.tsx:57` can request the same workflow
while optional enrichment is in progress.

Reuse bounded workspace resources with explicit freshness and invalidation.
Keep repositories-with-scripts separate from repositories-only request shapes.
Reuse same-workspace metadata across task switches, and invalidate on mutation,
reconnect, auth changes, or workspace generation changes as required.

Task step context should use workflow metadata and ordered steps, rather than
load every task in the workflow. The existing `useWorkflowStepsById` consumer
illustrates the narrow need, but current enrichment also seeds board state.
Review that dependency before removing snapshot hydration. Board membership must
remain authoritative when the user returns Home.

Acceptance evidence: A to B navigation in one workspace does not repeat valid
workspace metadata reads or fetch an unrelated full board. Direct links, cold
loads, reconnect, workflow changes, new scripts, and session changes still work.

## 4. A bounded UI wait does not cancel database work

`beginOptionalHydration` in `apps/web/lib/ssr/session-page-state.ts:50` races
requests against a shared timer. When it expires, wrappers return unavailable;
the underlying fetches continue. Route cleanup and snapshot hooks likewise
discard superseded results without passing an AbortSignal.

Add cancellation at the request owner. Exclusive optional reads can stop on
deadline or task switch. A consumer leaving a shared request must release its
interest; it must not abort a request still needed by another consumer.
Keep generation fences because cancellation cannot undo a completed response.

Use request-count and held-response tests to distinguish ignoring a response
from actually cancelling a request. Verify cancellation reaches Go request
contexts and SQL waits. Never treat a client timeout as cancellation of an
already-accepted message or agent action.

## 5. Repository retry ownership needs correction

`apps/web/hooks/domains/workspace/use-repositories.ts:31` retries every exception
with delays of 100, 250, 500, and 1,000ms, then makes a final attempt.
Its request counter tracks loading state but does not share the request promise.
Cleanup discards results without stopping the retry loop.

A temporary fake-timer test returned a permanent 401 on every request.
It observed five calls. The test was removed after execution.

Share the scoped read and its retry cycle across mounted pickers and refreshes.
Stop automatic retries for authorization and permanent errors. For transient
errors, honor Retry-After and cancel on scope or lifetime changes. Preserve the
current behavior that failed loads do not fabricate an authoritative empty list.

The global `useEnsureWorkspaceWorkflows` and route bootstrap also call
`listWorkflows` independently. They need the same scoped acquisition where their
request shape matches. Keep boot reuse and existing workspace recovery budgets.

## 6. Turn history grows independently of the message window

The transcript uses a latest 100-message window and already deduplicates matching
requests by subscription readiness in `use-session-messages.ts:208`.
Repeated debug lines do not necessarily mean repeated network requests.

`use-session-turns-hydration.ts` reads the full persisted turn history and owns
three initial attempts plus delayed recovery. Optional route enrichment calls
`listSessionTurns` separately. Those two active paths do not share acquisition.
Its delayed timer checks session existence, rather than whether a transcript
still needs the read. Error handling retries all failures without classification.

First share hydration, failure classification, cooldown, and consumer lifetime
between route enrichment and transcript recovery. Preserve subscription readiness
and the reconciliation epoch for active-turn clears. The standalone
`use-session-turns.ts` hook has its own retry loop, but no production importer was
found in this checkout. Do not count it as an active source of this incident.
Plugin conversation turns have a separate host contract and require their own
scope review before sharing core caches.

Later, consider an additive turn-window API tied to visible message turn IDs,
plus the active marker. Current contracts expect full history. Paging needs a
separate design for older messages, turn navigation, plugin consumers, and
authoritative marker reconciliation. Adding LIMIT alone would break those needs.

## 7. Uncertain message admission should use its stable identity

Normal sending derives the input mode from current store state and returns the
created message directly. It does not first load every session's messages.
The expensive path is transport uncertainty or retry of a pending admission.

`apps/web/hooks/message-request.ts:36`, `findMessageByID`, lists task sessions,
then sequentially reads up to 100 messages from each session until it finds the
stable message ID. A task with S sessions can need one session-list request and
S message-list requests for a single lookup. An accepted message outside these
windows is missed. Existing idempotent resend protects against duplicate
acceptance; do not remove that protection.

Evaluate an authorized, task-scoped durable admission lookup by stable ID,
using the existing backend message/prompt-index primitives. It must resolve
cross-session routing, queue receipts, accepted-but-not-yet-dispatched messages,
and payload conflicts. Preserve per-user concealment for missing or foreign IDs.
Do not use a cache miss as evidence that a message was never accepted.

After successful delivery, plan-comment and preview-feedback reads run only
when those references were submitted. They currently run serially. Parallel or
event-based refresh can reduce composer wait, but has less impact than admission
lookup and must retain accepted-delivery success if enrichment fails.

Acceptance evidence: a lost response followed by session routing or a busy
transcript resolves the same admission with bounded reads and exactly one
dispatch. Include queued and conflicting retries.

## 8. Usage invalidations can overlap summary requests

`apps/web/hooks/domains/session/use-conversation-usage.ts:46` requests totals and
the latest usage turn together on mount, reconnect, and each invalidation.
Sequence guards reject old results but do not stop old requests or coalesce an
event burst. The latest-turn API is already limited to one row, and detail is
loaded on demand. Preserve those useful bounds.

Use one in-flight summary read and one trailing invalidation for the active
session. Pass cancellation on task/session changes. Profile before introducing
a combined totals/latest-turn endpoint; frontend coordination may be sufficient.

## 9. Sidebar response bounds do not bound query cost

The sidebar already uses a shared, bounded page cache and request ownership.
Do not replace it with full workspace snapshots or per-row REST reads.

`sidebar_task_query_snapshot.go` pins one reader through staging, indexing,
paging, hydration, and cleanup. SQLite materializes filtered candidates into a
temporary relation; PostgreSQL uses a repeatable-read snapshot. A small returned
page can still require workspace-wide candidate and grouping work.
The HTTP path propagates cancellation but has no explicit total operation
deadline or shared server admission in this path.

Measure time and rows at the existing stage checkpoints: created, indexed,
page, headers, hydrated, and commit. Include empty searches, collapsed groups,
deep hierarchies, large workspaces, and simultaneous tabs. Preserve the coherent
snapshot and existing scratch cleanup tests. Add deadline or admission only
with measured capacity and sanitized transient responses.

## 10. Workspace integration associations can share reads

`useWorkspacePRs` in `apps/web/hooks/domains/github/use-task-pr.ts` uses local
refs, so another mounted consumer or later route mount can start another read.
The MR equivalent shares simultaneous requests in a module map, but its key is
only workspace ID. Evaluate store/auth/generation-scoped resources for both.
Keep task PR sync and tooltip hydration resources, which already share reads.
Task-specific CI, review, and permission checks cannot be replaced by a stale
workspace association map.

## Validation and next execution order

Focused Vitest run: five files, 66 tests passed. This includes two temporary
audit reproductions and existing workflow merge, turn hydration, and message
admission recovery tests. Temporary tests were removed. No production code or
permanent tests changed. This was not an end-to-end or SQL performance benchmark.

Suggested order after the existing availability repair:

1. Count overview SQL operations and batch completion-gate reads.
2. Share workflow snapshot acquisition and measure bounded workflow starts.
3. Reduce task enrichment and propagate owner-scoped cancellation.
4. Unify repository and turn retry ownership; coalesce usage invalidations.
5. Design stable admission lookup, then consider bounded turn-history reads.

Use the existing task-navigation efficiency E2E helpers for held responses and
causal request counts. Cover cold Home, warm Home, multi-workflow Kanban,
A to B to A task switches, preview-to-details, direct URLs, reconnect, prolonged
503, and message sends during background refresh. Run desktop and phone flows.
Use disposable SQLite and PostgreSQL fixtures for query plans and SQL counts.
Measure mixed workloads against the required-store probe, rather than infer
pool availability from one endpoint's response time.
