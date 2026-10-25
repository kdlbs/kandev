---
status: draft
system: tasks
created: 2026-09-05
owners:
  - kandev
---

# Orphaned Workspace Task Indicator Requirements

## Overview

An `inherit_parent` subtask borrows its parent's materialized workspace. When the
parent is archived that workspace is removed and the subtask can never be launched
again. PR #3235 (`84c83c3d5`) detects this at archive time and stamps a durable
marker into the subtask's `metadata.workspace` map. Nothing reads it: the subtask
still renders as an ordinary `CREATED` card, identical to a launchable one, until a
start attempt fails closed.

This capability makes that state visible. It owns one invariant:

> **A task that cannot start must not render identically to one that can.**

The `tasks` system owns the contract: a task-level marker on `tasks.metadata`
projected onto the task DTO, like
[interrupted-task-indicator](interrupted-task-indicator.md). The board consumes it
rather than owns it, so its outcomes belong here.

## Terminology

- **Orphaned workspace:** a live, non-archived, non-ephemeral, non-automation-origin
  task whose `metadata.workspace.mode` is `inherit_parent`, whose parent is archived,
  and which has no `task_environments` row of its own. The producer's exact predicate:
  its first three clauses are `ListChildren`'s own filter, which both mark sites
  select through, so a repair omitting them marks a wider population.
- **Orphan marker:** the four keys the producer writes under
  `tasks.metadata.workspace` — `orphaned` (bool), `orphaned_reason`
  (`parent_archived`), `orphaned_parent_id`, `orphaned_at` (RFC 3339 UTC).
- **Marked:** carrying `workspace.orphaned` with the boolean value `true`. A key
  present with any other value is **unmarked**, everywhere in this document.
- **Born stranded:** a task created with `workspace_mode=inherit_parent` under an
  already-archived parent; archive-time detection cannot reach these.
- **Board surfaces:** the kanban card and the graph step node, the two already
  rendering `auto_start_failed`.

## Prior art

Legs and receipts are in the indicator design's `## Prior art`. The wiki and `saas-kb`
legs DID NOT RUN, both tools unavailable; the repository leg RAN and found
[interrupted-task-indicator](interrupted-task-indicator.md), the direct precedent
whose shape this reuses.

## Measured population

Measured 2026-09-05 against this instance's database, re-run at review. Supersedes the
card's counts, taken without the `mode = inherit_parent` filter. Queries are in the
indicator design. Live children of an archived parent: **56**, of which **55** are
`mode = new_workspace` and start fine and **1** is `inherit_parent` and orphaned
(`b7c01447`). Tasks carrying any `metadata.workspace.orphaned` key: **0**.

Three facts constrain the requirements. **Deriving the boolean from the parent's
`archived_at` is rejected on this evidence**: 55 of 56 are `new_workspace` and start
fine, so it would badge 55 healthy cards. **A marker-only reader renders nothing
today** (no task carries the marker), hence REQ-003. And **the one orphaned task is
invisible to every existing signal at once** (`workspace_status` `active`,
`blocked_reason` `""`), so the marker is the only affordance left to carry it.

## Requirements

### REQ-TASKS-ORPHANED-WORKSPACE-001: Derived orphaned-workspace field on the task contract

**Intent:** Publish the orphaned state as one authoritative boolean, so no
consumer re-derives the producer's predicate.

**User story:** As an operator triaging a board, I want the backend to tell me which
tasks cannot start, so I need not open a card to find out.

#### Acceptance criteria

- **AC-TASKS-ORPHANED-WORKSPACE-001.1:** When a task is marked and its
  `workspace.mode` is `inherit_parent`, the system shall report
  `workspace_orphaned: true` on that task's HTTP DTO.
- **AC-TASKS-ORPHANED-WORKSPACE-001.2:** When a task's metadata does not contain
  `workspace.orphaned`, or contains it with any value other than boolean `true`
  (`false`, `null`, a string, a number), the system shall report
  `workspace_orphaned` as false and omit it from the serialized HTTP DTO
  (`omitempty`); AC-001.4 and AC-001.5 govern the boot payload and events, which
  always carry the key. Key presence alone shall not suffice, unlike `interrupted`
  and `auto_start_failed`.
- **AC-TASKS-ORPHANED-WORKSPACE-001.3:** When a task's metadata contains
  a `workspace` value that is not a JSON object, the system shall report
  `workspace_orphaned` as false and shall not error.
- **AC-TASKS-ORPHANED-WORKSPACE-001.4:** When the backend serves the boot payload,
  it shall include the derived value as `workspaceOrphaned` on every kanban task.
- **AC-TASKS-ORPHANED-WORKSPACE-001.5:** When the backend publishes
  `task.updated`, `task.created`, or `task.state_changed`, the system shall include
  `workspace_orphaned` as an explicit `true` or `false`, never omitting the key, so
  that a clear reaches already-open clients.
- **AC-TASKS-ORPHANED-WORKSPACE-001.6:** When a client receives a task event
  payload omitting `workspace_orphaned`, the system shall preserve the previously
  known value rather than clearing it.
- **AC-TASKS-ORPHANED-WORKSPACE-001.7:** The system shall derive this field at
  serialization time from task metadata and shall not add a database column, task
  state, workflow step, HTTP route, WebSocket action, or event type.
- **AC-TASKS-ORPHANED-WORKSPACE-001.8:** The system shall leave `blocked` and
  `blocked_reason` unchanged. A task that is both orphaned and blocked shall keep
  its dependency chip; the marker replaces the state icon only.
- **AC-TASKS-ORPHANED-WORKSPACE-001.9:** The system shall report
  `workspace_orphaned: true` only when `workspace.mode` is also `inherit_parent`.
  When `mode` has been changed off `inherit_parent` it shall report false even while
  the four marker keys remain stored: every path that changes it (the delete path's
  normalize, a re-parent, a detach) leaves the task startable, and none is reachable
  by AC-003.9, whose retraction needs a present unarchived parent. Stale keys are
  inert, not a second state. The conjunct also makes AC-003.1's "shall agree exactly"
  true: the repair's selection already requires `mode`.

### REQ-TASKS-ORPHANED-WORKSPACE-002: Board marker

**Intent:** Render the state where an operator chooses what to start next.

**User story:** As an operator scanning Backlog, I want an unstartable card to
look different, so I do not pick it.

#### Acceptance criteria

- **AC-TASKS-ORPHANED-WORKSPACE-002.1:** When a task reports
  `workspace_orphaned: true` and no higher-precedence affordance applies, **each** of
  the two board surfaces shall render a marker icon that is a distinct component from
  both the interrupted marker (alert circle) and the auto-start-failed marker (alert
  triangle) — including when the task has no session, no foreground activity and no
  pending prompt, which is the whole measured population. The surfaces gate their
  icons independently, so satisfying one does not satisfy the other.
- **AC-TASKS-ORPHANED-WORKSPACE-002.2:** The marker shall carry a localized
  accessible label and tooltip and shall be keyboard-focusable, matching the
  existing two markers.
- **AC-TASKS-ORPHANED-WORKSPACE-002.3:** When a task's state is `COMPLETED`,
  `FAILED`, or `CANCELLED`, terminal state shall suppress all three markers and the
  system shall render that terminal state's existing icon. It suppresses markers
  **only**: it does not outrank the pending-permission, pending-clarification,
  activity, or waiting-for-input affordances, which still win as today.
- **AC-TASKS-ORPHANED-WORKSPACE-002.4:** When a task has a pending permission request,
  a pending clarification, `generating`/`background` activity, or is waiting for
  input, the system shall render those existing affordances and not the orphaned
  marker. It replaces the coarse task-state icon only.
- **AC-TASKS-ORPHANED-WORKSPACE-002.5:** When a task reports more than
  one of `interrupted`, `auto_start_failed`, and `workspace_orphaned`, the system
  shall render exactly one marker, by the fixed order `interrupted`,
  `auto_start_failed`, `workspace_orphaned`.
- **AC-TASKS-ORPHANED-WORKSPACE-002.6:** The marker's copy shall exist in `en`,
  `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`, with no Unicode em dash in any of them.

### REQ-TASKS-ORPHANED-WORKSPACE-003: Repair of the unmarked historical population

**Intent:** Make the feature true of tasks already orphaned; without it the contract
above is correct and renders nothing on any install.

**User story:** As an operator whose board predates the producer, I want stranded
tasks to show the marker, so the feature applies to my board.

#### Acceptance criteria

- **AC-TASKS-ORPHANED-WORKSPACE-003.1:** On startup, the system shall stamp the
  marker on every live, non-archived task that satisfies the producer's predicate
  (non-ephemeral, non-automation-origin, `metadata.workspace.mode = inherit_parent`,
  parent archived, no `task_environments` row of its own) and is not already
  **marked** per Terminology.
  A task whose `workspace.orphaned` is present but not boolean `true` is unmarked
  and shall be stamped, overwriting that value. The repair's selection predicate and
  AC-001.2's derivation shall agree exactly.
- **AC-TASKS-ORPHANED-WORKSPACE-003.2:** The repair shall write the same four
  keys, with the same names and value shapes, that the producer writes, and shall
  set `orphaned_parent_id` to the archived parent's ID. No second marker shape and
  no "backfilled" flag: orphaned by repair and orphaned by archive are one state.
- **AC-TASKS-ORPHANED-WORKSPACE-003.3:** The repair shall be idempotent: run
  against a database where every qualifying task is already marked and no marker is
  stale per AC-003.9, it shall make no write and no event.
- **AC-TASKS-ORPHANED-WORKSPACE-003.4:** The repair shall not clear a marker
  merely because the task no longer satisfies the orphan predicate. Retraction is
  owned by the unarchive path (REQ-005), which knows *which* parent's claim it
  retracts; AC-003.9 is the single, narrower exception.
- **AC-TASKS-ORPHANED-WORKSPACE-003.5:** When the repair stamps or clears a task,
  the system shall publish `task.updated` so an open client converges without a
  reload.
- **AC-TASKS-ORPHANED-WORKSPACE-003.6:** When the repair fails for one task, the
  system shall log a warning naming it, continue with the rest, and not abort
  startup.
- **AC-TASKS-ORPHANED-WORKSPACE-003.7:** The repair shall cover born-stranded tasks,
  which archive-time detection cannot reach.
- **AC-TASKS-ORPHANED-WORKSPACE-003.8:** The repair shall run on every startup,
  not once. If its `task.updated` publish is dropped because the WS gateway is not
  yet accepting clients, the marker shall still be durably stamped, so the next boot
  payload carries it.
- **AC-TASKS-ORPHANED-WORKSPACE-003.9:** The repair shall clear a marker whose
  `orphaned_parent_id` names a task that exists and is **not** archived: a provably
  false claim nothing else revisits. It shall leave in place a marker whose named
  parent is still archived, and one whose named parent row is absent. This is the
  recovery owner for a failed clear.
- **AC-TASKS-ORPHANED-WORKSPACE-003.9a:** The clearing pass shall consider tasks that
  are themselves archived, unlike the stamping pass, which AC-003.1 confines to live
  tasks. An archived task's marker has no other revisit path, so excluding them would
  make a failed clear permanent, contradicting AC-003.9.
- **AC-TASKS-ORPHANED-WORKSPACE-003.10:** The repair shall re-check the predicate
  at write time and skip, without recording a failure, any task that no longer
  satisfies it — parent unarchived, own `task_environments` row acquired, archived,
  or **reparented** after selection. A selection list is stale by construction. The
  clearing pass shall likewise skip a task whose named parent was re-archived after
  selection, so a marker that became valid again is not torn off.
- **AC-TASKS-ORPHANED-WORKSPACE-003.12:** When rows exist whose `metadata` is
  non-empty and unparseable, the repair shall log exactly one warning carrying their
  count, never one per row. When a post-write re-read fails, it shall log a warning
  naming the task and continue the pass.
- **AC-TASKS-ORPHANED-WORKSPACE-003.11:** When the repair's own selection query
  fails, the system shall log a warning, skip the pass for that boot, and continue
  startup. It shall not abort startup nor retry within the same boot.

### REQ-TASKS-ORPHANED-WORKSPACE-005: Marker lifecycle integrity

**Intent:** Keep the rendered state honest across the archive/unarchive cycle,
including under concurrency.

**User story:** As an operator who unarchived a parent, I want the marker on its
subtasks to disappear, so the board stops calling them broken.

#### Acceptance criteria

- **AC-TASKS-ORPHANED-WORKSPACE-005.1:** When a parent is unarchived and the
  producer clears a child's marker, the system shall publish an event carrying
  `workspace_orphaned: false`, and board surfaces shall drop the marker live.
- **AC-TASKS-ORPHANED-WORKSPACE-005.2:** When an archive and an unarchive mutate the
  same task's `metadata.workspace` map concurrently, the system shall not lose the
  `workspace.mode` value, and shall not leave a marker whose `orphaned_parent_id`
  names a task that is present and not archived. Scoped to that interleaving: a marker
  whose named parent row is **absent** is left in place by AC-003.9, which also owns
  one surviving a crashed clear. The fix shall apply to **both** mark implementations,
  not one; the design names them.
- **AC-TASKS-ORPHANED-WORKSPACE-005.3:** When two callers mark the same task
  concurrently, the resulting marker shall be well formed and name one of the two
  parents, never mixing keys from two writers. The write shall be guarded so a
  losing writer leaves the stored marker untouched rather than partially
  overwriting it; four independently written leaf keys cannot satisfy this.
- **AC-TASKS-ORPHANED-WORKSPACE-005.4:** When a marker's `orphaned_parent_id` does
  not match the parent being unarchived, the system shall leave it in place,
  preserving producer behavior for a task orphaned by a different ancestor.
- **AC-TASKS-ORPHANED-WORKSPACE-005.5:** When a marker write loses its guard, the
  system shall treat it as a no-op success: no retry loop, no error on the enclosing
  archive or unarchive, and **no task event** — one would broadcast a marker that was
  never stored, which AC-001.6 then pins client-side. Scoped to the marker writers;
  the design gives the delete path's stricter rule.
- **AC-TASKS-ORPHANED-WORKSPACE-005.6:** When the delete path normalizes a marked
  child's `mode` from `inherit_parent` to `shared_group`, the system shall clear the
  four marker keys in the same write. AC-001.9 already stops that child rendering a
  marker; this keeps the row itself honest, since its named parent is being deleted
  and AC-003.9 would leave the keys standing forever.

## Ordering, idempotency, concurrency, and boundary behavior

- **Marker precedence** is fixed and total: pending permission > pending
  clarification > foreground activity > waiting-for-input > terminal state (which
  suppresses the three markers) > `interrupted` > `auto_start_failed` >
  `workspace_orphaned` > coarse task state. One entry appended to today's chain;
  terminal state gates the markers rather than heading it. Ties are impossible: a
  total order over named fields, not a sort. Each board surface applies it
  independently, so both must be changed (AC-002.1).
- **Repair-pass ordering** is unconstrained; each write touches one row. For
  reproducible logs, iterate `tasks.created_at` then `tasks.id` ascending.
- **Idempotency:** deriving the field is a pure read; the repair is idempotent
  (AC-003.3). Re-marking refreshes `orphaned_at`, matching the producer.
- **Concurrency:** AC-005.2 to AC-005.6. The contended unit is one task's whole
  `$.workspace` map. Its **guarded** writers are archive (two implementations),
  unarchive, the repair, and the delete path's normalize-and-clear. Two more writers
  reach that map and are deliberately **not** guarded, the generic metadata PATCH and
  the detach query; the design names both, and AC-001.9 rather than a CAS is what
  keeps their output honest. A losing writer is a no-op, never a retry loop.
- **Nil / empty:** AC-001.2, AC-001.3 and AC-001.9 (nil `metadata`, absent or
  non-object `workspace`, `orphaned` not boolean `true`, `mode` not `inherit_parent`).
  An absent, empty, non-string or unresolvable `orphaned_parent_id` never changes the
  derived boolean; it only narrows what AC-003.9 can retract. An empty board yields no
  markers and no repair writes.
- **Errors:** AC-003.6 (one task's write), AC-003.11 (the selection query), AC-003.9
  (a producer clear that failed). A failed producer *mark* stays a warn-and-continue
  upstream: no marker until the next repair pass.
- **Defaults:** `workspace_orphaned` is `false` everywhere — omitted from the DTO,
  `false` on the boot payload, explicit `false` on events.
- **Boundary:** a task whose parent is archived *and* which has its own
  `task_environments` row is not orphaned: the producer's carve-out, which the repair
  honors.

## Out of scope

- **Changing the producer's predicate — who gets marked — or its call sites.**
  PR #3235 is merged and frozen input here; a wrong predicate is a separate card.
  **In scope, not a contradiction:** the metadata-write primitive those call sites
  use, per REQ-005. That changes *how* a decision is persisted, never *which* tasks
  are marked.
- **A guard against new born-stranded tasks.** `lookupOrCreateParentGroup` rejects an
  archived parent and `CreateTask` rolls back, so `origin/main` creates no new ones.
  The residual (that guard is unreachable with no workspace-group repository wired,
  which production always wires) is a producer concern.
- **Every surface except the two board ones**: sidebar rows, the mobile
  task-switcher drawer, the rich task-list row, the open-task header, and Office
  agent cards and the Office dashboard, which carry their own status presentation.
  `auto_start_failed` reaches none of them either, so each is new plumbing.
- **Subtasks whose parent was deleted rather than archived.** Cascade delete removes
  the child; non-cascade delete normalizes an `inherit_parent` child to
  `shared_group` before reparenting, so the predicate cannot match afterwards. A
  child marked *before* that delete is AC-005.6's business, not this exclusion's.
- **A repair action, and a user-facing dismiss.** No button to re-parent, materialize
  a workspace, or convert to `new_workspace`; unarchiving the parent is the existing
  remedy and already clears the marker. The state is not advisory, so hiding it would
  restore the ambiguity this removes.
- **Changing `TaskContext.workspace_status` / `blocked_reason` semantics.** The former
  is read by the Office prompt builder and neither describes an orphan, so overloading
  either would make an existing reader lie. **Blocking launch on the marker** is out
  too: the launch path already fails closed with
  `DescribeInheritedEnvironmentUnavailable`, and a second gate could disagree.
- **An explanation surface for the marker. Cut at the round-7 spec review; deferred,
  no follow-up task filed.** The marker says a card cannot start, not which archived
  parent took the workspace. The cut requirement put that in a banner on the task
  detail context panel; it was cut because that panel cannot reach this card's
  audience. `TaskDetailContextPanel` is mounted only by `OfficeSimplePane`, which only
  `/office/tasks/[id]` renders: `/tasks/[id]` never mounts it and kanban-origin tasks
  have no Office row, so the banner is unreachable from the surfaces REQ-002 marks. A
  follow-up must pick the surface FIRST, then re-derive the contract. The indicator
  design's `## Deferred: explanation surface` keeps the surface options, the
  same-workspace gate, and the convergence bug a follow-up inherits.

## Related

- [Interrupted Task Indicator](interrupted-task-indicator.md) — the marker shape, DTO
  derivation, and icon-precedence rules this capability extends.
- `84c83c3d5` (#3235) — the producer of the marker this reads.
