---
id: plugins-task-dependency-edge-ends
title: Dependency edge ends on the plugin data API
status: draft
system: plugins
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Dependency edge ends on the plugin data API Requirements

## Overview

The [task dependency projection](task-dependency-projection.md) adds each task's
direct predecessors and dependents to the plugin and canvas task read model. Each
entry in those lists names *another* task, and carries that task's title and
state. Those entries are where the projection stops describing the task the
caller asked for and starts describing one it did not, so they raise two
questions the projection itself does not answer.

**Who may read an end.** On a canvas surface an entry can describe a task the
caller is refused outright: a task-scoped canvas may read exactly one task, yet an
unredacted end would hand it the titles of every task blocking that one.
`REQ-PLUGINS-TASK-DEPS-003` bounds that disclosure.

**What an end says when the response does not contain it.** A graph assembled
from one page names ends that are archived, on another page, or gone from storage
entirely. `REQ-PLUGINS-TASK-DEPS-006` fixes what each of those reports, so a
consumer never renders a stale or invented node.

This document is separated from the projection requirements because the two are
reviewed against different risks: the projection decides what the graph says, and
this decides what each end of it is allowed to say. The projection's criteria
defer here wherever an end's `title`, `state`, or existence is at stake. A third
sibling, [response bounds](task-dependency-response-bounds.md), owns
`REQ-PLUGINS-TASK-DEPS-004`; the query bound that
`AC-PLUGINS-TASK-DEPS-003.3` keeps the admission test inside lives there.

Identities continue the `TASK-DEPS` token, so `REQ-PLUGINS-TASK-DEPS-003` and
`REQ-PLUGINS-TASK-DEPS-006` keep the identities they were reviewed under.

## Terminology

- **Dependency edge:** A directed peer-to-peer relation "this task depends on
  that task", distinct from the parent/child subtask hierarchy. Its two ends are
  the **predecessor** and the **dependent**, listed in `depends_on` and in
  `blocks` respectively; **edge end** means either.
- **Canvas scope:** The scope kind and trusted resource identity bound to a
  canvas instance (instance, workspace, task, repository, or session), which
  already decides which tasks that canvas may read directly.
- **Redaction:** Emptying an edge end's `title` and `state` while keeping the
  entry, its `id`, and (for a predecessor) its `status`.
- **Plugin task read:** As defined in
  [task dependency projection](task-dependency-projection.md) — any surface
  returning the plugin task read model, on either the gRPC or the canvas surface.

## Requirements

### REQ-PLUGINS-TASK-DEPS-003: Scope-bounded edge ends

**Intent:** An edge end must not become a way to read a task the caller is
refused directly. A task-scoped canvas sees that its task is blocked; it must not
learn the titles of the tasks blocking it.

#### Acceptance criteria

- **AC-PLUGINS-TASK-DEPS-003.1:** When a plugin task read on a canvas surface
  returns an edge end the canvas scope would not admit as a directly readable task,
  that entry shall carry only the end's `id` and, for a `depends_on` entry, its
  `status`. This shall hold on the list route, the single-task route, and the task
  object returned by the update route alike.
- **AC-PLUGINS-TASK-DEPS-003.2:** A redacted entry shall report `title` and
  `state` as empty rather than omitting the entry.
- **AC-PLUGINS-TASK-DEPS-003.3:** The admission test for an edge end shall be the
  same scope predicate that decides direct readability, evaluated only over data
  the response has already loaded. It shall issue no additional read per edge end,
  so it cannot conflict with `AC-PLUGINS-TASK-DEPS-004.1`. An end whose predicate
  inputs are unavailable shall be treated as not admitted.
- **AC-PLUGINS-TASK-DEPS-003.4:** The cross-workspace write validator shall not
  be relied on as a blanket exemption from the workspace test in
  `AC-PLUGINS-TASK-DEPS-003.8`, because it does not reject a pair in which either
  end carries no workspace.
- **AC-PLUGINS-TASK-DEPS-003.5:** A gRPC plugin read shall redact no edge end, on
  the grounds of disclosure parity rather than of a workspace filter: that surface
  has no canvas scope, and the caller may already read any such end as a task
  through the single-task read, which applies no workspace narrowing. The list
  read's narrowing shall not be cited, because it does not apply on every gRPC
  task read.
- **AC-PLUGINS-TASK-DEPS-003.6:** Redaction shall not change `blocked`,
  `blocked_reason`, `start_when_unblocked`, the number of entries in either
  list, or their order.
- **AC-PLUGINS-TASK-DEPS-003.7:** No field shall be added to report that an entry
  was redacted, and consumers shall be documented as rendering an empty `title`
  as an unlabelled node. The contract shall not claim a redacted entry is
  indistinguishable from one whose task genuinely has an empty title: it is not,
  because such a task still reports a non-empty `state`.
- **AC-PLUGINS-TASK-DEPS-003.8:** The admission test shall resolve by scope kind as
  follows and admit no other end: instance scope admits every end; workspace scope
  admits an end whose workspace is known equal to the bound workspace; task scope
  admits no end, because an end is never the bound task itself; repository and
  session scope admit an end only when it is also returned in the response as a
  directly readable task.

### REQ-PLUGINS-TASK-DEPS-006: Archived and removed edge ends

**Intent:** A graph assembled from one page has ends the page does not contain.
The consumer must render them without being told a stale story about them.

#### Acceptance criteria

- **AC-PLUGINS-TASK-DEPS-006.1:** An archived predecessor shall appear in
  `depends_on` with `status: pending`, and shall never report `resolved`.
- **AC-PLUGINS-TASK-DEPS-006.2:** An edge end shall appear in its list whether or
  not the same response contains that end as a task, so a default
  `include_archived=false` list still names its archived ends.
- **AC-PLUGINS-TASK-DEPS-006.3:** An entry not redacted under
  `REQ-PLUGINS-TASK-DEPS-003` shall carry enough to render a node without reading
  the end as a task; a consumer needing the full task reads it by id or lists with
  `include_archived=true`.
- **AC-PLUGINS-TASK-DEPS-006.4:** An edge whose end no longer exists as a task
  shall be omitted from `depends_on` and `blocks`, and shall not contribute to
  `blocked` or `blocked_reason`. This differs from the redaction in
  `AC-PLUGINS-TASK-DEPS-003.2`, which keeps an entry for a task that exists but is
  outside the caller's scope.
- **AC-PLUGINS-TASK-DEPS-006.5:** No entry of `depends_on` or `blocks` shall
  report a *non-empty* status value outside the closed vocabulary in
  `AC-PLUGINS-TASK-DEPS-001.6`, including for an end whose task row is absent. The
  derivation's internal `missing` marker shall never reach either list. An empty
  status is required on a `blocks` entry by `AC-PLUGINS-TASK-DEPS-001.7` and is
  not a vocabulary violation there; on a `depends_on` entry it is one, because
  `AC-PLUGINS-TASK-DEPS-001.6` requires exactly one of the three values.

## Out of scope

- **A flag reporting that an entry was redacted.** Excluded by
  `AC-PLUGINS-TASK-DEPS-003.7`: a false value on such a flag invites reading as an
  authority statement, and what it carries is diagnostic only.
- **Admitting an end by reading it.** Excluded by
  `AC-PLUGINS-TASK-DEPS-003.3`, which bounds the admission test to data the
  response already holds so it cannot conflict with the query bound in
  `AC-PLUGINS-TASK-DEPS-004.1`. The cost is accepted under
  `AC-PLUGINS-TASK-DEPS-003.8`: repository and session scope under-admit, and an
  end the canvas could fetch directly may still be redacted.
- **Page-stable redaction.** Under `AC-PLUGINS-TASK-DEPS-003.8` a repository- or
  session-scoped canvas admits an end only when the same response returns it, so
  the same edge end may carry a title at one page size and not another. The rule
  fully determines each response; no criterion requires the outcome to be stable
  across requests.
- **Redaction on the event stream.** The canvas event projection has no per-entry
  scope evaluation, so the dependency projection is kept off events entirely by
  `AC-PLUGINS-TASK-DEPS-005.1` rather than redacted there.
