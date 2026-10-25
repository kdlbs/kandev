---
id: plugins-task-dependency-projection
title: Task dependency projection on the plugin data API
status: draft
system: plugins
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Task dependency projection on the plugin data API Requirements

## Overview

The plugin task read model exposes `parent_id` and nothing else about how one
task relates to another. Dependency edges ("task B cannot start until task A
succeeds") are derived for every Kandev board read, but the Plugin Host data API
and the isolated-canvas data protocol both drop them, so a gRPC plugin or a
canvas draws the parent/child tree ("part of") and never the dependency graph
("not until") from tasks it has already read. This capability adds the edges, the
derived blocked verdict, and the deferred-launch intent to that read model as a
read-only projection, on both surfaces.

Plugins owns it because the extended contract is the plugin and canvas read
model ([plugins README](../README.md)). Dependency semantics stay with Tasks
([task dependencies](../../tasks/system-design/task-dependencies.md)), which this
capability re-projects, never redefines. Identities use the `TASK-DEPS` token,
distinct from `REQ-TASKS-TASK-DEPENDENCIES-001`.

Two sibling documents own the questions this one does not. What an edge *end*
may say lives in [dependency edge ends](task-dependency-edge-ends.md), which owns
`REQ-PLUGINS-TASK-DEPS-003` (which ends a canvas may see described, and what is
left of one it may not) and `REQ-PLUGINS-TASK-DEPS-006` (ends the response does
not contain: archived, out of page, or removed); it defines *canvas scope* and
*redaction*. What one response may cost and how large it may be lives in
[response bounds](task-dependency-response-bounds.md), which owns
`REQ-PLUGINS-TASK-DEPS-004` (query count, list cut, edge-end fan-out, and the
response-size bound on each surface).

This document owns the projection itself: the field set, the derivation, the
order, and the refresh rule. Criteria here defer to the edge-ends sibling
wherever an end's `title`, `state`, or existence is at stake, and to the
response-bounds sibling wherever a query count, a list length, or a response size
is.

## Prior art

**Leg 1: Kandev's own prior reasoning (wiki). Receipt: did not run.** No
`wiki-query` in either skill directory or on `PATH`, no `~/.obsidian-wiki/`, no
`OBSIDIAN_VAULT_PATH`; no vault path or QMD collection can be reported.
Tool-unavailable, not an empty result.

In its place the in-repository record was searched (`scripts/list-docs.py
decisions`, `docs/specs/{plugins,tasks}/**`). Three positions found there are
adopted rather than re-derived: **ADR 0043** (DTO fields are additive-only;
hand-mapped DTOs read through the service layer);
**[isolated web-app contributions](../system-design/isolated-web-app-contributions.md)**
(canvas routes reuse the Host data API DTOs, so a field added for one adapter is
returned by both with the same nullability and authorization, as
`workflow_step_id` established); and
**[task dependencies](../../tasks/system-design/task-dependencies.md)** (blocked
state is derived on every read, never persisted, because the auto-start gate
would read a stale copy).

**Leg 2: what other products shipped (saas-kb). Receipt: did not run.** No
`search_fsm_docs` tool and no `saas-kb` MCP server in this session's tool
surface, and no tool-discovery tool to find one. No query issued.

**What we are doing differently.** Two departures, both normative below. The
board treats an **absent** projection as "unknown" and an empty list as "no
edges"; this contract computes it on every read, so within one host absence never
occurs (`AC-PLUGINS-TASK-DEPS-001.2`) and an underivable projection reports the
withheld verdict (`REQ-PLUGINS-TASK-DEPS-002`). And `Task.pull_requests` degrades
to an empty list on a failed lookup; dependencies must not, because an empty edge
list is indistinguishable from a real answer.

## Terminology

- **Dependency edge:** A directed peer-to-peer relation "this task depends on
  that task", distinct from the parent/child subtask hierarchy. Its two ends are
  the **predecessor** and the **dependent**, listed in `depends_on` and in
  `blocks` respectively; **edge end** means either.
- **Projection:** The derived, never-persisted dependency field set on one task,
  enumerated in `AC-PLUGINS-TASK-DEPS-001.1`.
- **Plugin task read:** Any surface returning the plugin task read model: the
  gRPC `ListTasks`, `GetTask`, task-write and task-tree-preview RPCs, and the
  canvas `GET`/`PATCH ./_kandev/v1/data/tasks[/{task_id}]` routes.

## Requirements

### REQ-PLUGINS-TASK-DEPS-001: Dependency projection on the plugin task read model

**Intent:** A canvas author reading tasks draws the dependency graph from the
task read model alone, each task naming its direct predecessors and dependents
with the same edges and verdict the board shows, without a second data source.

#### Acceptance criteria

- **AC-PLUGINS-TASK-DEPS-001.1:** Every plugin task read shall return
  `blocked`, `blocked_reason`, `depends_on`, `blocks`, `depends_on_truncated`,
  `blocks_truncated`, and `start_when_unblocked` for each task in the response.
- **AC-PLUGINS-TASK-DEPS-001.2:** Every one of those fields shall be present on
  every task returned by a host implementing this capability, including a task
  with no edges, so a consumer never treats absence as a third state. Scoped by
  `AC-PLUGINS-TASK-DEPS-001.15`.
- **AC-PLUGINS-TASK-DEPS-001.3:** When `blocked_reason` is not `unknown`, an
  empty `depends_on` or `blocks` shall mean "no edge in that direction". When it
  is `unknown`, both lists shall be non-authoritative and shall not be read as
  "no edges"; `blocked_reason` shall be the only discriminator.
- **AC-PLUGINS-TASK-DEPS-001.4:** `blocked_reason` shall be exactly one of
  `pending`, `failed`, `unknown`, or the empty value, and shall be the empty
  value if and only if `blocked` is false.
- **AC-PLUGINS-TASK-DEPS-001.5:** `depends_on` shall list only direct
  predecessors and `blocks` only direct dependents. Neither shall be transitive
  and neither shall contain the task itself.
- **AC-PLUGINS-TASK-DEPS-001.6:** Each `depends_on` entry shall carry the
  predecessor's `id`, `title`, `state`, and a `status` that is exactly one of
  `resolved`, `failed`, or `pending`, subject to `REQ-PLUGINS-TASK-DEPS-003`.
- **AC-PLUGINS-TASK-DEPS-001.7:** Each `blocks` entry shall carry the
  dependent's `id`, `title`, and `state`, and no resolution status, subject to
  `REQ-PLUGINS-TASK-DEPS-003`.
- **AC-PLUGINS-TASK-DEPS-001.8:** `depends_on` shall be ordered by edge creation
  time ascending, tiebroken by predecessor task id ascending; `blocks` likewise,
  tiebroken by dependent task id ascending. Both orders shall be total and
  repeatable for unchanged data.
- **AC-PLUGINS-TASK-DEPS-001.9:** For the same task and edges, the gRPC read and
  the canvas read shall report the same values for every projection field,
  allowing only the scope redaction in `REQ-PLUGINS-TASK-DEPS-003`.
- **AC-PLUGINS-TASK-DEPS-001.10:** `start_when_unblocked` shall report the stored
  deferred-launch intent as recorded, not a judgement about whether it can still
  fire: a task whose last edge was removed keeps reporting true, because removing
  an edge does not clear the intent. It shall be read-only on every plugin
  surface, and false under the withheld verdict of `AC-PLUGINS-TASK-DEPS-001.14`.
- **AC-PLUGINS-TASK-DEPS-001.11:** A plugin task read shall require no capability
  beyond `api_read:tasks`. A surface whose own gate requires it shall keep
  rejecting a caller without it outright, never turning that rejection into a
  successful response. On a surface gated by something else, including the gRPC
  task-write RPCs whose write capability gates independently, a caller without
  `api_read:tasks` shall receive the withheld verdict of
  `AC-PLUGINS-TASK-DEPS-001.14` and never an edge list, an edge end, or a
  `blocked` value derived from real data.
- **AC-PLUGINS-TASK-DEPS-001.12:** Reading a task shall not change any task,
  edge, or launch intent, and repeating a read shall only report the current
  derivation.
- **AC-PLUGINS-TASK-DEPS-001.13:** A task object returned by a plugin or canvas
  write shall carry the projection derived after that write commits, matching what
  a read would report at the instant of derivation, subject to
  `AC-PLUGINS-TASK-DEPS-001.11`. Because that derivation takes no lock
  (`AC-PLUGINS-TASK-DEPS-005.6`), an edge mutated between commit and derivation may
  appear; a consumer needing a later state refetches under
  `REQ-PLUGINS-TASK-DEPS-005`.
- **AC-PLUGINS-TASK-DEPS-001.14:** The withheld verdict shall be `blocked: true`,
  `blocked_reason: unknown`, empty `depends_on`, empty `blocks`, both truncation
  flags false, and `start_when_unblocked` false. It shall be reported on any
  response that returns a task object but cannot carry a real projection, so no
  response asserts "not blocked" without authority for that claim. It shall never
  be the body of a response that should have been rejected instead;
  `AC-PLUGINS-TASK-DEPS-001.11` governs which callers reach it.
- **AC-PLUGINS-TASK-DEPS-001.15:** The presence guarantee in
  `AC-PLUGINS-TASK-DEPS-001.2` shall hold within one host version only. The wire
  form carries no field presence, so a host predating this capability yields the
  bytes of `blocked: false` with empty lists, indistinguishable from a real "not
  blocked, no edges" answer.
- **AC-PLUGINS-TASK-DEPS-001.16:** The mechanism separating the two shall be the
  manifest's `min_kandev_version` floor, declared by the consumer as the first
  host release carrying this capability and enforced at install time: an older
  host rejects the install outright, with no partial registration. The concrete
  version shall be pinned at release and never guessed ahead of one. It is a
  load-time gate, not a runtime check. No runtime capability or version signal
  shall be added, because the projection adds no capability -- it is a field set
  on the already-approved `api_read:tasks` resource, the gRPC handshake version is
  fixed, and the canvas protocol version does not change.
- **AC-PLUGINS-TASK-DEPS-001.17:** The floor shall not be a capability-keyed
  minimum on `api_read:tasks`, which would reject every installed plugin that
  reads tasks without this capability. It is therefore declared, not validated,
  and the public documentation shall state the floor, the effect of omitting it,
  and that a sideloaded plugin bypassing the install path gets the ambiguous
  bytes with no signal.
- **AC-PLUGINS-TASK-DEPS-001.18:** In the canvas JSON an empty `depends_on` or
  `blocks` shall be encoded as an empty array, never `null` and never omitted: a
  null is the third state `AC-PLUGINS-TASK-DEPS-001.2` forbids and is the default
  encoding of an unallocated list, so this binds the encoder, not the derivation.
  The withheld verdict shall be encoded the same way.

### REQ-PLUGINS-TASK-DEPS-002: Fail-closed derivation

**Intent:** A read that cannot be answered must say so; an empty edge list is
indistinguishable from a real answer and renders a confidently wrong graph.

#### Acceptance criteria

- **AC-PLUGINS-TASK-DEPS-002.1:** When the dependency derivation for a task
  cannot be completed, the read shall report the withheld verdict of
  `AC-PLUGINS-TASK-DEPS-001.14` for that task.
- **AC-PLUGINS-TASK-DEPS-002.2:** A failed dependency derivation shall not fail
  the surrounding task read, shall not remove any other field of the task, and
  shall not omit any projection field.
- **AC-PLUGINS-TASK-DEPS-002.3:** Failure granularity shall follow the derivation's
  batch boundaries. A batch-wide failure shall produce the withheld verdict for
  every task in that batch; a failure confined to one task's edge-end resolution
  shall produce it for that task alone. Per-task isolation of a batch-wide failure
  shall not be required, because `AC-PLUGINS-TASK-DEPS-004.1` forbids the per-task
  queries needed to detect it.
- **AC-PLUGINS-TASK-DEPS-002.4:** A failed dependency derivation shall emit one
  structured log record per derivation attempt, not one per withheld task. The
  record shall carry plugin, instance, resource type, operation, and result code,
  and shall not record task titles, task identifiers, or edge contents. This
  capability shall add no counter and no metric namespace; the log record is its
  whole observability contract.
- **AC-PLUGINS-TASK-DEPS-002.5:** Each of the following shall count as a
  derivation that cannot be completed and shall produce the withheld verdict, not
  a partial or degraded result: a failed read of the predecessor direction; a
  failed read of the dependent direction; a failed resolution of edge-end detail;
  and a derivation source absent or not wired to the read at all. None shall be
  reported as an empty list carrying a `blocked_reason` other than `unknown`.

### REQ-PLUGINS-TASK-DEPS-005: Refresh contract for dependency changes

**Intent:** A consumer must know how it learns the graph changed, without the
event stream becoming a second source of truth for edges.

#### Acceptance criteria

- **AC-PLUGINS-TASK-DEPS-005.1:** Canvas events shall not carry the dependency
  projection, any edge list, or any edge end's title or state.
- **AC-PLUGINS-TASK-DEPS-005.2:** Adding or removing a dependency edge shall
  publish a `task.updated` event for both ends of that edge. Delivery to a canvas
  shall follow the existing event scope matching, which for repository and session
  scopes matches on a single identity carried by the event rather than the full
  set the direct-read predicate accepts. The contract shall therefore not promise
  delivery to every canvas whose scope admits an end; the consumer's guarantee is
  `AC-PLUGINS-TASK-DEPS-005.3`.
- **AC-PLUGINS-TASK-DEPS-005.3:** The documented refresh rule shall be that a
  consumer re-reads the affected tasks after `task.updated`,
  `task.dependencies_resolved`, or `task.dependency_failed`, because none of
  those events identifies which fields changed.
- **AC-PLUGINS-TASK-DEPS-005.4:** One response shall not be a guaranteed
  transactional snapshot: a consumer shall be documented as unable to assume that a
  task appearing in another task's `blocks` also lists that task in its own
  `depends_on` within the same response.
- **AC-PLUGINS-TASK-DEPS-005.5:** The refresh rule and the non-snapshot rule shall
  be stated in the public canvas and plugin authoring documentation.
- **AC-PLUGINS-TASK-DEPS-005.6:** Deriving the projection shall acquire no
  dependency mutation lock, so a read shall neither delay nor be delayed by a
  concurrent edge add or remove, and concurrent reads of the same task shall not
  serialize against each other.
- **AC-PLUGINS-TASK-DEPS-005.7:** The rule shall also cover a predecessor's
  ordinary state change, which the three events above miss: a predecessor moving
  state while another predecessor is still pending publishes neither
  `task.dependencies_resolved` nor `task.dependency_failed` for the dependent,
  only `task.state_changed` for itself, so the `state` on that dependent's
  `depends_on` entry would otherwise stay stale indefinitely. The documented rule
  shall require a consumer holding a cached task to re-read it on
  `task.state_changed` for any task named in its `depends_on` or `blocks`.
- **AC-PLUGINS-TASK-DEPS-005.8:** No dependent-side event shall be added for a
  predecessor state change: the event contract is owned elsewhere, the fan-out is
  unbounded in the dependent count, and `AC-PLUGINS-TASK-DEPS-005.2` already
  declines to promise scope-complete delivery. A consumer rule over an
  already-published event is the guarantee instead.

## Out of scope

- **Dependency writes.** Neither gRPC `UpdateTask` nor the canvas `PATCH` route
  gains an edge field; a consumer mutates edges through the Kandev task API or UI.
  A write contract needs cycle rejection, cross-workspace rejection, and Tasks'
  serialized validate-then-insert path.
- **A blocked filter on the task list.** The filter keeps workspace, workflow,
  state, and parent only; a consumer pages and filters on `blocked` itself. A
  server-side filter would evaluate a derived, unindexed value: a Tasks decision.
- **Session reads for canvases.** The canvas data surface exposes only tasks and
  workflows, so a canvas colours a node by task `state` but cannot tell a running
  session from a waiting one. `data/sessions` is a separate extension.
- **Dependency fields in the event payload.** Excluded by
  `AC-PLUGINS-TASK-DEPS-005.1`: the event projection has no per-entry scope
  evaluation, so edge ends there would bypass `REQ-PLUGINS-TASK-DEPS-003`.
- **Event routing enrichment.** `AC-PLUGINS-TASK-DEPS-005.2` accepts that a
  repository- or session-scoped canvas may miss a `task.updated` for a task it
  could read directly. Widening routing changes the event contract, owned
  elsewhere; `AC-PLUGINS-TASK-DEPS-005.3` is the guarantee instead.
- **Transitive closure, path finding, and cycle reporting.** Both lists are
  direct edges only; a consumer composes reachability from what it read.
- **A transactional snapshot across one response.** Excluded by
  `AC-PLUGINS-TASK-DEPS-005.4`; symmetry would need one transaction across the
  whole batched derivation.
- **A new capability id.** The projection is a field set on the already-approved
  `api_read:tasks` resource; adding fields changes neither a plugin manifest nor
  its digest, so no re-approval is triggered.
- **A runtime host-version signal.** Excluded by `AC-PLUGINS-TASK-DEPS-001.16`
  in favour of the install-time `min_kandev_version` floor.
- **Further fields on the projection or on an edge end.** Considered in Spec and
  deferred as a set: true `depends_on_count` and `blocks_count` beside the
  truncation flags, an end's `archived_at`, `identifier`, or `parent_id`, and the
  edge's own `created_at`. Every one of them is already loaded and then discarded
  by the derivation, so this is a scope decision rather than a cost one. Two
  things make them a later capability instead of a late addition here: each
  edge-end field must be placed on one side of
  `AC-PLUGINS-TASK-DEPS-003.8`'s redaction line, which reopens that whole table,
  and any new field reopens `AC-PLUGINS-TASK-DEPS-001.1` and
  `AC-PLUGINS-TASK-DEPS-001.2`. ADR 0043's additive-only rule means each can ship
  later without breaking a consumer; the carrying cost is that
  `AC-PLUGINS-TASK-DEPS-001.15`'s "within one host version" presence guarantee
  makes every later addition repeat the version-skew ambiguity this contract
  prices once in `AC-PLUGINS-TASK-DEPS-001.16` and
  `AC-PLUGINS-TASK-DEPS-001.17`.
