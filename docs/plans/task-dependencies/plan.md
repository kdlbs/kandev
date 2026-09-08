---
spec: docs/specs/tasks/system-design/task-dependencies.md
created: 2026-08-09
status: not_started
---

# Implementation Plan: Task Dependencies and Auto-Start Chains

## Overview

Peer-to-peer task dependencies become a core Kanban capability with three
observable behaviors: a blocked task is visible, a blocked task is never
auto-started, and an unblocked task with a recorded launch intent starts exactly
once.

The work is deliberately additive over four shipped mechanisms rather than a new
subsystem:

- the `task_blockers` edge table and its BFS cycle detector;
- the `tasks.metadata.deferred_launch` intent with its atomic claim;
- the single auto-start chokepoint `orchestrator.autoStartTaskForStep`;
- WIP admission and queue promotion.

Nothing in this plan adds a second launch path, a denormalized `is_blocked`
column, or a chain entity.

## Confirmed current behavior

Verified in `apps/backend` and `apps/web` at `d38d9a0b8`:

- `task_blockers (task_id, blocker_task_id, created_at)` exists with a
  composite PK and a `CHECK (task_id != blocker_task_id)`. It is created by
  `internal/office/repository/sqlite/base.go:createTaskExtensionTables` but
  lives in the same database as `tasks`.
- `internal/office/dashboard/service_tasks.go` owns edge creation with
  self-edge, cross-workspace, and BFS cycle validation, returning
  `*BlockerCycleError{Path}` which the frontend already renders as `A → B → A`.
- `internal/task/service/service_office.go` exposes `AddBlocker` /
  `RemoveBlocker` with a separate, weaker cycle check, and errors with
  `blocker repository not configured` when `s.blockers == nil`.
- `internal/backendapp/main.go` calls `SetBlockerRepository(repos.Office)`
  **after** the `if !cfg.Features.Office { return }` early return, so in a
  Kanban-only install the blocker repository is not wired at all:
  `create_task` with `blocked_by` fails and `list_related_tasks_kandev`
  returns empty `blockers` / `blocked_by`.
- Edge HTTP routes exist only under the Office group:
  `POST /api/v1/office/tasks/:id/blockers`,
  `DELETE /api/v1/office/tasks/:id/blockers/:blockerId`
  (`internal/office/dashboard/handler.go`).
- `internal/office/scheduler/reactivity.go:cascadeBlockersResolved` is the only
  reaction to a resolved blocker. It runs from the Office `ApplyTaskMutation`
  path when a status becomes done, resolves "done" as
  `IsTaskInTerminalStep`, and requires a non-empty
  `AssigneeAgentProfileID`. It never fires for a Kanban board move or a
  workflow-engine transition. `normalisedStatus` maps `FAILED` to blocked, not
  done, so Office already does not cascade on failure.
- `internal/orchestrator/event_handlers_workflow.go:autoStartTaskForStep` is
  the single automated-launch chokepoint. It already returns early when
  `task.QueuedForStepID != ""`, consumes a `deferred_launch` intent via
  `launchDeferredTask`, and guards watcher races with
  `MetaKeyAutoStartGuard` / `claimAutoStart` / `restoreAutoStartClaim`. It is
  reached from `task.moved` (`handleTaskMovedNoSession`) and
  `task.queue_promoted` (`handleTaskQueuePromoted`).
- `wfmodels.IsTerminalStepName` recognizes `done|complete|completed|approved`;
  entering such a final step persists `state = COMPLETED`.
- The workflow engine declares `TriggerOnBlockerResolved` ("on_blocker_resolved")
  but `Engine.HandleTrigger` does not wire it; Office routes it through
  `SetWorkflowEngineDispatcher`.
- `create_task_kandev`'s handler struct reads `blocked_by`
  (`internal/mcp/handlers/handlers.go:576`, `json:"blocked_by"`) and forwards it
  to `CreateTask`, but the tool's `mcp.NewTool` schema in
  `internal/mcp/server/server.go:854` never declares the parameter. It is
  therefore undiscoverable to agents and not reliably passable through a
  schema-validating client. There is no MCP tool at all for adding or removing a
  dependency on an existing task.
- Frontend: `components/task/simple/components/blockers-picker.tsx` exists with
  cycle-message handling and optimistic mutation, but is mounted only from
  `task-properties.tsx` → `OfficeSimplePane`, which the Kanban route
  (`app/tasks/[id]/kanban-task-shell.tsx`) renders as a stub. Kanban has no
  dependency UI.
- The status row above the composer is `data-testid="chat-status-bar"` in
  `components/task/chat/chat-input-area.tsx`. It hosts `TodoIndicator`,
  `PRStatusChip`, `AzureDevOpsTaskPullRequestChip`, a queue chip, PR
  merged/closed banners, and right-aligned transcript controls. Its visibility is
  gated by `shouldRenderChatStatusBar({hasTask, hasTodos, hasQueueChip,
  showRightControls, showProceed})` — a new chip that is not added to that
  predicate renders nothing when it is the row's only content.
- A **second** status row exists: `data-testid="passthrough-status-row"` in
  `components/task/passthrough-toolbar.tsx`, which mounts its own `PRStatusChip`.
  Anything that belongs "next to the PR chip" belongs in both rows.
- `PRStatusChip` returns null when not applicable and branches on
  `usesMobileDrawer` to `PRStatusChipDrawer` versus `PRStatusChipHoverCard` —
  the mobile pattern to mirror rather than reinvent.
- `components/kanban-card-content.tsx` already renders a `queuedForStepId`
  badge — the precedent and the layout neighbour for a blocked badge.
- `lib/kanban/view-registry.ts` is a pluggable board-view registry with
  `kanban` and `graph2` ("Pipeline"). `graph2` visualizes one task across
  workflow steps, not cross-task edges. `getEffectiveView` forces the Kanban
  view on mobile.

## Backend design

### One owner for the dependency relationship

Move the authoritative edge operations into the task domain so a Kanban-only
install has them:

- Keep the physical `task_blockers` table where it is. It is in the same
  database, and relocating live DDL buys nothing. Add the missing indexes via
  idempotent migrations in `internal/task/repository/sqlite/base_migrations.go`
  (`(blocker_task_id)` for the reverse walk).
- Move the strong validator — self-edge, cross-workspace, BFS cycle with the
  existing walk limit and `*BlockerCycleError` — into the task service so both
  the new task-scoped routes and the existing Office routes call one
  implementation. Delete the weaker `checkCircularBlocker` in
  `service_office.go` rather than leaving two validators.
- Wire `SetBlockerRepository` **before** the `Features.Office` early return in
  `internal/backendapp/main.go`. This is the smallest change that makes
  `blocked_by` and `list_related_tasks_kandev` work in Kanban mode.
- Edge cleanup on task **delete** is explicit repository work, not a SQLite
  `ON DELETE CASCADE`, because the table predates the foreign key and must
  behave identically on PostgreSQL. **Archive** deliberately keeps the edges:
  an archived predecessor reads as `pending`, so dropping them would silently
  unblock the chain.

### Derived blocked state

Add one service helper that, for a set of task IDs, returns each task's direct
predecessors **and direct dependents**, plus a resolution verdict per
predecessor:

- resolved — `state = COMPLETED`, or resident in a final step whose name passes
  `IsTerminalStepName`;
- failed — `state` is `FAILED` or `CANCELLED`;
- pending — anything else, including archived.

`blocked` and `blocked_reason` are computed from that verdict on every read and
are never stored. Batch it: the Kanban board reads a whole workflow at once, so
a per-task query would add N round trips to the board load. Follow the existing
batch-helper convention in `task/service/service_access.go` — callers derive the
IDs from an already-authorized task list.

Both directions ship in the payload because the dependency chip shows both, and
the reverse direction is one indexed query rather than a client-side scan the
board's partial task list could not answer correctly anyway.

### MCP surface

Declaring `blocked_by` on the `create_task_kandev` schema is the smallest fix
with the largest effect: the plumbing already exists and only the contract is
missing. Two new tools, `add_task_dependency_kandev` and
`remove_task_dependency_kandev`, mirror the HTTP routes one-to-one and follow the
repo's existing preference for explicit small tools
(`add_branch_to_task_kandev`, the three `*_task_plan_kandev` tools) over a
mode-flag mutator. `list_related_tasks_kandev` is already the read side.

The load-bearing semantic decision: `blocked_by` non-empty plus
`start_agent: true` records a start-when-unblocked intent instead of launching.
`start_agent` defaults to true and its description tells agents to leave it true,
so the alternative silently launches the whole chain at once — exactly the
collision the feature exists to prevent. The create response reports
`started: false` with `start_when_unblocked: true` so the caller is not guessing.

### The gate

Add the dependency check to `autoStartTaskForStep` next to the existing
`QueuedForStepID` check, before `launchDeferredTask`. Because that function is
the only automated-launch entry point, one check covers `task.moved`,
`task.queue_promoted`, watcher auto-start, and dependency resolution itself.

The gate fails **closed**: if the predecessor lookup errors, skip the launch and
log a warning. Failing open would launch work whose predecessor may not have
run, which is the one outcome this feature exists to prevent.

Manual start (`StartTask` from a user action) does not consult the gate.

### Resolution and auto-start

Subscribe to the task-state and task-move events that can make a predecessor
resolved. On each, look up dependents with `ListTasksBlockedBy`, and for each
dependent re-evaluate all of its predecessors:

- any still pending → nothing;
- all resolved → publish `task.dependencies_resolved` and call
  `autoStartTaskForStep` with the dependent's current step;
- at least one failed and none pending → publish `task.dependency_failed`
  (Task 04).

`start_when_unblocked` on task creation persists the resolved launch inputs into
`metadata.deferred_launch` — the same record WIP overflow already writes and
`launchDeferredTask` already consumes idempotently. No new table, and the
"exactly one launch" property is inherited rather than re-derived. Removing the
last edge unblocks the task but must not consume the intent.

Startup reconciliation: after the queue reconciler runs, sweep non-archived
tasks that carry a `deferred_launch` intent and have edges, and launch those
whose dependencies are all resolved. This is the recovery path for a restart
between a predecessor's completion and the dependent's launch. Bound and batch
it so a large graph does not delay readiness.

### Not touched

`cascadeBlockersResolved`, Office assignment, `TriggerOnBlockerResolved` wiring,
and the Office task pane keep their current behavior. For a task eligible under
both reactions, the existing atomic deferred-launch/auto-start claim admits one
launcher.

## Frontend

- Extend the Kanban task type, boot mapper, and WS handlers with `blocked`,
  `blockedReason`, `dependsOn`, `blocks`, and `startWhenUnblocked`.
- Treat an omitted projection as **unknown**, never as "no edges", at every layer
  (mapper, WS merge, hydration backfill). The chip re-reads the task when its
  cached copy has no projection, so no WS/hydration ordering can blank it.
- Blocked badge on the shared card next to the existing queued badge. Both must
  fit without truncating either, on desktop and mobile, with no hover-only
  disclosure.
- Dependency chip in the status row above the composer, next to `PRStatusChip`,
  in **both** `chat-input-area.tsx` and `passthrough-toolbar.tsx`. It shows both
  directions (blocked by / blocks), opens to a navigable list with each entry's
  state, distinguishes failed predecessors, returns null with no edges, and uses
  the drawer-on-mobile pattern `PRStatusChip` already establishes.
  `shouldRenderChatStatusBar` must learn about the chip, or a task whose only
  status content is the chip renders no row.
- No "Depends on" picker in the Kanban task detail view. Dependencies are
  declared in the create dialog or over MCP (see the spec's What); the detail
  view's dependency surfaces — card badge, chip, list — stay read-only.
- "Depends on" field plus a "Start automatically when unblocked" checkbox in the
  task-create dialog, and an "Add dependency" entry in the Kanban card context
  menu (`kanban-card-link-submenu.tsx` is the precedent).
- No whole-graph board view. One was built and removed before shipping (see the
  spec's Out of scope): a `VIEW_REGISTRY` entry alone is not reachable anyway —
  the header toggle speaks the separate `TaskListingView` vocabulary — and
  without drawn connectors the result read as columns of text.
- All new copy goes through `t()` / `<Trans>`, and any newly touched path is
  appended to `i18nGuardFiles` in the same change.

## Test strategy

### Backend

- Edge creation: cycle at length 2, 3, and N; self-edge; cross-workspace;
  duplicate edge is idempotent; validator is shared by the task-scoped and
  Office routes.
- Derived state: pending / resolved-by-state / resolved-by-terminal-step /
  failed / cancelled / archived predecessors produce the right `blocked` and
  `blocked_reason`; batch helper matches the per-task result.
- Gate: a blocked task is not launched by `task.moved`, by
  `task.queue_promoted`, or by dependency resolution; a predecessor-lookup
  error skips the launch; manual start is unaffected.
- Resolution: last-predecessor-resolves launches exactly once; a non-last
  resolution launches nothing; no-intent unblocks without launching;
  last-edge-removed unblocks without launching; a queued dependent launches on
  WIP promotion, not on resolution.
- Chain: A → B → C with intents launches each exactly once, in order, and never
  restarts A.
- Races: dependency resolution and WIP promotion both firing produce one
  session; a claim loser logs and stops; a failed launch restores the claim.
- Restart: a restart between completion and launch yields exactly one session
  after startup reconciliation.
- Failure: `FAILED` / `CANCELLED` predecessor holds the dependent blocked with
  `reason: failed`, publishes the event once per transition, and raises one
  notification; retry-then-success unblocks and fires the intent once.
- Deletion: deleting a predecessor removes edges both ways, unblocks the
  dependent, and does not fire its intent.
- Kanban-only wiring: with `Features.Office` false, `blocked_by` on create
  succeeds and `list_related_tasks_kandev` populates `blockers` /
  `blocked_by`.
- MCP: a pinning test asserts `create_task_kandev`'s registered schema declares
  `blocked_by` and `start_when_unblocked` — the current bug is precisely a
  handler/schema mismatch that no behavior test would have caught.
  `blocked_by` + default `start_agent` starts nothing and reports
  `start_when_unblocked: true`; three chained create calls run in order; the two
  dependency tools cover default `task_id`, cycle rejection, absent-edge removal,
  and cross-workspace denial.
- PostgreSQL: add the environment-gated behavior test for every changed
  dialect-sensitive repository method, per `apps/backend/CLAUDE.md`.

### Frontend unit

- DTO/store conversion preserves the five new fields across boot and WS.
- Blocked badge renders count and reason; coexists with the queued badge.
- `shouldRenderChatStatusBar` returns true when the dependency chip is the row's
  only content.
- The chip lists both directions with states, distinguishes a failed
  predecessor, renders null with no edges, and appears in both status rows.
- Picker creates and removes edges optimistically and replaces the optimistic
  state with the formatted cycle path on rejection.
- Create dialog submits `blocked_by` and `start_when_unblocked`.
- Dependencies view builds nodes/edges from a graph payload, marks blocked and
  failed nodes, and is absent from the mobile view set.

### Browser E2E

- Desktop chain: seed A → B → C with intents, complete A, assert B starts and C
  does not; complete B, assert C starts.
- Desktop gate: a blocked task moved into an auto-start step gains no session.
- Desktop failure: fail A, assert B stays blocked with the failed reason and
  gains no session.
- Desktop view: create and delete an edge from the Dependencies view.
- Mobile: blocked badge readable in a focused column, no horizontal overflow,
  usable tap targets, Dependencies view not offered.

Seed tasks without an agent and locate by task id — `createTaskWithAgent` on a
start step moves the card mid-test.

## Public documentation

Update `docs/public/tasks-and-workflows.md` (and `workflow-tips.md` if the
auto-start interaction needs a worked example) to cover: declaring a dependency,
what counts as resolution, that a failed predecessor halts the chain and needs
human action, that dependencies gate automated starts but not manual ones, and
that dependency resolution does not bypass WIP limits.

## Implementation waves

Tasks 01–04 are strictly sequential: each consumes a contract the previous one
introduces. Tasks 05 and 06 need only Task 01's API surface, so either may start
once 01 lands. Task 08 closes out. Task 07 was built on 06 and then dropped
before shipping, so it is not part of the executable set and Task 08 does not
wait on it.

- [x] [Task 01: Promote dependency edges to a core relationship](task-01-core-dependency-relationship.md)
- [x] [Task 02: Gate automated launches on unresolved dependencies](task-02-gate-automated-launch.md)
- [x] [Task 03: Resolve dependencies and auto-start on unblock](task-03-resolve-and-auto-start.md)
- [x] [Task 04: Halt chains on a failed predecessor](task-04-failed-predecessor-halt.md)
- [x] [Task 05: MCP dependency tools](task-05-mcp-dependency-tools.md)
- [x] [Task 06: Kanban dependency UI](task-06-kanban-dependency-ui.md)
- [x] [Task 08: Verify browser flows and documentation](task-08-verify-flows-and-docs.md)

No task is marked `parallel-safe`; waves do not authorize subagent execution.

## Risks

- **Two validators.** `office/dashboard` has the strong BFS cycle check;
  `task/service` has a weaker one. Adding a third for the new routes would let
  a cycle in through whichever path is weakest. One validator, or the DAG
  invariant is not real.
- **Failing open on lookup error.** A blocked-state read that errors and returns
  "not blocked" launches exactly the work this feature prevents. The gate must
  fail closed and the test must prove it by breaking the store.
- **A denormalized `is_blocked` column.** Tempting for board queries, but a
  stale value gates launches wrongly. Batch the derived read instead.
- **Two launch paths.** Dependency resolution and WIP promotion can both make
  the same task launchable within milliseconds. Only the shared atomic claim
  keeps that to one session; a new bespoke launch call site would reintroduce
  the double-agent bug that `wip-limit-pull-system` documents.
- **Divergent "terminal" definitions.** `on_children_completed` treats `FAILED`
  as terminal; dependency resolution must not. Sharing a helper between them
  would silently make chains proceed on failure.
- **Office double-reaction.** With Office enabled, `cascadeBlockersResolved` and
  the new Kanban reaction can both fire for one task. Verify one session, and do
  not "fix" it by disabling either reaction.
- **Board load cost.** A per-card dependency query multiplies board queries by
  card count. The batch helper is load-bearing, not an optimization.
- **Graph view scope creep.** The Dependencies view is the largest single piece
  of UI. It must not become a general graph editor; edges and nodes only.
- **`start_agent: true` launching a blocked task.** Agents pass it by default.
  If create does not convert it to a start-when-unblocked intent, every chain an
  agent builds launches at once — the exact failure the feature prevents, and it
  would look like the gate is broken rather than the create semantics.
- **A schema/handler mismatch that no behavior test catches.** `blocked_by`
  already works end to end in the handler and is still unusable because the tool
  schema omits it. The pinning test on the registered schema is what stops the
  same class of gap recurring for `start_when_unblocked`.
- **Two status rows.** `chat-input-area.tsx` and `passthrough-toolbar.tsx` both
  mount `PRStatusChip`. Adding the dependency chip to one leaves it missing in
  the other layout, and the omission is invisible in the default view.
- **`shouldRenderChatStatusBar`.** A chip added without extending that predicate
  is dead in exactly the case it matters most: a blocked task with no PR, no
  todos, and no queued prompts.
