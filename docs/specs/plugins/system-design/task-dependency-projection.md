---
id: plugins-task-dependency-projection-design
title: Task dependency projection on the plugin data API
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-TASK-DEPS-001
  - REQ-PLUGINS-TASK-DEPS-002
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Task dependency projection on the plugin data API System Design

## Purpose and boundaries

This design owns how the plugin task read model carries task dependency state:
the wire contract on the gRPC plugin surface and the canvas JSON protocol, the
batched derivation step, the ordering, where the projection is attached, and the
refresh rule.

It does not own what an edge *end* may say. Scope redaction
(`REQ-PLUGINS-TASK-DEPS-003`) and ends the response does not contain
(`REQ-PLUGINS-TASK-DEPS-006`) are owned by
[dependency edge ends](task-dependency-edge-ends.md), which this design's field
set and derivation feed.

It does not own what a response may cost or how large it may be. The query
bound, the edge-end fan-out, the per-task list cut, and the response-size bound
(`REQ-PLUGINS-TASK-DEPS-004`) are owned by
[response bounds](task-dependency-response-bounds.md). This design fixes the set
of objects a response serializes; that one counts derivations and bytes over it.

It does not own the refresh contract. When a consumer re-reads, what the events
carry, and the no-lock and no-snapshot rules (`REQ-PLUGINS-TASK-DEPS-005`) are
owned by [refresh contract](task-dependency-refresh.md), which this design's
field set and derivation feed.

It does not own dependency semantics. The derivation of `blocked`,
`blocked_reason`, and the two edge lists, the resolution vocabulary, the
fail-closed rule, and the deferred-launch intent are owned by
[task dependencies](../../tasks/system-design/task-dependencies.md). This design
consumes that derivation and adds no second definition of it.

Adjacent contracts used but not owned:

- [Isolated web-app contributions](isolated-web-app-contributions.md) — the
  canvas browser data protocol, its scope binding, response limits, and event
  stream.
- [Capability approval](capability-approval.md) — the effective-authority
  intersection behind `api_read:tasks`.
- [Task completion](../../tasks/system-design/task-completion.md) — what makes a
  predecessor resolved.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-TASK-DEPS-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |
| `REQ-PLUGINS-TASK-DEPS-002` | [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

The projection crosses four existing layers. Each keeps its current
responsibility; none gains a dependency query of its own.

| Component | Responsibility |
| --- | --- |
| Task service dependency derivation | Sole producer of the derived view for a batch of tasks. Already batched for the Kanban board. |
| Plugin host task reader | Requests one batched derivation per response, after pagination, and stamps it onto the SDK task DTOs. Mirrors how linked pull requests are attached. |
| Plugin SDK task type and proto mapping | Carries the projection as additive, hand-mapped fields with a stable wire form in both directions. |
| Canvas JSON adapter | Projects the SDK task into the browser JSON shape and applies scope redaction, because it is the only layer that holds the canvas binding. |

There is no single funnel today, and the design does not pretend otherwise. It
states the attachment point **once**, as a property of the response rather than
of a call site:

> A response carries the projection on every plugin task read model object it
> serializes, and on no other. A task read whose object never reaches a wire
> encoder does not derive it.

The rule is stated on the response because the call sites are not in one-to-one
correspondence with responses: one route can read three task objects and return
one, and one host method serves both kinds of caller.

The test a builder applies is mechanical, because "serializes" has exactly three
implementations. A task object derives the projection if and only if it reaches
`toProto` or `tasksToProto` on the gRPC side (`pkg/pluginsdk/host.go`), or
`webAppTaskFromSDK` on the canvas side
(`internal/plugins/webapp_protocol_json.go:120`). Everything else read on the way
is an intermediate.

Applied today, the rule resolves to nine routes: the six gRPC handlers on
`grpcHostServer` -- `ListTasks`, `GetTask`, `CreateTask`, `UpdateTask`,
`MoveTask` and `PreviewPluginOwnedTaskTree` -- and three canvas routes, the task
list, the single-task `GET`, and the `PATCH`. The canvas list route reaches the
encoder from **two** sources: its task-scope branch serializes a single task
obtained from `Tasks().Get` and never calls `Tasks().List` at all
(`internal/plugins/webapp_protocol_data.go:43`), which a checklist written from
host methods misses entirely.

That list is *derived from* the rule and does not replace it. If a later call
site makes the list stale, the rule governs and the list is what is wrong. This
enumeration has already been incomplete twice, so the contract may not depend on
it being complete a third time.

## Data and contracts

### Task read model additions

Five fields plus two bounding flags, on the gRPC `Task` message, the SDK `Task`
type, and the canvas task JSON, with one name per field across all three:

| Field | Type | Meaning |
| --- | --- | --- |
| `blocked` | bool | Derived blocked verdict for this task. |
| `blocked_reason` | string | `pending`, `failed`, `unknown`, or empty. Empty if and only if `blocked` is false. |
| `depends_on` | repeated ref | Direct predecessors. |
| `blocks` | repeated ref | Direct dependents. |
| `depends_on_truncated` | bool | The predecessor list was cut to the per-task limit. |
| `blocks_truncated` | bool | The dependent list was cut to the per-task limit. |
| `start_when_unblocked` | bool | The task's stored chain-launch intent, reported raw. |

An edge-end ref carries `id`, `title`, `state`, and `status`:

- `status` on a `depends_on` entry is exactly `resolved`, `failed`, or
  `pending`.
- `status` on a `blocks` entry is always empty. Resolution is a property of a
  predecessor, so a dependent has none. This diverges from the Kanban board wire
  shape, which copies a status onto `blocks` entries and can emit the derivation's
  internal `missing` marker there; emptying it closes the public vocabulary and
  removes that leak.

The ref type is a new named message on the proto surface so both lists share one
shape, and a mirrored SDK type with explicit `toProto` and `fromProto`
functions, per ADR 0043's hand-mapped-DTO rule.

### Presence and the absent-versus-empty rule

All seven fields are always populated on every returned task, including a task
with no edges. This is the one intentional departure from the Kanban and
WebSocket task payload, where an absent projection means "not computed" and an
empty list means "no edges".

The departure is safe only because the plugin surface always computes the
projection, so absence never occurs within one host version. It follows the
in-repo precedent for derived task-DTO booleans always serialized rather than
omitted when false -- `is_ephemeral`, `autopilot`, `wip_admitted` -- for the same
reason: a missing key is indistinguishable from false.

On the proto surface, `blocked`, the two flags, and `start_when_unblocked` are
plain `bool` and `blocked_reason` is a plain `string`; the always-computed rule
is what makes their zero values meaningful rather than ambiguous. In the canvas
JSON, none of the seven keys is omitted when empty, for the same reason.

The two lists need one more thing said about them, because "not omitted" is not
enough on that surface: an empty `depends_on` or `blocks` is encoded as `[]`,
never `null` (`AC-PLUGINS-TASK-DEPS-001.18`). A null would be exactly the third
state the presence rule exists to forbid, and it is the *default* encoding of an
unallocated list, so the adapter must allocate an empty one rather than pass
through whatever the derivation left. The neighbouring task fields on this
surface are declared `omitempty`, which makes the omission trap live too; both
are avoided by the same explicit construction. This binds the encoder, not the
derivation, and the withheld verdict is encoded the same way.

**What `start_when_unblocked` reads.** `models.HasStartWhenUnblockedIntent`, which
returns the `start_when_unblocked` key of the task's `deferred_launch` metadata
record. That key separates a dependency-chain intent from a WIP-overflow one,
which share the record, so the projection calls the accessor rather than testing
the record's presence. `RemoveDependency` does not clear the record, which is why
`AC-PLUGINS-TASK-DEPS-001.10` reports the stored value rather than re-judging it
against the current edges.

### Wire compatibility

Field additions only, per ADR 0043's additive-only rule. New proto field numbers
continue past the highest number currently assigned on the `Task` message; no
existing number, name, or type changes, and no field is removed or renumbered. A
plugin built against the previous contract ignores the new fields.

A plugin built against the new contract reading an older host is the case that
needs care, and it is not benign. proto3 gives these scalars no field presence,
so the older host's response decodes as `blocked: false`, empty lists, both
flags false -- a well-formed "not blocked, no edges" answer, and the exact
opposite of the withheld verdict. The client cannot tell it from a real one by
looking at the fields, so the presence guarantee holds within one host version
only (`AC-PLUGINS-TASK-DEPS-001.15`).

**What separates them is the install-time floor, not a runtime check**
(`AC-PLUGINS-TASK-DEPS-001.16`). There is no runtime signal to consult and none
is added: the gRPC handshake's protocol version is a fixed constant, the canvas
context's protocol version is likewise fixed and does not change here, and the
capability set is unchanged because the projection is a field set on the
already-approved `api_read:tasks` resource. Nothing on either surface reports the
host's feature set. The mechanism that does exist is the manifest's
`min_kandev_version`, compared against the running host during install and
rejecting an older one outright with no partial registration
(`CheckMinimumKandevVersion`,
`internal/plugins/manifest/semver.go:12-28`, called from
`service_install.go:259`). That check is capability-**agnostic**: it reads one
declared floor and compares it, and knows nothing about which capabilities the
manifest requests. A consumer that must not misread the ambiguous bytes declares
the first host release carrying this capability there.

The `api_read:messages` floor is **not** that mechanism and must not be copied as
though it were. It is capability-**keyed**, runs at `Manifest.Validate()` time
against a hardcoded constant, and never consults the running host
(`RequiresMessagesCapabilityMinimum` / `validateCapabilityMinimumVersions`,
`internal/plugins/manifest/min_version_policy.go:11-28`). Building that shape for
`api_read:tasks` is precisely what the next paragraph forbids.

That floor is declared by the author, not enforced per capability
(`AC-PLUGINS-TASK-DEPS-001.17`): a capability-keyed minimum on `api_read:tasks`
would reject every already-installed plugin that reads tasks and wants nothing to
do with dependencies. The residue is honest and documented -- a plugin that omits
the floor, or is sideloaded past the install path, receives the ambiguous bytes
with no signal. Adding a wrapper message to manufacture field presence was
rejected separately: it would change the field shape for every consumer to serve
a check the install gate already answers.

The canvas protocol version does not change: the browser data protocol already
documents its task object as a field list, and this is an addition to that list.
A canvas ships inside a plugin package, so the same manifest floor covers it.

### Edge order

`depends_on` is ordered by edge creation time ascending, tiebroken by
predecessor task id ascending. `blocks` is ordered by edge creation time
ascending, tiebroken by dependent task id ascending. The tiebreak column is the
far end's task id, which is unique within one list because an edge is keyed by
the pair, so both orders are total.

In storage those are `task_blockers.created_at` for both lists, tiebroken by
`task_blockers.blocker_task_id` for `depends_on` and by `task_blockers.task_id`
for `blocks`. The batched predecessor query selects neither column today, so
establishing the order means adding the `ORDER BY`, not sorting after the read.

The `blocks` direction already has exactly this order in its batched query. The
`depends_on` direction does not: the batched predecessor query has no `ORDER BY`
at all, so today's order is whatever the storage engine returns, even though the
Tasks system design states predecessors are listed in creation order. Satisfying
`AC-PLUGINS-TASK-DEPS-001.8` therefore requires that ordering
to be established in the shared derivation.

That is a change to a shared path and also affects the Kanban board's order.
It is a correction toward an already-frozen Tasks contract rather than a new
contract, so it belongs in the shared derivation and not in a plugin-only
re-sort: a plugin-only sort would leave the two surfaces disagreeing about an
order the Tasks design has already fixed. Implementation must treat the shared
derivation as the ordering authority and flag the board-visible effect.

This is one of **five** changes this design requires in the shared derivation.
They are enumerated here because Build plans its work orders off this list, and an
undercount silently under-scopes the Tasks-owned half of the capability:

1. **Edge order**, above: an `ORDER BY` on the batched predecessor query.
2. **Dangling-edge drop in the dependent direction**, in the sibling's
   [Edge ends outside the response](task-dependency-edge-ends.md#edge-ends-outside-the-response).
3. **Fail closed on every derivation-failure path**, in
   [Failure and recovery](#failure-and-recovery). Three of the four do not today.
4. **Carry the far end's workspace id on the edge-end ref**, in the sibling's
   [Redaction rule](task-dependency-edge-ends.md#redaction-rule), so scope
   admission stays query-free.
5. **Chunk the derivation's batched id queries** -- edge-end resolution, and the
   task-keyed predecessor and dependent queries on the unpaginated preview flow --
   in the sibling's
   [Edge-end fan-out](task-dependency-response-bounds.md#edge-end-fan-out), so
   neither family can exceed the database's bind-parameter ceiling.

All five are corrections toward, or additive reads on, the existing Tasks
contract; all five are board-visible; none needs a migration; and all five must be
planned as work on the Tasks-owned derivation rather than as plugin-local code.

## Control flow

For a list read:

1. The host reader resolves scope, fetches, filters, and sorts tasks as today.
2. The page is cut to the requested limit.
3. On the canvas list route, the scope filter runs here, and the steps below see
   only its survivors.
4. The remaining tasks are mapped to SDK DTOs, and one batched dependency
   derivation runs for exactly their task ids.
5. The derived view is stamped onto each DTO, with per-list bounding applied.
6. For a canvas request, the JSON adapter projects each DTO and redacts
   out-of-scope edge ends.

For a single-task read, and for the task object returned by a write, steps 4 to
6 run for that one task. For the plugin-owned task-tree preview they run once
over the whole returned tree: one derivation, not one per task, and the tree is
not paginated, so [response bounds](task-dependency-response-bounds.md#response-bounds-by-flow)
states what bounds that response and why the smaller-`limit` remedy is the one
remedy that does not apply to it. The derivation for a write response runs after
the write commits, so the response reflects the post-write state.

It is not, however, a snapshot taken atomically with the write. The derivation
takes no dependency mutation lock (see
[refresh contract](task-dependency-refresh.md#events-and-refresh)),
so the observation point is the derivation instant, and an edge mutated between
commit and derivation is permitted to appear in the response. This is the
`AC-PLUGINS-TASK-DEPS-001.13` contract: the write response matches what a read
would report at that instant, not what the write itself established. A consumer
needing a later state refetches on signal.

Ordering of steps 2 and 4 is load-bearing. Deriving before pagination would fan
the derivation out over every task the filter matched, which is the fault the
pull-request attachment step already avoids by attaching after pagination.

The canvas list route applies its own scope filter after the host has paginated,
and **the derivation runs after that filter**. That is what makes the attachment
rule's second sentence true here: a task `webAppTaskMatches` drops never reaches
`webAppTaskFromSDK`, so it derives nothing. Implementation collects the surviving
tasks first and derives over that slice, rather than deriving over the host's
page inside the existing filter-and-encode loop
(`internal/plugins/webapp_protocol_data.go:56-68`). Filtering first costs
`AC-PLUGINS-TASK-DEPS-004.1` nothing: the predicate is query-free in instance,
workspace, task and repository scope, and in session scope it already issues a
per-task session read this capability neither adds nor removes, which is not a
dependency read. Deriving over the host's page would also satisfy
`AC-PLUGINS-TASK-DEPS-004.2`, but it contradicts the rule and spends the
derivation on tasks the canvas is about to drop.

### Reads whose task object is discarded

`Tasks().Get` is the method that makes the response-level rule necessary rather
than merely tidy. It has six callers, and only two of them serialize the task
they read:

| Caller | Uses the task for | Serializes it |
| --- | --- | --- |
| `listWebAppTasks`, task-scope branch (`:43`) | the response itself | Yes |
| `getWebAppTask` (`:76`) | the response itself | Yes |
| `updateWebAppTask` (`:108`) | binding preflight, then writes and returns the written object | No |
| `sendWebAppMessage` (`:166`) | binding preflight; returns no task | No |
| `listWebAppWorkflows` (`:209`) | reads `WorkflowID` to filter workflows | No |
| `listWebAppWorkflowSteps` (`:249`) | reads `WorkflowID` to authorize | No |

All six are in `internal/plugins/webapp_protocol_data.go`. An earlier draft named
only two of the four that discard -- which is why the contract is the rule above
and not a list, since a seventh caller would make any replacement list wrong
again. Under the rule the four need no enumeration, because none reaches an
encoder.

**The consequence for placement.** The projection therefore may **not** be
attached unconditionally inside `Tasks().Get`: four of its six callers would
derive for a response that carries no projection, and the `PATCH` route would
derive twice. Two shapes satisfy the rule and either is acceptable -- attach in
the responders that serialize, or keep a non-attaching lookup for the preflights
and attach in `Tasks().Get` for the rest. What the design forbids is the third
shape: the helper inside `Tasks().Get`, with the preflight callers expected not
to mind.

**The `PATCH` route derives once.** Its body declares `title`, `description`,
`state` and `workflow_step_id` as independently optional with no mutual
exclusivity, so one valid body naming both a title and a step takes **both**
write branches -- `Tasks().Update` (`:124`) then `Tasks().Move` (`:135`) -- and
returns one object while discarding the other (`:147`). One response, one
derivation, on the object serialized.

## Failure and recovery

One verdict covers every path on which the projection is unavailable to the
caller. The **withheld verdict** is `blocked: true`, `blocked_reason: unknown`,
both lists empty, both truncation flags false, and `start_when_unblocked` false
(`AC-PLUGINS-TASK-DEPS-001.14`). It reuses the derivation's existing `unknown`
vocabulary rather than adding a value, so no consumer needs a new branch.

The upstream derivation already fails closed this way on a predecessor read
error. The plugin surface preserves that verdict verbatim, and extends it to the
paths the derivation never sees:

- A failed derivation does not fail the task read. The task is returned with the
  withheld verdict and every other field intact.
- A failed derivation is not silently degraded to empty. This is the deliberate
  difference from the pull-request attachment step, which returns an empty list
  on a failed lookup: an absent pull request is a plausible true answer, while an
  empty edge list is indistinguishable from a real answer.

Four paths produce it, enumerated so none is discovered later as a degraded
empty list: a failed read of the predecessor direction,
a failed read of the dependent direction, a failed resolution of edge-end
detail, and a derivation source absent or not wired to the plugin host at all.
There is no configuration in which tasks silently read as unblocked.

**Only the first of the four fails closed today**, which is why this is change 3
of the five listed under [Edge order](#edge-order). The dependent-direction read
error is swallowed into an empty map, and the derivation then returns a real,
non-`unknown` verdict computed from the predecessor tally alone; an edge-end batch
error marks every unresolved end `pending`, which reads as a live predecessor
rather than an unknown one; and an absent derivation source returns an empty map,
which reads as `blocked: false` on every task. Each is exactly the "degraded to
empty" this section forbids.

The withheld verdict is **whole-task and indivisible**: a failure on any of the
four paths yields every field of `AC-PLUGINS-TASK-DEPS-001.14` together. A
dependent-direction failure does not empty `blocks` while keeping a real
`blocked` -- that publishes a verdict derived from half a read, the
confidently-wrong answer the requirement exists to prevent.

A fifth path is authority rather than failure, and applies only where a response
is returned at all. A surface gated on `api_read:tasks` -- the read RPCs and the
canvas data routes -- keeps rejecting a caller without it outright; the withheld
verdict never becomes the body of a call that should have been refused
(`AC-PLUGINS-TASK-DEPS-001.11`). It applies where the read model is returned
behind a *different* gate: the gRPC task-write RPCs gate on the write capability,
granted independently of the read one, so a caller holding only `api_write:tasks`
would otherwise read the graph through the write response. The projection is
therefore stamped behind the read-capability check on every surface returning the
read model, not only the read RPCs. The shape is uniform on purpose: a distinct
"forbidden" projection would tell a caller there was something to withhold.

Failure granularity follows the derivation's batch boundaries, not the task. A
failure that affects the batched read yields the withheld verdict for every task
in that batch; a failure confined to one task's edge-end resolution yields it for
that task alone. Per-task isolation of a batch-wide failure is deliberately not
promised, because detecting it would need the per-task queries that
`AC-PLUGINS-TASK-DEPS-004.1` forbids.


## Persistence

None. No table, column, migration, or cache is added. The projection is derived
on every read, which is the invariant the Tasks design exists to protect: a
stored blocked flag would be read by the auto-start gate, and a stale value
there would launch work whose predecessor never ran.


## Observability

One structured log record per failed dependency derivation, carrying plugin,
instance, resource type, operation, and result code. No task title, task id, or
edge content is logged, matching the isolated-web-app diagnostics rule that keeps
data content out of logs and metrics.

The unit is the **derivation attempt, not the withheld task**: one batched
derivation that fails emits one record however many tasks it withholds for, and a
failure confined to one task's edge-end resolution likewise emits one. Records
therefore do not scale with page size, which is what stops a failing dependency
store turning a 200-task read into 200 log lines; since no field identifies a
task, per-task records would be indistinguishable anyway.

That log record is the whole of the observability contract
(`AC-PLUGINS-TASK-DEPS-002.4`). No counter and no metric namespace is added, and
the design does not claim a withheld verdict is visible on an existing
data-request counter -- it is not, because a failed derivation does not fail the
surrounding request, so the request is counted as a success. An operator who
needs the rate of withheld verdicts reads the log records. Promoting that to a
counter is a deliberate non-goal: it would be the capability's only metric, and
the isolated-web-app diagnostics surface is where one would belong.

## Test strategy

- Proto and SDK round trip for the ref type and all seven fields, including the
  empty-list and empty-status cases.
- Canvas JSON projection: every field present on a task with no edges, and no
  key omitted when empty.
- Parity: the same task yields the same projection values on the gRPC and canvas
  surfaces, redaction aside.
- Order: predecessors and dependents come back in the defined order with the
  named tiebreak, repeatably.
- Withheld verdict: each of the four failure paths -- predecessor read,
  dependent read, edge-end resolution, and an unwired derivation source -- yields
  `blocked: true`, reason `unknown`, empty lists, both flags false, without
  failing the read and without omitting a field.
- Failure granularity: a batch-wide failure withholds for every task in the
  batch; a failure confined to one task's edge-end resolution leaves the other
  tasks' real verdicts intact.
- Capability: a caller holding only the write capability receives the withheld
  verdict from a task-write RPC's task object, byte-identical to the derivation
  failure case, and never an edge end; a caller without `api_read:tasks` on a
  surface gated on it is rejected outright, with no task object returned.
- `start_when_unblocked` reports the stored intent: true for a task whose last
  edge was removed, false for a WIP-overflow-only deferred launch.
- Write response carries the post-write projection, asserted on each of the three
  write RPCs and on the preview RPC, not only on the read path.
- Empty lists encode as `[]` in the canvas JSON: asserted on the raw response
  body for a task with no edges and for the withheld verdict, so a `null` or an
  omitted key fails rather than being normalized away by a typed decoder.
- Derivation count per response, asserted with a counting fake rather than on
  the response body: the canvas `PATCH` route derives exactly once for a body
  naming both a title and a workflow step, which takes both the update and the
  move branch; and each of the four `Tasks().Get` callers that discards its task
  -- the `PATCH` preflight, the message route, the workflow list, and the
  workflow-step list -- derives not at all. The last two fail if the projection
  is attached unconditionally inside `Tasks().Get`.
- Every serializing route carries the projection, asserted per route rather than
  once, over the nine routes enumerated in
  [Components and responsibilities](#components-and-responsibilities) -- the
  canvas list route's task-scope branch included.
- The canvas list route derives only for the tasks it serializes: on a page where
  the scope filter drops tasks, the derived task-id set is exactly the survivors
  and no dropped task is derived for. Asserted on the id set handed to the
  derivation, because the response body cannot distinguish the two orders.

There is no new Kandev UI, so the web E2E suites are not a coverage source. The
consumer-facing proof is a canvas that renders a workspace dependency graph from
the data API alone.

## Documentation

Seven things across this capability are consumer contract rather than wire shape,
so they must be stated in the public documentation rather than inferred. One is
this design's: the `min_kandev_version` floor and the consequence of omitting it.
Two are the [refresh contract](task-dependency-refresh.md#documentation)'s, and
four are [response bounds](task-dependency-response-bounds.md#documentation)'.
All seven go in `docs/public/canvases.md`,
`docs/public/plugins-authoring.md`, and the canvas authoring browser-API
reference, whose task-object field list and event section both need updating.

## Related decisions

- [ADR 0043 — Plugin host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Isolated web-app contributions](isolated-web-app-contributions.md)
- [Task dependencies](../../tasks/system-design/task-dependencies.md)
- [Dependency edge ends](task-dependency-edge-ends.md)
- [Response bounds](task-dependency-response-bounds.md)
