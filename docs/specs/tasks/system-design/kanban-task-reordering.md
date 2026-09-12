---
status: current
system: tasks
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
created: 2026-09-04
owners:
  - kandev
---
# Kanban Task Reordering System Design

## Purpose

Paired design for
[Kanban Task Reordering](../requirements/kanban-task-reordering.md). The
requirement states what a user observes; this file states the mechanism behind
it, the prior art the contract was drawn from, and the reasoning that would
otherwise have to be rediscovered at build time.

## Requirement mapping

| Requirement | Satisfied by |
| --- | --- |
| `REQ-TASKS-KANBAN-TASK-REORDERING-001` `.1`, `.36` | Ordering keys and the promotion comparators |
| `.2`, `.38` | Surfaces |
| `.12` | Surfaces, Position allocation |
| `.8`, `.16`, `.17`, `.18`, `.19`, `.20`, `.25` | Reorder contract |
| `.15`, `.28`, `.29`, `.30`, `.31` | Position allocation |
| `.26`, `.27`, `.37` | Concurrency |
| `.32`, `.33`, `.34` | Band model, Position allocation |

## Prior art

**Wiki (`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`, QMD collection
`wiki`).** Queried twice: `lex: kanban board ordering drag drop reorder backlog
priority queue position` / `vec: how tasks should be ordered in a kanban column
and how drag-and-drop reordering with a persisted position field works`, then
`lex: backlog prioritization WIP limit queue next in line pull` / `vec: how to
decide what work gets picked up next, backlog ordering, work-in-progress limits
and pull-based queues`. Direct page reads were unavailable (the vault path is
not readable from the task sandbox, so `grep` and `obsidian-wiki graph-query`
both failed); QMD's own index served the retrieval. **The wiki returned nothing
useful on board ordering or drag-and-drop reordering.** The nearest hits are
product-method pages, not interaction or persistence contracts:
`concepts/shape-up.md` (0.39) records the Basecamp position that backlogs,
sprints and points should be replaced by a betting table, and
`concepts/roadmap-from-failure-signal.md` (0.78) records Block's inversion where
customer signal generates the backlog rather than prioritising within it.
Neither takes a position this specification departs from: both argue about where
a backlog comes from, not how an existing queue is ordered. No prior position is
overridden.

**Cross-vendor (`saas-kb` / `search_fsm_docs`, `category: "ai_sdlc"`).**
Unavailable: the tool is not exposed in this session and no tool-discovery
mechanism is offered, so this leg did not run. Recorded rather than faked.

**In-repo prior art, which is the load-bearing one.** Kandev has already solved
this problem once, for the session message queue:
[Reorder Queued Messages](../../ui/requirements/message-queue-reorder.md). Its
request contract is followed deliberately: submit the whole ordered id set
rather than one task's new index, validate the submitted set against the current
persisted set and reject atomically when it differs, rewrite positions to a
dense sequence, update optimistically and reconcile to the server. Two
departures. The grab handle, per
AC-TASKS-KANBAN-TASK-REORDERING-001.6: message rows carry a dedicated handle
because their bodies own text selection and inline editing, whereas a kanban
card is already dragged by its whole body for cross-step moves, so a handle
would make the two drags on one card start differently. And the band
discriminator, per AC-TASKS-KANBAN-TASK-REORDERING-001.17: a message queue has
one list per session, so `{session_id, ordered_ids}` needs no band field, while
a step has two bands and the request must say which one it names.

That document also carries `## API Surface`, `## Data Model` and
`## Failure Modes` sections. Those are migrated legacy detail, not the current
house style for a requirement, which is why the paired requirement here does not
imitate them. The equivalents live below: the wire contract in
`## Reorder contract`, the storage rules in `## Position allocation`, and the
failure and race behavior in `## Concurrency`.

## Reorder contract

The requirement names an error token (`step_changed`,
AC-TASKS-KANBAN-TASK-REORDERING-001.19), requires the board to reconcile to
"the persisted order returned by the system"
(AC-TASKS-KANBAN-TASK-REORDERING-001.8), and requires publication to every
connected view (AC-TASKS-KANBAN-TASK-REORDERING-001.16). Those three are only
observable if one carrier is named, and
AC-TASKS-KANBAN-TASK-REORDERING-001.20 makes the split behavioral: the board
reconciles silently on `step_changed` and shows a localized message on anything
else. Backend and frontend must therefore agree on where the token appears.

**Route.** `PUT /api/v1/workflow-steps/:id/tasks/reorder`. This mirrors the
existing collection-reorder precedent `PUT
/api/v1/workspaces/:id/workflows/reorder`
(`task/handlers/route_registration_test.go`): a `PUT` on a collection nested
under the parent whose order is being rewritten. The step is the path
parameter, so the request body never repeats it.

**This route is the only request surface for a reorder.** A WebSocket request
action was specified in an earlier draft and then deliberately cut; the
requirement's `## Out of scope` names the task that tracks it. Nothing in the
board loses anything by that: it already moves a task between steps over HTTP,
so a reorder now travels the same way as the gesture beside it. The cut is
scoped to the *request*, and **Publication** below is unaffected —
`task.reordered` is still delivered over the WebSocket event stream, so
AC-TASKS-KANBAN-TASK-REORDERING-001.16's live reconciliation across connected
views works exactly as specified. A reader who finds only one surface here is
looking at a decision, not an omission.

**Request.** `{"band": "admitted" | "queued", "ordered_task_ids": [...]}`. The
band is an explicit discriminator rather than something inferred from the
submitted ids. Inference would make
AC-TASKS-KANBAN-TASK-REORDERING-001.18's both-bands rejection depend on the
very value it is validating, and would leave the empty-list case with no band
at all. Explicit means validation is a single pass: resolve the named band's
membership, then compare the submitted set against it.

**Success.** `200` with `{"workflow_step_id": ..., "revision": ...,
"tasks": [{"id": ..., "position": ...}]}` covering the step's non-hidden tasks
in their new order,
both bands. The whole step is returned because a reorder renumbers the whole
step (AC-TASKS-KANBAN-TASK-REORDERING-001.15), so returning only the named band
would leave the client unable to reconcile the sibling band it just saw change.

**Rejection.** The conflict in AC-TASKS-KANBAN-TASK-REORDERING-001.19 is `409`
with `{"code": "step_changed", "workflow_step_id": ..., "revision": ...,
"tasks": [{"id": ..., "position": ...}]}` — the same whole-step list the
success body carries. The conflict response carries the order deliberately:
AC-TASKS-KANBAN-TASK-REORDERING-001.19 requires the board to reconcile
*silently* to the authoritative order, and a bare error code leaves it nothing
to reconcile to, forcing the builder to invent either an unspecified refetch or
a wait on an event that may not have arrived yet. Validation failures under
AC-TASKS-KANBAN-TASK-REORDERING-001.18 are `400` with the code
`invalid_reorder` and no task list, because a malformed request implies nothing
about the persisted order; AC-TASKS-KANBAN-TASK-REORDERING-001.20's restore is
then from what the client already holds. The board branches on `code`, which is
what makes AC-TASKS-KANBAN-TASK-REORDERING-001.20's
silent-reconcile-versus-message split implementable.

**Publication.** A new event `task.reordered` in the `task.*` family
(`internal/events/types.go`), payload `{workflow_step_id, band, revision,
tasks: [{id, position}]}` with the same whole-step task list and revision as
the success body. Deliberately not `task.moved`
(AC-TASKS-KANBAN-TASK-REORDERING-001.21 forbids it) and deliberately not N
separate `task.updated` events, because a view applying a reorder one task at a
time would render intermediate orders that were never persisted.

**Which order wins (AC-TASKS-KANBAN-TASK-REORDERING-001.25).** Each step
carries a monotonic `order_revision`, incremented once per committed reorder of
that step inside the same serialized section that renumbers it. Every carrier
of an order — the `200` body, the `409` body and the `task.reordered` payload —
reports the revision its order was written at. A view applies an *unsolicited*
published order only when that revision is greater than the one it already holds
for that step, and applies the body of a response to *its own* request when that
revision is greater than **or equal to** the one it holds.

The equal case is not a detail, it is the whole reason the two rules differ. The
counter advances once per committed reorder, so a membership change that is not a
reorder — an arrival under
AC-TASKS-KANBAN-TASK-REORDERING-001.28, or any other cause listed in
AC-TASKS-KANBAN-TASK-REORDERING-001.26 — changes the step's order without
changing its revision. A `409` reports the state at rejection time, which is
exactly that case: **an equal revision does not mean an equal order.** Gating the
`409` on a strictly greater revision would therefore discard the very list
AC-TASKS-KANBAN-TASK-REORDERING-001.19 requires the board to reconcile to,
leaving it showing an optimistic order it never persisted and showing no error —
silently, which is the one outcome that criterion rules out. A response body is
safe on equality where an event is not, because it is causally after the
caller's own request and reports the step as the serialized section saw it.

Resolving a request therefore also releases the in-flight hold of
AC-TASKS-KANBAN-TASK-REORDERING-001.27: the board reconciles that band to the
highest-revision order it now holds for the step, counting both the response body
and any `task.reordered` it received and withheld while the request was
outstanding. Should those two carry the *same* revision and different orders —
possible, because the response is read after the event's reorder committed and
may include a membership change that bumped nothing — the response body wins, as
the later read. A view holding no revision for a step yet, which is every view
until its first carrier arrives, applies the first order it receives.

The alternative fix — bumping the revision on every membership change — was
rejected: it puts a step-row write on the path of every task creation and move,
where this rule costs nothing.

Without this, AC-TASKS-KANBAN-TASK-REORDERING-001.25's "the last committed
order shall be authoritative, and every connected view shall reconcile to it"
is not implementable from the client side: a `200` for reorder A can arrive
after the `task.reordered` for a later reorder B, and a view trusting arrival
order would render A and stay there. The counter is per step, so unrelated
steps never invalidate each other, and it lives on the step rather than in the
task rows, so it adds no per-task write and does not touch stored `position`
values (AC-TASKS-KANBAN-TASK-REORDERING-001.31). It is a new column on
`workflow_steps`, which today holds no such field. It starts at `0` for every
step, existing rows included, so a step that has never been reordered is not a
special case: the first reorder of any step commits revision `1`.

## Position allocation

`tasks.position` is an existing integer column with no unique constraint and no
index. This capability changes who writes it, not its shape, and requires no
migration.

**Renumbering on reorder (AC-TASKS-KANBAN-TASK-REORDERING-001.15).** A reorder
names one step and one band, but renumbers the whole step. Take the step's
non-hidden tasks, partition them into the admitted and queued bands, order the
named band by the submitted id sequence and the other band by its existing step
order, then assign `0` upward across the concatenation with the admitted band
first. The admitted band therefore receives the lower contiguous range and the
queued band the higher one. Two consequences worth stating because a reader will
otherwise infer the opposite: the two ranges are disjoint rather than
interleaved, and band membership is never recoverable from `position` alone: it
is decided by `wip_admitted` and `queued_for_step_id`.

**Arrival (AC-TASKS-KANBAN-TASK-REORDERING-001.28).** An arriving task takes
`max(position) + 1` over the step's non-hidden tasks, or `0` in an empty step.
This is deliberately not a renumbering: arrival must not reorder work the user
has already ordered. It follows that between reorders a step's values drift
non-contiguous, and that the next reorder of that step restores `0..N-1`. The
dense invariant is a post-reorder guarantee, not a stored invariant, and nothing
should assert it at rest.

Because every reorder compacts the step back to `0..N-1`, values are bounded by
step size across reorders rather than accumulating, so no overflow policy is
required beyond the column's integer range.

**The caller-supplied `position` argument
(AC-TASKS-KANBAN-TASK-REORDERING-001.28).** Every move path today takes
`position` as a literal from its caller and writes it verbatim
(`MoveTaskWithOptions`, `task/service/service_workflow.go`). AC-TASKS-KANBAN-
TASK-REORDERING-001.28 makes the server compute it instead, so that argument
stops deciding anything on an arrival. Spelling out which callers change,
because the list is not obvious and one of them is a published contract:

- `httpMoveTask`, `wsMoveTask` and the MCP `handleMoveTask` keep accepting the
  field for wire compatibility and ignore its value.
- The two frontend drag paths stop computing a target index client-side; the
  cross-step drag no longer sends `targetTasks.length`.
- Both bulk-move paths stop assigning indices themselves and take consecutive
  server-computed values per AC-TASKS-KANBAN-TASK-REORDERING-001.29. Deleting the
  client-assigned index is necessary but **not sufficient**, and this is the step
  most easily missed: each selected task is moved by its own call, and the server
  allocates `max + 1` per call, so the final order is simply the order the calls
  were issued in. The submission order must therefore be re-derived from
  AC-TASKS-KANBAN-TASK-REORDERING-001.29 — source step ordinal ascending, then
  within one source step that step's admitted band in step order followed by its
  queued band in step order. A selection can span workflows, so two source steps
  can share an ordinal; break that tie on source step `id` ascending, which keeps
  the submission order total and reproducible. It must **not** reuse the existing created-descending
  helper the multi-select path sorts with today
  (`lib/kanban/task-order.ts`, consumed by `hooks/use-task-multi-select.ts`),
  whose stated premise is "the board's visible created-desc order" — a premise
  AC-TASKS-KANBAN-TASK-REORDERING-001.2 retires. Created-descending differs from
  AC-TASKS-KANBAN-TASK-REORDERING-001.29 whenever a selection spans two source
  steps, and also within a single one, because step order leads on `position`.
- The plugin gRPC field `MoveTaskRequest.position`
  (`proto/kandev/plugin/v1/plugin.proto`) is the one published contract in the
  list. Its current comment documents the opposite of the new rule — that an
  omitted position and a position of zero both place the task at the top of the
  target step — so that comment becomes wrong the moment
  AC-TASKS-KANBAN-TASK-REORDERING-001.28 lands and must be corrected in the
  same change. The field itself stays on the wire and keeps its tag: removing
  it would break every plugin that sets it, whereas ignoring it changes only
  where the task lands.

The alternative — letting a caller keep injecting a task at the top — was
rejected rather than overlooked. It would let a plugin or an API client
silently displace work a user has ordered by hand, which is the single outcome
AC-TASKS-KANBAN-TASK-REORDERING-001.28 exists to prevent, and it would make the
top of a step mean different things depending on who wrote the task there.

**A move that names the task's current step is not an arrival.** It changes no
band membership and no step, so AC-TASKS-KANBAN-TASK-REORDERING-001.28 does not
apply to it and the task keeps the `position` it already holds. Position
changes within one step go through the reorder contract instead, which is what
keeps the band validation in
AC-TASKS-KANBAN-TASK-REORDERING-001.18 and AC-TASKS-KANBAN-TASK-REORDERING-001.19
from having a back door.

**Deriving the submitted order under a board filter
(AC-TASKS-KANBAN-TASK-REORDERING-001.34).** The request carries the band's
complete membership, but the user only dropped the card relative to the tasks
they could see. The submitted sequence is the band's persisted order with the
dragged task moved immediately before the visible task it was dropped above, or
last in the band when it was dropped below the last visible task. Every other
task keeps its persisted relative position, which is what makes the
"only the dragged task moves" guarantee hold for tasks the filter hid.

The keyboard path in AC-TASKS-KANBAN-TASK-REORDERING-001.12 uses the same rule,
with the visible task the card was moved past standing in for the one it was
dropped above: each arrow press moves the card one place among the members the
board is showing, and the submitted sequence places it immediately before the
visible member now below it, or last in the band when it has passed the last
visible member. Filtered-out members therefore keep their relative order under a
keyboard reorder exactly as under a drag, and one arrow press never moves the
card past two visible cards at once.

**Hidden tasks.** Archived, ephemeral and automation-run tasks keep whatever
`position` they hold and are skipped by both rules. Their values may collide
with live ones; the collision is unobservable because every step and workflow
listing already excludes them (`archived_at IS NULL`, `is_ephemeral = 0`, and
the automation-origin exclusion applied across the task repository's step and
workflow queries).

## Concurrency

**The step is the serialization unit, and it covers arrivals as well as
reorders.** Reorders against one step serialize
(AC-TASKS-KANBAN-TASK-REORDERING-001.37) because a reorder of either band
renumbers the whole step and two concurrent renumberings would interleave their
writes. Arrival position allocation
(AC-TASKS-KANBAN-TASK-REORDERING-001.28) takes the same step-scoped
serialization, so an arrival's `max(position) + 1` read and its write cannot
straddle a reorder's renumbering.

That second half is not a refinement, it is a correctness requirement, and the
failing case needs no exotic timing. Take a step of five tasks whose highest
`position` is `0` — the common shape today, because a task created in a step
takes `0` and only tasks moved or promoted in take a higher value. An arrival
reads `max = 0` and computes `1`; a concurrent reorder renumbers the five to
`0..4`. Unserialized, the arriving task lands at `1` and therefore sits
*second*, displacing work the user has just ordered and contradicting
AC-TASKS-KANBAN-TASK-REORDERING-001.28's own guarantee that an arrival sorts
last. Serializing the two makes `max` observed after the renumbering, so the
arrival takes `5`. Nothing in the argument depends on the whole step being
tied: any renumbering that raises `max` between an arrival's read and its write
produces the same displacement.

Serializing at the step, rather than the band, is also what lets two
different-band reorders both succeed rather than making the second a conflict. A
reorder changes no band's membership, so the `step_changed` check in
AC-TASKS-KANBAN-TASK-REORDERING-001.19, which compares only the named band's
membership, still passes for the second caller. Each reorder imposes order on
its own band and preserves the other band's current relative order, so applying
them one after another preserves both intents. A per-band lock would have been
the wrong unit: it admits concurrent whole-step renumbering and silently loses
one caller's ordering.

**There is no existing primitive to extend, and the nearest one is a no-op on
the default store.** `lockWorkflowStepForCapacity`
(`task/repository/sqlite/task.go`) is not merely scoped to WIP capacity: it
opens by returning immediately unless the driver is Postgres, so on SQLite —
the default store — it does nothing at all. Reading it as "a step lock that
only needs widening" is therefore wrong on the deployment that matters most.
Serializing reorders and arrival position allocation at the step is build work
that must supply its own primitive, and the choice is the builder's provided it
holds across both drivers: on SQLite the write transaction is already
single-writer, so an immediate-mode transaction spanning the read of `max` and
the write is sufficient; on Postgres the existing `SELECT ... FOR UPDATE` on
the step row extends naturally. What the design fixes is the *unit* and the
*span*, not the mechanism: the unit is the step, and the span must enclose both
the `max(position)` read and the write that depends on it.

**Two steps at once.** A move both leaves a source step and enters a
destination step, and AC-TASKS-KANBAN-TASK-REORDERING-001.26 makes it conflict
with a reorder of either. A move therefore holds both steps, and to avoid a
deadlock against a second move running the opposite direction, it acquires them
in a fixed global order — ascending step id — rather than source-then-destination.
A reorder touches one step and needs no ordering rule.

Failing to acquire is a transient failure, not a conflict: it is retried, and
if it still fails it surfaces as an ordinary failure under
AC-TASKS-KANBAN-TASK-REORDERING-001.20, never as `step_changed`. The two are
not interchangeable, because
AC-TASKS-KANBAN-TASK-REORDERING-001.19 makes `step_changed` reconcile silently,
and silently discarding a reorder the user made because a lock was busy would
look like the drag never happened.

**Publication during an in-flight reorder
(AC-TASKS-KANBAN-TASK-REORDERING-001.27).** Because a reorder renumbers the
whole step and a different-band reorder may commit while this band's request is
still in flight, the originating view can receive a `task.reordered` event
carrying its own band's *pre-request* order. It applies such an event to the
other band and to other steps, and holds its optimistic order for the band whose
request is outstanding until that request resolves, then reconciles to the
authoritative order. Without that rule the two requirements pull in opposite
directions: applying the event wholesale flickers the in-flight band back to the
order the user just changed, and ignoring it wholesale strands the sibling band.

## Ordering keys and the promotion comparators

AC-TASKS-KANBAN-TASK-REORDERING-001.1 names the keys the WIP overflow queue
already applies. Three implementations of those keys exist and they do not
agree on one of them: the SQL promotion query orders by
`COALESCE(queued_at, created_at)`, the two Go comparators
(`task/service/service_workflow.go` and `orchestrator/workflow_store.go`,
byte-identical to each other) skip the `queued_at` key entirely when either side
is null, and the frontend queue comparator (`lib/kanban/wip-queue.ts`) orders a
null `queued_at` last.

**This divergence is now observable, and
AC-TASKS-KANBAN-TASK-REORDERING-001.36 requires the three to be aligned on the
SQL query's rule.** An earlier draft of this design left them alone, on the
premise that within a single band either every task carries a `queued_at` or
none does, so the disagreeing case never arises. That premise is false. A task
created against a full destination step with a configured feeder lands in the
*feeder* step with `wip_admitted` true, `queued_for_step_id` set to the
destination, and `queued_at` set
(`task/repository/sqlite/task.go`, the feeder branch of the create-time WIP
placement). Under the band model below that task is in the feeder step's
**admitted** band, sitting beside ordinary admitted tasks whose `queued_at` is
null. The admitted band therefore mixes the two routinely, and it does so
exactly in the configuration this capability exists to serve.

The consequence is not theoretical. The feeder pull selects from the feeder step
with `queued_for_step_id IN ('', NULL, destination)`, which admits both classes
of task, and orders them by `position` first. On a `position` tie the board
(null last) and the promotion query (`COALESCE`) order those cards oppositely,
so the topmost card would not be the one the step yields next, breaking
AC-TASKS-KANBAN-TASK-REORDERING-001.2 and
AC-TASKS-KANBAN-TASK-REORDERING-001.4. Ties are not rare at ship time either. Nothing has ever
assigned a task's `position` for ordering's sake, so every task created in the
step it still sits in holds `0`, and a step that has only ever been filled by
creation is entirely tied. Tasks moved, bulk-moved or promoted in hold whatever
their move path wrote, so real steps mix a block of `0`s with scattered higher
values (AC-TASKS-KANBAN-TASK-REORDERING-001.35).

Alignment is on `COALESCE(queued_at, created_at)`, for two reasons. It is what
the SQL promotion query, the path that actually decides promotion today, already
does. And it is the principled reading: `queued_at` marks when a task became
eligible, and a task that was never queued became eligible when it was created,
so falling back to `created_at` measures the same thing rather than inventing a
sentinel.

This changes no stated contract. The shipped WIP requirement specifies only
"destination-first deterministic order"
([WIP Limits and Visible Overflow Queues](../requirements/wip-limit-pull-system.md),
AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2) and says nothing about `queued_at` null
handling, so no acceptance criterion is being weakened. What is being removed is
an undocumented three-way disagreement between implementations of one
under-specified rule, which is the opposite of the determinism that requirement
asks for. The reordering of the keys themselves, the destination-first
preference for a same-step queued candidate over a feeder candidate, and every
other part of the promotion contract are untouched.

## Band model

The board partitions a step by exclusion: the queued band is
`workflow_step_id == step && queued_for_step_id == step && !wip_admitted`, and
the admitted band is everything else in the step. The requirement's Terminology
matches that, deliberately, so the two bands are exhaustive.

The case this makes explicit is a task sitting in step A while queued for step
B, which holds no WIP slot in A yet renders among A's admitted cards. It is a
full member of A's admitted band: reorderable, counted in membership, and
ordered by `position` like any other. That is not merely cosmetic, because B's
promotion query selects from A ordered by `position`, so reordering these cards
in A chooses which one B pulls next. This is the WIP-queue case the capability
exists to serve, and it works without any additional rule. It is also the case
that makes the ordering-key alignment above mandatory rather than cosmetic.

## Surfaces

**The board ships exactly two views, and they are named in the registry.**
`lib/kanban/view-registry.ts` enables `kanban` (`SwimlaneKanbanContent`) and
`graph2`, labelled **Pipeline** (`SwimlaneGraph2Content`). Those two, and no
others, are what AC-TASKS-KANBAN-TASK-REORDERING-001.2 has to cover. Mobile is
not a third view: `getEffectiveView` forces the default view on a mobile
viewport, so the mobile presentation is the Kanban view rendered through
`components/kanban/swipeable-columns.tsx`.

This is worth stating flatly because there is a decoy.
`components/kanban/swimlane-graph-content.tsx` looks like a third view, exports
a component shaped like one, and has its own test file — but it is absent from
the registry and no production module imports it. It is unreachable, and a
change made only there ships nothing. Earlier drafts of this design called it
"the graph presentation"; that was wrong, and Pipeline is the surface those
criteria mean.

**Kanban view.** `components/kanban-column.tsx` is the only component that
partitions a step into bands and renders the queued divider, and it backs both
the desktop/tablet path (`swimlane-kanban-content.tsx`) and the mobile path
(`swipeable-columns.tsx`). It is the surface that accepts a reorder.

**Pipeline view.** It renders one row per task rather than a column per step:
`SwimlaneGraph2Content` sorts every task in the workflow by step index and then
by `position` alone, with no tiebreak, and hands each to `Graph2TaskPipeline`.
Two consequences for AC-TASKS-KANBAN-TASK-REORDERING-001.2. Its ordering must
gain the rest of AC-TASKS-KANBAN-TASK-REORDERING-001.1's keys, because bare
`position` is not a total order and ties are the ship-time norm; and "step
order" applies to each step's contiguous run of rows rather than to a column.
It gains no band split and no divider — it has neither today, no acceptance
criterion asks for one, and rows for one step land admitted-before-queued
anyway immediately after a reorder, because
AC-TASKS-KANBAN-TASK-REORDERING-001.15 gives the admitted band the lower
contiguous range. Pipeline accepts no reorder input at all
(AC-TASKS-KANBAN-TASK-REORDERING-001.38); its existing per-task step-move
control is unchanged.

The board's column list is virtualized, with absolutely positioned rows and a
small overscan window. A sortable context that only covers rendered rows cannot
express a drop beyond that window, and the keyboard path in
AC-TASKS-KANBAN-TASK-REORDERING-001.12 has the same constraint: an arrow press
must be able to move a card past rows that are not mounted. This is the main
implementation constraint on the interaction ACs and is called out here so it is
costed rather than discovered. The mechanism is Build's choice: scroll-to-index
synchronization with the virtualizer and suspending virtualization for the
duration of a drag both satisfy the stated outcomes.

The in-repo keyboard-sortable convention (a keyboard sensor with sortable
coordinate resolution) already exists in the quick-chat tab strip and the task
row settings list, and matches the key bindings the requirement names.

## Related

- [WIP Limits and Visible Overflow Queues](../requirements/wip-limit-pull-system.md)
- [Reorder Queued Messages](../../ui/requirements/message-queue-reorder.md)
