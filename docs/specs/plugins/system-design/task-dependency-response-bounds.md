---
id: plugins-task-dependency-response-bounds-design
title: Response bounds for the task dependency projection
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-TASK-DEPS-004
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Response bounds for the task dependency projection System Design

## Purpose and boundaries

This design owns what one response carrying the dependency projection may cost
and how large it may be: the query bound, the edge-end fan-out, the per-task list
cut, and the response-size bound on each surface
(`REQ-PLUGINS-TASK-DEPS-004`). It is separated from the
[task dependency projection](task-dependency-projection.md) because the two are
reviewed against different risks, and because these are the rules that decide
whether the capability still behaves on a workspace larger than the one it was
demonstrated on.

It does not own the field set, the derivation, the ordering, the refresh rule, or
**where** the projection is attached. Those are the projection design's. In
particular, *which* responses carry the projection is settled by
[Components and responsibilities](task-dependency-projection.md#components-and-responsibilities);
this design counts derivations over whatever set that rule produces, and does not
define a second one.

Adjacent contracts used but not owned:

- [Task dependency projection](task-dependency-projection.md) — the field set,
  the derivation, and the attachment rule these bounds are counted against.
- [Dependency edge ends](task-dependency-edge-ends.md) — the redaction rule,
  whose admission test is deliberately written to issue no read so that it cannot
  spend the budget in `AC-PLUGINS-TASK-DEPS-004.1`.
- [Isolated web-app contributions](isolated-web-app-contributions.md) — the
  canvas runtime's response limit.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-TASK-DEPS-004` | [Bounding rules](#bounding-rules), [Edge-end fan-out](#edge-end-fan-out), [Response bounds by flow](#response-bounds-by-flow) |

## Bounding rules

Four independent bounds, none of which subsumes another. The first two bound
cost, the last two bound payload.

1. **Query count.** At most one batched derivation per response, counted on the
   task set the response serializes — a read whose task object is discarded runs
   none. The set is fixed by the projection design's attachment rule, not
   restated here. The derivation's own read count is fixed by its steps and does
   not grow with the number of tasks in the page — except that a task set larger
   than the host-parameter ceiling chunks its task-keyed edge queries too, which
   only the unpaginated preview flow can reach
   (`AC-PLUGINS-TASK-DEPS-004.11`).
2. **Edge-end fan-out.** The one part of the derivation whose cost is driven by
   data rather than by its own structure, because its input is the set of
   distinct edge ends across the whole response. It is narrowed before it runs:
   the dependent direction is ordered and cut to 512 *first*, since no verdict
   reads a dependent's far row, while the predecessor direction is resolved whole
   because the verdict does read those (`AC-PLUGINS-TASK-DEPS-004.13`). What is
   left is bounded twice — by a chunk size that keeps each statement legal, and,
   on paginated flows, by a maximum that keeps the read count affordable.
   [Edge-end fan-out](#edge-end-fan-out) below. This bounds **cost**, not bytes.
3. **Per-task list size.** Each list is cut at 512 entries, reusing the existing
   per-task dependency maximum rather than introducing a second number. The cut
   keeps the first entries in the defined order and sets the matching truncation
   flag. `blocked` and `blocked_reason` are computed before the cut, so a
   truncated list never changes the verdict. This is the bound that holds the
   **payload** side down on a single-task read, and it is the one
   `AC-PLUGINS-TASK-DEPS-004.9`'s single-task claim rests on — rule 2's maximum
   does not, and never did.

   Both directions need the bound, and both flags are reachable. The predecessor
   maximum is enforced on the full-set replacement path only; the incremental
   add path does not check it, so a task can accumulate more than 512
   predecessors. The dependent direction has no write-side bound at all, because
   any number of tasks may depend on one task.
4. **Response size.** The existing per-surface limits still apply, and this
   design adds none. [Response bounds by flow](#response-bounds-by-flow) below
   states which flows can reach them and what the remedy is on each.

## Edge-end fan-out

Rule 3 bounds one task's list at 512 and the page limit bounds a list read at
200 tasks. **Two different quantities grow out of that, and conflating them is
the specific mistake this section exists to prevent.**

- **Distinct edge ends** are what edge-end resolution reads: the deduplicated
  union of every end across the page, after the dependent-direction cut of
  `AC-PLUGINS-TASK-DEPS-004.13`. Deduplication makes this set far smaller than
  the product whenever tasks share edges -- a page of 200 tasks all waiting on
  the same five blockers resolves five ends, not a thousand.
- **Serialized entries** are what the response writes: as many as 200 x 2 x 512 =
  204,800, because an end shared by forty tasks is written forty times. This is
  the quantity the per-surface response limits act on
  (`AC-PLUGINS-TASK-DEPS-004.14`).

Neither bounds the other in either direction. A page can resolve a thousand ends
and serialize two hundred thousand entries, or resolve four thousand ends and
serialize four thousand. So each needs its own rule, and a bound written over one
must not be described as protecting the other. The paragraphs below bound the
first. The second is bounded only by the per-surface response limit already in
`AC-PLUGINS-TASK-DEPS-004.6` and `.7` -- a real bound, since the runtime refuses
an oversized response rather than truncating it, but a bound on the response
rather than one this capability places on the projection.

That matters because the resolution query is a batched `IN (...)` with one bind
parameter per id and no splitting (`GetTasksByIDs`,
`internal/task/repository/sqlite/task.go:3791`). Past the database's
bind-parameter ceiling — 999 on older SQLite builds, 32766 on newer ones — such a
statement does not return a short answer; it fails to execute.

Left unaddressed, that failure has a specific and bad shape. It surfaces as an
error from the edge-end resolution step, which is exactly
`AC-PLUGINS-TASK-DEPS-002.5`'s third failure path, so the derivation fails closed
and every task on the page returns the withheld verdict. The result is
well-formed, satisfies every criterion as written, and reports a uniformly
unknown graph for a workspace whose only fault is being large. A small
acceptance workspace passes while a dense one silently degrades. The two bounds
below exist to make that unreachable rather than to handle it.

**Chunking keeps the statements legal** (`AC-PLUGINS-TASK-DEPS-004.11`). Two
families of statement need it, for the same reason and with the same fix. The
edge-end resolution query is the one this section is named for. The task-keyed
edge queries — `ListBlockersForTasks` and `ListDependentsForTasks`, which bind
one parameter per task id in the response — carry the identical hazard, but only
on the preview flow: every paginated flow is capped at 200 task ids, comfortably
inside the ceiling, while preview walks an uncapped subtree. A builder who
chunks only the first leaves a large plugin-owned tree failing at the database
with the derivation's own error, which fails closed and reports a uniformly
unknown graph — the precise shape the next paragraph rejects.

The
repository already owns this convention in the same package as the offending
query: `sqliteMaxHostParams = 500` and a `chunkIDs` helper in
`internal/task/repository/sqlite/base_queries.go`, whose sibling
`buildInPlaceholders` documents that the caller is responsible for splitting
oversized input, and which `task_status_summary.go`, `message.go` and
`session.go` already use. So this is not a new mechanism; it is the existing one
applied to a query that skipped it. This is **change 5** of the changes the
projection design enumerates in the shared derivation, and like the other four it
is a correction toward existing convention, is board-visible, and needs no
migration.

Splitting the query does not split the step. A failure in any batch is the one
edge-end-resolution failure of `AC-PLUGINS-TASK-DEPS-002.5`, over the whole
derivation, and does not create a narrower failure boundary than the one
`AC-PLUGINS-TASK-DEPS-002.3` already describes. Order is unaffected for a
different reason: entry order comes from the edge queries, and this step only
resolves detail for ends those queries already returned, so which batch resolved
an end cannot be observed. What the split does change is that the derivation
observes over an interval rather than an instant -- already permitted, and
already stated, by the no-lock and no-snapshot rules.

Chunking alone is not enough, which is why it is not the only rule. It converts
one illegal statement into an unbounded number of legal ones, so an oversized
page would still cost reads in proportion to its edge volume — the fault
`AC-PLUGINS-TASK-DEPS-004.1` exists to prevent.

**A maximum keeps the response affordable** (`AC-PLUGINS-TASK-DEPS-004.10`). The
number of distinct ends one derivation resolves is capped, and a task set
exceeding the cap fails as an oversized response rather than as a derivation
failure. Two properties of the number are load-bearing:

- It binds **paginated flows only**. The single-task flows are exempt
  (`AC-PLUGINS-TASK-DEPS-004.10`), because the refusal's whole premise is that
  the caller can retry with a smaller `limit`, and those flows have no `limit` to
  reduce. Refusing there would make a task permanently unreadable for the single
  reason that too many tasks name it -- the outcome
  `REQ-PLUGINS-TASK-DEPS-004`'s own Intent forbids. An earlier draft instead
  claimed one task could never reach the maximum, on the grounds that its two
  lists hold at most 1024 entries. That reasoning was wrong, and the way it was
  wrong is worth keeping: the list cut is a **presentation** cut applied at
  stamping, while resolution runs upstream over every end the task has, so a task
  with ten thousand predecessors resolves ten thousand ends and serializes 512.
- It is **reachable on a list read**, because 200 tasks each naming hundreds of
  distinct ends exceed any affordable figure. The bound is real rather than
  decorative, and the smaller-`limit` remedy is the consumer's lever.

The number is **4096**, declared as a named constant beside the derivation rather
than left for implementation to choose, because an unpinned bound is one every
call site picks differently. Three checks place it, all three on the cost side,
because this number bounds reads and rows and does not bound bytes. At the
repository's chunk size of 500 it is at most nine resolution queries
(ceil(4096/500) = 9), the same order as the derivation's fixed steps, so rule 1
still holds at the maximum rather than only in the average case. A full page of
200 tasks reaches it at an average of 4096 / 200 = 20.48 distinct ends per task,
which is a graph a real workspace can have, so the bound engages before a dense
page turns into hundreds of queries -- a breakeven test should be anchored at 20
ends per task and just under the cap, or at 21 and just over it, rather than on
21 as the breakeven figure. And it leaves four times the headroom over the 1024
entries one task can serialize. That headroom is not decoration: a *paginated*
read at `limit=1` is still a paginated flow and still bound by the maximum, so
the floor is what makes a one-task page of an ordinary task agree with the exempt
single-task read of the same task. The two regimes part company only for a task
that passed the 512 predecessor limit on the incremental write path, where a
`limit=1` page has no smaller `limit` to retry with; the documented remedy there
is the single-task read (`AC-PLUGINS-TASK-DEPS-004.10`).

**What this number does not do is bound the response size**, and the design says
so rather than implying otherwise (`AC-PLUGINS-TASK-DEPS-004.14`). 4096 entries
of roughly 150 bytes would be about 600 KB, which is 58.6% of the canvas 1 MiB
limit -- but 4096 counts *distinct ends*, not serialized entries, and a page that
resolves 4096 distinct ends may serialize up to 204,800 entries, some 30 MB. The
two bounds therefore do **not** trip at comparable sizes, and the byte side is
held by the per-surface response limit alone. A dense page can pass this maximum
and still be refused as oversized; that is expected, and both refusals carry the
same smaller-`limit` remedy.

Routing this to the withheld verdict instead is explicitly rejected. The two
conditions are different: a derivation failure means the graph is *unknown*, and
an over-budget page means the graph is *known but undeliverable at this page
size*. Reporting the second as the first would tell a consumer its workspace has
no readable dependency state, when halving its page size would return the whole
graph. The fail-closed rule is for the first case only.

The failure mode also matches how this package already treats the same hazard.
`maxMessageFilterValues = 400` exists so batched message filters stay
"comfortably under SQLite's `SQLITE_MAX_VARIABLE_NUMBER`"
(`internal/plugins/host_data.go:66-76`), and the message reader rejects an
over-limit request outright rather than letting it reach the database as "a
cryptic 'too many SQL variables'" (`:605-612`). The difference here is only that
the caller did not choose the edge count, so the error is an oversized-response
error on the response rather than an invalid-argument error on the request.

## Response bounds by flow

`AC-PLUGINS-TASK-DEPS-004.6` and `.7` both name a smaller page `limit` as the
remedy. Most in-scope flows accept no `limit`, so the remedy has to be stated per
flow rather than once.

The canvas limit is not a list-route limit: the runtime applies its response
ceiling to **every** canvas response it writes
(`writeWebAppJSON`, `internal/plugins/webapp_protocol_json.go:34`), so a
single-task or update response is subject to it too. On the gRPC side the Host
server sets no maximum send size and the plugin is the client, so the effective
bound is the plugin's own client-side maximum receive size, and a response over
it fails the whole RPC with `ResourceExhausted`.

| Flow | Paginated | Can the projection reach a bound? | Remedy |
| --- | --- | --- | --- |
| gRPC `ListTasks` | Yes | Yes, via the page's edge-end total and size | Smaller `limit` |
| Canvas task list | Yes | Yes, same | Smaller `limit` |
| gRPC `GetTask` | No | Not through the projection — at most 1024 serialized entries; exempt from the fan-out maximum | None needed |
| Canvas single-task and update routes | No | Not through the projection — same | None needed |
| gRPC `CreateTask` / `UpdateTask` / `MoveTask` | No | Not through the projection — same | None needed |
| Plugin-owned task-tree preview | No | **Yes, unbounded** — and not exempt from the fan-out maximum | Reduce the tree |

The "none needed" rows are a claim about the projection, not a promise that such
a response always fits. A single task already carries a description and metadata
this capability neither adds nor bounds, so a single-task response can exceed a
limit on its own; what the table states is that the projection is unlikely to be
the reason, because two lists of at most 512 entries are roughly 150 KB at the
design's 150 B/entry estimate.

Two qualifications keep that row honest. On the **canvas** side 150 KB sits well
under the 1 MiB ceiling, so the claim holds outright. On the **gRPC** side the
ceiling belongs to the plugin's own client connection, not to the host, so the
claim is conditional: it holds for any plugin at or above the gRPC library
default receive maximum of 4 MiB, and a plugin that deliberately lowers its
`MaxCallRecvMsgSize` below roughly 150 KB plus the task object can trip
`ResourceExhausted` on a single-task read. `AC-PLUGINS-TASK-DEPS-004.9` states
that as a floor on the receiving end rather than as a guarantee, and the
documentation carries the byte figure so a plugin author can size the connection
against it.

These rows are also where the exemption lands. Because rule 2's maximum does not
bind them, a single-task read of a task with an extreme predecessor count
*succeeds*, resolving those ends in chunks rather than refusing. That is the
`REQ-PLUGINS-TASK-DEPS-004` Intent honoured literally: the task stays readable.
The dependent direction, which is the one a third party can inflate without the
task's consent, never reaches resolution beyond 512 at all
(`AC-PLUGINS-TASK-DEPS-004.13`), so the conceded cost is confined to a task's own
declared predecessors.

**The remedy's boundary semantics differ by surface, and both are inherited
rather than set here** (`AC-PLUGINS-TASK-DEPS-004.15`). A consumer told to
"retry with a smaller `limit`" meets two different contracts. The canvas surface
**rejects** a supplied `limit` outside 1 to 200 with `invalid_request`
(`webAppRequestPage`, `internal/plugins/webapp_protocol.go:277-288`), so a
consumer that halves its way down to 0 gets an error rather than a page, while
the host's own page normalization **clamps** a supplied out-of-range value into
the same range (`normalizePageLimit`, `internal/plugins/host_data.go:66-88`) --
so a gRPC caller asking for 500 silently receives 200 where a canvas caller
asking for 500 receives an error. The divergence is confined to a supplied
value: `webAppRequestPage` validates only a non-empty `limit` parameter, so an
absent one is an error on neither surface and reaches the host's default of 50
either way. This capability changes none of it; it documents it, because the
remedy is stated in two criteria and a builder reading only those would have to
guess which behaviour applies where.

Preview is the real exception and is stated as one rather than smoothed over. It
returns every plugin-owned task in a subtree, walked recursively with no cap
(`ownedTaskSubtree`, `internal/plugins/host_write.go:630`), and accepts no page
parameter, so both its response size and its edge-end total are bounded only by
how large that tree is. Adding pagination to it is out of scope: its request and
response shape belongs to the plugin-owned task-tree contract, and the same
unboundedness already exists there today, since `Preview` attaches linked pull
requests over exactly the same unpaginated set (`:541`). The projection inherits
that exposure rather than creating it. What this design requires is that the
documentation stop implying a `limit` the RPC does not have, and say plainly that
the only lever is the size of the plugin-owned tree.

## Test strategy

- Query count, per `AC-PLUGINS-TASK-DEPS-004.1`, in two parts that must not be
  collapsed into one. **Structural steps:** a one-task page and a many-task page
  *carrying the same number of distinct edge ends* cost the same reads, measured
  with a counting fake, including the degenerate pair where neither page carries
  any edges. **No per-task read:** read count is flat as the task count grows at
  constant edge volume, which is the N+1 regression this guards. Neither bullet
  asserts a fixed read count across pages of *differing* edge volume — chunking
  makes that false by construction, and an earlier draft of this section asserted
  it alongside the chunking test below, which no implementation could satisfy.
- Chunking: a derivation over more distinct edge ends than the chunk size
  completes and returns a correct graph, and issues more than one resolution
  query — the assertion is on correctness plus a bounded query count, so a
  regression that removes the chunking fails here rather than at a database
  error.
- The bind-parameter ceiling is never reached, asserted on **both** statement
  families: a derivation over an end count that would exceed the lower ceiling of
  999 in a single statement succeeds, and a preview over a task count that would
  do the same succeeds. The second fails if a builder chunks edge-end resolution
  and leaves the task-keyed edge queries unsplit.
- The fan-out maximum is not routed to the withheld verdict: a task set past the
  maximum returns the surface's oversized-response error, and no task in it
  reports `blocked_reason: unknown`. This is the test that fails if a builder
  wires the condition into `AC-PLUGINS-TASK-DEPS-002.5`'s third path.
- The paginated and exempt regimes agree where they should: a task holding the
  full 512 entries in both lists returns an identical projection from a
  `limit=1` list read and from each single-task flow, and a task past the
  predecessor limit returns it from the single-task flows while the `limit=1`
  list read refuses with the oversized-response error. This pins the one place
  the two regimes deliberately differ, so a builder cannot collapse them.
- The single-task exemption holds, per `AC-PLUGINS-TASK-DEPS-004.10`: a task
  whose *predecessor* count alone exceeds the fan-out maximum still returns a
  complete projection on each non-paginated single-task flow — no
  `response_too_large`, no `ResourceExhausted`, no withheld verdict — with both
  lists cut to 512, both truncation flags true, and a verdict computed from every
  predecessor including the omitted ones. This is the test that fails if a builder
  applies the maximum uniformly across flows, which would make the task
  unreadable.
- The dependent direction is cut before resolution, per
  `AC-PLUGINS-TASK-DEPS-004.13`: a task with far more than 512 dependents resolves
  at most 512 dependent ends, asserted on the id set passed to edge-end
  resolution, while a task with more than 512 predecessors resolves all of them.
  A regression that resolves first and cuts second passes every output assertion
  and fails only here.
- Per-task cut: a list past 512 is cut with the flag set, in the defined order,
  and the verdict is computed before the cut and left unchanged.
- Response size: a canvas response over the runtime limit returns the existing
  bounded error on a single-task route as well as a list route, which fails if
  the limit is implemented as list-only.
- No bound is met by shrinking the projection: none of the above paths returns a
  task whose projection was dropped, emptied, or redacted to fit.
- Chunking is not a failure boundary: a resolution failure confined to one batch
  withholds for the whole derivation, not for the subset of tasks whose ends fell
  in that batch, and the entry order of an unaffected run is identical whether the
  ends resolved in one batch or several.
- Distinct ends do not bound bytes, per `AC-PLUGINS-TASK-DEPS-004.14`: a page
  whose tasks share their predecessors — so that its distinct-end count stays well
  under the fan-out maximum — but which serializes enough repeated entries to pass
  the canvas response limit returns `response_too_large`. This fails if a builder
  treats the fan-out maximum as the payload guard and skips the size check.
- The documented remedy behaves as documented on each surface, per
  `AC-PLUGINS-TASK-DEPS-004.15`: the canvas list route rejects `limit=0` and
  `limit=201` with `invalid_request`, and the host's normalization clamps the same
  inputs and defaults an absent limit to 50. These assert inherited behaviour
  rather than new behaviour, and exist so the remedy's two contracts cannot drift
  apart unnoticed while two criteria still name one remedy.
- The oversized-response refusal uses the existing codes and precedes
  serialization: an over-maximum page returns `response_too_large` on the canvas
  surface and `ResourceExhausted` on the gRPC surface, with no task object in the
  response.

## Documentation

Four of the seven consumer-contract items listed in the
[projection design](task-dependency-projection.md#documentation) are this
design's, all of them about what a response costs or weighs:

- The `ResourceExhausted` failure mode of an oversized gRPC response, and that
  the receive maximum belongs to the plugin's own client connection rather than
  to the host (`AC-PLUGINS-TASK-DEPS-004.7`).
- That the plugin-owned task-tree preview RPC accepts no page `limit`, so
  reducing the tree is the only lever on its size
  (`AC-PLUGINS-TASK-DEPS-004.9`).
- The projection's maximum contribution to a single-task response as a byte
  figure -- two lists of at most 512 entries, roughly 150 KB -- so a plugin author
  can size a lowered `MaxCallRecvMsgSize` against it
  (`AC-PLUGINS-TASK-DEPS-004.9`).
- The page `limit`'s accepted range, default and out-of-range behaviour on each
  surface, because the canvas rejects an out-of-range limit while the host's
  normalization clamps it and defaults it to 50
  (`AC-PLUGINS-TASK-DEPS-004.15`). Without this a consumer following the
  smaller-`limit` remedy cannot predict what it will get.

They belong in `docs/public/canvases.md`, `docs/public/plugins-authoring.md`, and
the canvas authoring browser-API reference.

## Related decisions

- [ADR 0043 — Plugin host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Isolated web-app contributions](isolated-web-app-contributions.md)
- [Task dependency projection](task-dependency-projection.md)
- [Dependency edge ends](task-dependency-edge-ends.md)
