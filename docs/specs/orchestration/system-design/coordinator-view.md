---
status: draft
system: orchestration
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-001
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-002
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-003
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-004
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-005
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-006
---

# Coordinator view system design

## Context and boundary

Design baseline: Kandev v0.94.0 plus local coordinator commit `b1cd0d2e`.
The task view is new. Existing roles, assignments, conversations, runtime,
automations and ownership remain their current contracts.

Orchestration owns this vertical view; canonical task state stays in
`internal/task`, including its `statussummary` projector. The current
`OrchestratedTasks` query in `internal/orchestration/repository/sqlite` returns
only linked ID/title/state rows, capped at 100. Do not extend it into a duplicate
task projection or use it as the source for the all-workspace view.

No schema or plugin SDK change is needed for this package. The related plugin's
host-managed conversation APIs and `WorkspaceAgentChat` export are absent at
this baseline; its implementation cannot be substituted for the existing runtime.

## Requirement mapping

| Requirement                            | Design sections                    |
| -------------------------------------- | ---------------------------------- |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-001 | Task data and paging; Presentation |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-002 | Group projection                   |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-003 | Conversation and navigation        |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-004 | Freshness and recovery             |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-005 | Presentation                       |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-006 | Authority and privacy              |

## Task data and paging

Use `GET /api/v1/workspaces/:id/tasks` through
`apps/web/lib/api/domains/kanban-api.ts:listTasksByWorkspace`. The handler already
has bounded page/page_size (maximum 100), search, workflow/repository filters and
batched session/status enrichment. Add the already-supported `exclude_config`
parameter to the typed client. Keep `include_ephemeral` and `include_archived`
false. Exclude native conversations and Office-owned workflow tasks from this
Kanban overview; verify exclusion server-side before counts/data leave the task
boundary when not already guaranteed by canonical task-list filtering.

Read task `status_summary`, workflow/step, identifier, profile, repositories and
coordinator-link metadata. Reuse native task mapping and status presentation
helpers. PR/diff/last-activity values come from existing `TaskStatusSummary` fields;
never scrape chat or issue prompts. Do not request per-task transcripts, tool
payloads or full session timelines merely to draw rows.

The page-scoped hook loads 100 rows initially and exposes Load more. Dedupe by
task ID, cancel outstanding loads on workspace/filter change, and retain
server-supported deterministic sorting. Search/workflow/repository filters run
at the server. Selected-coordinator/group filters apply to loaded eligible rows,
with explicit partial coverage until all matching pages have been read. A small
result set can complete in the initial read; larger sets must never silently stop
at the old 100-task coordinator-list limit.

The header distinguishes loaded eligible tasks from the underlying task-list
total and says “counts for loaded tasks” until complete. If canonical exclusions
reduce a page, do not mislabel the unfiltered total as a Kanban/group total.
Counts of open PRs or changed files are likewise sums for loaded rows with known
values, with unknown coverage shown. The initial package does not promise an
atomic workspace-wide statistics snapshot. It must not present complete-sounding
totals or a definitive empty group while unseen pages may contain matches.

## Group projection

Implement a pure, tested projection over task DTOs plus the freshest canonical
status summaries. Rows retain all relevant badges; this table only selects their
one primary group, evaluated in order:

| Order | Canonical signal                                                    | Group       |
| ----- | ------------------------------------------------------------------- | ----------- |
| 1     | Task-wide pending clarification or permission                       | Needs input |
| 2     | Active error, failed launch/interruption, FAILED or BLOCKED task    | Problems    |
| 3     | Live generating/background activity or active execution session     | Running     |
| 4     | REVIEW task, required review, or PR attention without higher signal | Review      |
| 5     | COMPLETED task with no higher signal                                | Done        |
| 6     | CREATED/TODO/SCHEDULING, queued step admission or pending prompts   | Queued      |
| 7     | Remaining states, including CANCELLED or insufficient evidence      | Other       |

Prefer the task-wide pending action projection over a primary-session shortcut:
another session may be waiting. Reuse native error acknowledgement/dismissal
semantics. WAITING_FOR_INPUT without a current canonical request is Other with
its raw status, not a fabricated question. An idle IN_PROGRESS task remains
Other unless a native activity signal explains it; parked background work keeps
its existing explanatory badge. Missing summaries permit documented coarse
fallbacks but never an inferred healthy/stalled state.

Display quiet time only from semantic `last_activity_at`, not projection
`updated_at` or wall-clock guesswork. Do not create a new inactivity threshold.
PR merged state remains a PR badge; native task completion determines Done.
This projection is presentation state, not a persisted assistant attention record.

## Conversation and navigation

Add `/workspaces/:workspaceId/coordinator` to SPA routing and workspace navigation,
gated by the existing Orchestration feature. A coordinator selection can be
represented by `orchestratorId` in the route query; validate it against the
workspace assignments. Use a previously valid selection for that workspace, or
the sole assignment; otherwise show an explicit selector. Do not choose another
account as fallback when an assignment becomes unavailable.

Keep existing `/orchestration` conversation/configuration URLs working. Provide
links into the central view with the same workspace/assignment identity. Global
role settings and workspace assignment settings remain the configuration owners.

Refactor `app/settings/orchestration/conversation-route.tsx` and
`conversation-pane.tsx` into reusable conversation content plus their existing
route shell as needed. Reuse `TaskChat`, identity, comment/recovery transports,
active-session context and streaming reconciliation. Avoid embedding a routed
page with duplicate headers or scroll owners.

Opening an assignment may ensure its deterministic existing conversation mapping;
it must not queue a turn. Hide/show chat and mobile tab changes keep the composer
mounted or preserve drafts in memory keyed by workspace and assignment. Switching
assignment clears the visible old content before loading the next. Do not persist
draft text in a new localStorage key or expose it to task filters/report generation.

The initial page offers observation, chat and links to existing task controls.
It has no new Sweep now/Run build action. Existing automation Run now remains
available in Automations, with its established dispatch semantics.

## Freshness and recovery

Integrate with the canonical task cache and WebSocket status/lifecycle handlers;
do not assume the active board cache contains every workflow on this page.
Either register the scoped overview projection in the shared update path or
invalidate/refetch its loaded pages on relevant workspace events. Include
task create/update/delete, status summary, pending input, PR and connection events.
Coalesce read invalidations; do not poll an agent or launch a model for status.

Use `pickFreshestStatusSummary`/`isNewerStatusSummary` for HTTP/WS races, preserving
equal-revision queue-count refresh behavior. Keep an explicit workspace generation
with abort signals and stale-response rejection; the plugin's workspace guard is
a useful pattern, not a reason to copy its separate state store.

On reconnect, refresh the loaded window and reconcile changed/deleted tasks.
On a failed read, retain only same-workspace rows marked stale and offer Retry;
do not turn the failure into zero counts. Scope/access changes clear rows/chat.
Refresh is a read and never retries a coordinator message or automation dispatch.

## Presentation

Desktop has a workspace header/assignment selector, task summary/filter bar and
two panes. Task groups occupy the main pane; chat is a resizable or fixed-width
side pane using existing layout primitives. Group headings expose count/coverage,
and rows link to native task pages and known PRs. Keyboard focus survives refresh.

At narrow widths, Tasks/Chat tabs replace the split view. Use one scroll owner per
visible pane, accessible selected-tab labels, readable task cards, safe-area
padding and existing touch target conventions. No essential action depends on
hover. Empty state differentiates no assignments, no tasks, no filter matches and
unavailable data. Hidden chat must not send or lose a draft.

Illustrative layout (design only; not a feature screenshot):

```text
Workspace / Coordinator                 [assignment] [settings]
[counts for loaded tasks] [coverage / stale indicator]
[search] [workflow] [repository] [all / coordinated]
+--------------------------------------+----------------------+
| Needs input / Problems / Running ... | Selected coordinator |
| Task · Step · Activity · PR · Diff    | Persistent chat      |
| [load more]                          | [message composer]   |
+--------------------------------------+----------------------+
Mobile: [Tasks] [Chat], same selection and state
```

## Authority and privacy

Keep workspace authorization and retained conversation ownership checks in their
existing backend services. Hidden conversation records are excluded at the task
query boundary, not merely masked with CSS. Add regression evidence for private
conversations, cross-workspace access, pagination and feature-off paths. Do not
implement a new unscoped task query to obtain summary totals.

Task observation does not register `orchestration_chief_id`, export context to a
different profile or grant cross-workspace access. Pending-input links use the
native task interface; automatic answers/permissions are deferred to the separate
assistant design. No new API promises read-only agent tool enforcement.

## Persistence, observability and validation

No new durable tables or migration. Local view preferences may use existing
preference patterns for non-content values only. Preserve conversation IDs and
stored role/context data through the UI change.

Use existing task/API diagnostics for failures; UI diagnostics may include
workspace/task IDs and summary revisions, never chat bodies, prompts or secrets.
No new transcript telemetry. Unit tests cover classification/paging/races; native
task authorization tests cover server exclusions; browser tests cover tasks plus
conversation on desktop/mobile, navigation, reconnect and feature gates.

Capture screenshots and a short silent video with a disposable fictional
workspace, synthetic task statuses and a scripted demo provider. Label media as
demonstration, inspect every frame and publish only after the final feature scope
is implemented. Never capture production history or real user prompts.

## Related decisions and delivery

- [Orchestration ownership](../../../decisions/2026-09-07-workspace-orchestration.md).
- [Retained private conversation ownership](../../../decisions/2026-09-16-private-conversation-ownership.md).
- [Plan and work orders](../../../plans/workspace-coordinator-view/plan.md).

Core/plugin packaging and the upstream PR target remain maintainer decisions.
They do not block this local design at the requested exact v0.94.0 baseline.
