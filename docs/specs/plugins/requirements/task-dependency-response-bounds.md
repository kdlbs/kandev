---
id: plugins-task-dependency-response-bounds
title: Response bounds for the task dependency projection
status: draft
system: plugins
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Response bounds for the task dependency projection Requirements

## Overview

The [task dependency projection](task-dependency-projection.md) adds seven fields
to the plugin and canvas task read model, two of them lists that name other
tasks. That turns a task read into a read whose cost and whose payload both grow
with data the caller did not ask for: the edges of every task on the page, and
the far task rows behind those edges.

This document owns the third question the capability raises — **what one response
may cost, and what happens when it cannot be delivered.** It is separated from
the projection requirements because the two are reviewed against different risks.
The projection decides what the graph says; this decides what happens when the
graph is too expensive to compute or too large to send, and it is the part that
must stay honest when a workspace is larger than the one the capability was
demonstrated on.

`REQ-PLUGINS-TASK-DEPS-004` keeps the identity it was reviewed under; nothing is
renumbered. The projection's criteria defer here wherever a query count, a list
length, or a response size is at stake, and the sibling
[dependency edge ends](task-dependency-edge-ends.md) defers here for the query
bound its query-free admission test is written against.

## Terminology

- **Plugin task read:** As defined in
  [task dependency projection](task-dependency-projection.md) — any surface
  returning the plugin task read model, on either the gRPC or the canvas surface.
- **Edge-end resolution:** The derivation step that loads the far task row behind
  each edge, in order to give an entry its `title` and `state`. Its input is the
  set of distinct edge ends across the whole response, deduplicated, not one
  task's list, and narrowed before the step runs by
  `AC-PLUGINS-TASK-DEPS-004.13`.
- **Distinct edge ends:** The deduplicated set that edge-end resolution reads.
  **Serialized entries** are a different quantity: the entries a response writes
  across all its tasks, which repeat wherever two tasks share an end. The two are
  bounded separately and by different criteria; see
  `AC-PLUGINS-TASK-DEPS-004.14`.
- **Host-parameter ceiling:** The database's maximum number of bind parameters in
  one statement, below which a batched `IN (...)` query must stay.
- **Paginated flow:** A plugin task read accepting a page `limit`. The list reads
  are paginated; the single-task reads, the task-write RPCs, and the
  plugin-owned task-tree preview RPC are not.

## Requirements

### REQ-PLUGINS-TASK-DEPS-004: Bounded cost and bounded payload

**Intent:** Reading a page of tasks must not cost one dependency query per task;
no task may become unreadable because too many tasks depend on it; and a response
that cannot be delivered must fail in a way that says so, rather than arriving
complete-looking and wrong.

#### Acceptance criteria

- **AC-PLUGINS-TASK-DEPS-004.1:** No dependency read shall be issued per task in
  the response. The derivation's own structural steps shall be a fixed number of
  reads that does not change with the number of tasks in the response: a response
  of many tasks carrying no edges shall cost the same reads as a response of one.
  The only read count permitted to vary is the batch count edge-end resolution
  requires under `AC-PLUGINS-TASK-DEPS-004.11`, which varies with the number of
  distinct edge ends and not with the number of tasks, and which on a paginated
  flow is therefore bounded by the maximum in `AC-PLUGINS-TASK-DEPS-004.10`
  divided by the host-parameter ceiling. Two responses resolving the same number
  of distinct edge ends shall cost the same reads whether they carry one task or
  many, with the single exception that a response carrying more tasks than the
  host-parameter ceiling allows in one statement chunks its task-keyed edge
  queries under `AC-PLUGINS-TASK-DEPS-004.11` — reachable only on the unpaginated
  preview flow, since every paginated flow is capped at 200 tasks. No criterion
  shall be read as requiring a single fixed read count across responses of
  differing edge volume or differing task count past that ceiling, which chunking
  makes impossible to hold.
- **AC-PLUGINS-TASK-DEPS-004.2:** The dependency derivation for a task list
  shall cover only the tasks in the returned page, not every task the filter
  matched before pagination.
- **AC-PLUGINS-TASK-DEPS-004.3:** `depends_on` and `blocks` shall each carry at
  most 512 entries for one task.
- **AC-PLUGINS-TASK-DEPS-004.4:** When a list is cut to that limit, the read
  shall report that list's truncation flag true and keep the first entries in the
  order defined by `AC-PLUGINS-TASK-DEPS-001.8`. Both flags shall be present on
  every task and false when no cut occurred.
- **AC-PLUGINS-TASK-DEPS-004.5:** A truncated list shall not change `blocked` or
  `blocked_reason`, which shall still account for every predecessor including the
  omitted ones.
- **AC-PLUGINS-TASK-DEPS-004.6:** When a canvas response carrying the projection
  exceeds the runtime response limit, the runtime shall return its existing
  bounded `response_too_large` error. That limit shall be stated as applying to
  every canvas response rather than to the list routes alone, because the runtime
  applies it to each response it writes. A smaller page `limit` is the documented
  remedy on the paginated flows only; `AC-PLUGINS-TASK-DEPS-004.9` governs the
  flows that accept no `limit`.
- **AC-PLUGINS-TASK-DEPS-004.7:** The gRPC surface's bound shall be stated, not
  assumed absent. A Host response over the receiving end's maximum receive size
  shall fail the whole RPC with `ResourceExhausted`, never a truncated or
  partially populated list, so a consumer never reads a short page as the end of
  the data. The remedy is the same smaller `limit` on the paginated flows only,
  and the documentation shall say the size belongs to the plugin's own client
  connection, not to this capability.
- **AC-PLUGINS-TASK-DEPS-004.8:** Neither bound shall be met by dropping,
  truncating, or redacting the projection to fit. `AC-PLUGINS-TASK-DEPS-004.3` is
  this capability's only size reduction; an undeliverable response fails rather
  than arriving silently incomplete.
- **AC-PLUGINS-TASK-DEPS-004.9:** The remedy in `AC-PLUGINS-TASK-DEPS-004.6` and
  `AC-PLUGINS-TASK-DEPS-004.7` shall not be claimed for a flow that accepts no
  page `limit`, and those flows shall be named rather than left to inference: the
  gRPC single-task read, the three gRPC task-write RPCs, the canvas single-task
  and update routes, and the plugin-owned task-tree preview RPC. On every one of
  them that returns exactly one task, the projection's contribution to the
  response shall be at most the two lists bounded by
  `AC-PLUGINS-TASK-DEPS-004.3`. That is a bound on **serialized entries**, and it
  shall be grounded on `AC-PLUGINS-TASK-DEPS-004.3` alone rather than on the
  distinct-end maximum of `AC-PLUGINS-TASK-DEPS-004.10`, from which those flows
  are exempt under that criterion. The contribution shall be stated as a byte
  figure in the documentation, and what it places on the gRPC surface shall be
  stated as a floor on the receiving end rather than as a guarantee this
  capability can enforce: a plugin whose client receive maximum is at or above the
  gRPC library default shall not be able to trip `AC-PLUGINS-TASK-DEPS-004.7`
  through the projection on such a flow, and a plugin that lowers that maximum
  below the stated figure may — a consequence of its own connection setting, which
  `AC-PLUGINS-TASK-DEPS-004.7` already assigns to it. No criterion shall claim the
  bound holds against a receive maximum the host does not set. The preview RPC
  shall be the one exception, because it returns an unpaginated task set: its
  response size and its edge-end total shall be bounded only by the size of the
  plugin-owned tree, and
  the documentation shall state that reducing that tree is the only lever on it.
- **AC-PLUGINS-TASK-DEPS-004.10:** The number of distinct edge ends one
  derivation resolves shall be bounded by a declared maximum **on every paginated
  flow**, and that maximum shall exceed twice the per-task limit in
  `AC-PLUGINS-TASK-DEPS-004.3`.
  A response whose task set would exceed it shall fail under
  `AC-PLUGINS-TASK-DEPS-004.6` or `AC-PLUGINS-TASK-DEPS-004.7`. Exceeding it
  shall not count as a derivation that cannot be completed under
  `AC-PLUGINS-TASK-DEPS-002.5`, and shall therefore never produce the withheld
  verdict of `AC-PLUGINS-TASK-DEPS-001.14`: such a response is undeliverable, not
  unknown, and a consumer reducing its page shall receive a real graph rather than
  a page uniformly reporting `blocked_reason: unknown`. The refusal shall reuse
  each surface's existing oversized-response error rather than introduce a code:
  `response_too_large` on the canvas surface and `ResourceExhausted` on the gRPC
  surface, returned before any task object is serialized, so a caller cannot
  receive a partial page alongside it.
  The flows named in `AC-PLUGINS-TASK-DEPS-004.9` **that return exactly one
  task** shall be exempt from this maximum. They accept no page `limit`, so the
  remedy the refusal assumes does not exist on them, and refusing there would make
  a task unreadable exactly because too many tasks name it — the outcome this
  requirement's Intent forbids. On an exempt flow the derivation shall resolve
  whatever `AC-PLUGINS-TASK-DEPS-004.13` leaves it, in batches under
  `AC-PLUGINS-TASK-DEPS-004.11`, and shall not refuse on end count; the response
  shall remain subject to the size bounds of `AC-PLUGINS-TASK-DEPS-004.6` and
  `AC-PLUGINS-TASK-DEPS-004.7`, which act on serialized entries under
  `AC-PLUGINS-TASK-DEPS-004.14` and which `AC-PLUGINS-TASK-DEPS-004.3` already
  bounds for one task. The cost this concedes is named in `## Out of scope`.
  The preview RPC, which `AC-PLUGINS-TASK-DEPS-004.9` names in the same list of
  unpaginated flows, shall **not** be exempt: it returns many tasks, its
  documented remedy of reducing the plugin-owned tree is a real lever, and
  refusing it leaves every task in that tree individually readable through an
  exempt flow, so no task becomes unreadable.
  The maximum's floor of twice `AC-PLUGINS-TASK-DEPS-004.3` is what keeps the
  paginated and exempt regimes agreeing in the ordinary case: after
  `AC-PLUGINS-TASK-DEPS-004.13`'s cut, a task holding no more predecessors than
  that limit contributes at most twice it, so a paginated page of one such task
  stays under the maximum and reads the same as the exempt single-task flow. The
  two can still disagree for a task that passed the predecessor limit on the
  incremental write path, which a paginated read at the minimum `limit` of 1 would
  refuse with no smaller `limit` left to retry. That residue shall not be left to
  inference: the documented remedy there is the single-task read, which is exempt
  and returns the same projection.
- **AC-PLUGINS-TASK-DEPS-004.11:** No statement in the derivation shall carry a
  bind-parameter count that grows without bound with its input. This binds **two**
  families of statement, not one, and naming only the first is the gap this
  criterion closes. Edge-end resolution shall split its distinct ends into batches
  at or below the host-parameter ceiling. The task-keyed edge queries — the
  batched predecessor and dependent reads, whose parameters are the response's
  task ids — shall be split the same way; that bound is unreachable on a paginated
  flow, where `AC-PLUGINS-TASK-DEPS-004.15`'s maximum of 200 tasks keeps them to
  one statement, and is reachable only on the unpaginated preview flow. Both shall
  reuse the chunking convention the repository already applies to its other
  batched id queries rather than introducing a second one. No derivation
  shall fail because a generated statement exceeded that ceiling, so none of
  `AC-PLUGINS-TASK-DEPS-002.5`'s failure paths shall be reachable by that route.
- **AC-PLUGINS-TASK-DEPS-004.12:** Splitting edge-end resolution into batches
  shall change neither the failure granularity nor the ordering of the result.
  The batches shall remain one logical step, so a failure in any one of them shall
  be the single edge-end-resolution failure of `AC-PLUGINS-TASK-DEPS-002.5` over
  the whole derivation and shall not become a narrower per-batch boundary under
  `AC-PLUGINS-TASK-DEPS-002.3`. Entry order shall continue to come from the edge
  queries of `AC-PLUGINS-TASK-DEPS-001.8` and shall not depend on which batch
  resolved an end. Because the batches are not one statement, the derivation's
  observation point shall be an interval rather than an instant; this is already
  permitted by `AC-PLUGINS-TASK-DEPS-005.4` and
  `AC-PLUGINS-TASK-DEPS-005.6`, and shall not be read as a stronger guarantee
  than either.
- **AC-PLUGINS-TASK-DEPS-004.13:** Edge-end resolution shall read only the ends
  whose detail the response can carry, except where another criterion requires a
  wider read. The **dependent** direction shall be ordered and cut to the limit in
  `AC-PLUGINS-TASK-DEPS-004.3` *before* resolution, because no criterion reads a
  dependent's far row to compute a verdict — `AC-PLUGINS-TASK-DEPS-001.7` gives
  dependent entries no resolution status — and because
  `AC-PLUGINS-TASK-DEPS-001.8`'s order and its tiebreak are both decidable from
  the edge rows alone, so the cut does not need the detail it is deciding about.
  The **predecessor** direction shall be resolved in full, because
  `AC-PLUGINS-TASK-DEPS-004.5` requires the verdict to account for every
  predecessor and a predecessor's `status` is read from its far row. One task's
  contribution to a derivation's distinct-end count shall therefore be its full
  predecessor count plus at most the limit in `AC-PLUGINS-TASK-DEPS-004.3`, not
  its full edge count.
  Because that cut precedes resolution, an end kept by it and then dropped as
  dangling under `AC-PLUGINS-TASK-DEPS-006.4` shall leave the list **shorter than
  the limit with its truncation flag still true**, and the entries the cut
  discarded shall not be back-filled in its place. A shorter-than-limit truncated
  list is therefore correct rather than a defect, and no criterion shall be read
  as requiring a truncated list to hold exactly the limit.
- **AC-PLUGINS-TASK-DEPS-004.14:** The quantity the response-size bounds of
  `AC-PLUGINS-TASK-DEPS-004.6` and `AC-PLUGINS-TASK-DEPS-004.7` act on shall be
  the number of entries a response **serializes**, which is not the distinct-end
  count bounded by `AC-PLUGINS-TASK-DEPS-004.10` and shall not be documented as
  though it were. Entries repeat across tasks that share an end, so one response
  may serialize as many as the page limit times twice the per-task limit of
  `AC-PLUGINS-TASK-DEPS-004.3` while resolving far fewer distinct ends. A response
  may therefore exceed a surface's size limit while staying under the distinct-end
  maximum, and shall then fail under `AC-PLUGINS-TASK-DEPS-004.6` or
  `AC-PLUGINS-TASK-DEPS-004.7` with the smaller-`limit` remedy. No criterion and
  no design prose shall state or imply that `AC-PLUGINS-TASK-DEPS-004.10` bounds
  response size, or that the two bounds engage at comparable payloads.
- **AC-PLUGINS-TASK-DEPS-004.15:** The page `limit` that the remedy names shall
  have its accepted range, its default, and its out-of-range behaviour documented
  for each surface, because the two surfaces already differ and this capability
  changes neither. The canvas surface shall continue to **reject** a `limit`
  **supplied** outside 1 to 200, or non-numeric, with its existing
  invalid-request error, while the host's own page normalization shall continue to
  **clamp** a supplied out-of-range value into that range. The divergence shall be
  documented as applying to a supplied value only: an **absent** `limit` is not an
  error on either surface and shall continue to reach the host's default of 50. A
  consumer retrying with a smaller `limit` shall be able to predict which of the
  two behaviours it will get, and no criterion shall describe the remedy as though
  they were the same.

## Out of scope

- **Paginating the preview RPC.** `AC-PLUGINS-TASK-DEPS-004.9` names it as the
  one flow with no applicable remedy rather than adding a page parameter to it.
  That RPC's request and response shape is owned by the plugin-owned task-tree
  contract, and changing it is a wire change this capability does not need: the
  projection inherits the tree's existing unboundedness rather than introducing
  it, and the same is already true of the linked pull requests attached there.
- **A cost bound on a single-task read's own edges.** The exemption in
  `AC-PLUGINS-TASK-DEPS-004.10` concedes that a non-paginated single-task read of
  a task with an extreme predecessor count resolves that many ends, in
  `AC-PLUGINS-TASK-DEPS-004.11` batches, rather than refusing. This is deliberate,
  and it is the narrower of the two costs on offer: the alternative is precisely
  the unreadable task this requirement's Intent forbids, and the caller named that
  one task by id rather than asking for a page. The dependent direction — the one
  an arbitrary third party can grow without the task's consent, and the one the
  Intent is worded against — is cut before resolution by
  `AC-PLUGINS-TASK-DEPS-004.13`, so the residue is confined to a task's own
  declared predecessors, bounded at 512 on the full-set replacement write path and
  reachable past it only on the incremental add path.
- **A time or memory budget per derivation.** The bounds here are counts —
  reads, entries, distinct ends, and bytes — because those are the values the
  contract can state and a test can assert. A latency budget would be an
  operational target, not a contract a consumer can rely on.
- **Chunking as a substitute for the maximum.** `AC-PLUGINS-TASK-DEPS-004.11`
  keeps a statement legal; `AC-PLUGINS-TASK-DEPS-004.10` keeps the response
  affordable. Chunking alone would turn an oversized page into an unbounded number
  of small queries, which satisfies no bound and is the fault
  `AC-PLUGINS-TASK-DEPS-004.1` exists to prevent.
- **A server-side cap that silently shrinks a page.** Excluded by
  `AC-PLUGINS-TASK-DEPS-004.8`. Returning fewer tasks than asked for, without an
  error, is the failure mode `AC-PLUGINS-TASK-DEPS-004.7` rejects for the gRPC
  surface, and it is no better on the canvas surface.
