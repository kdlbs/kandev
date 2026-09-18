---
id: plugins-task-dependency-edge-ends-design
title: Dependency edge ends on the plugin data API
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-TASK-DEPS-003
  - REQ-PLUGINS-TASK-DEPS-006
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Dependency edge ends on the plugin data API System Design

## Purpose and boundaries

This design owns what an edge end is allowed to say: which ends a plugin task
read may describe (`REQ-PLUGINS-TASK-DEPS-003`) and what an end reports when the
response does not contain it (`REQ-PLUGINS-TASK-DEPS-006`). It is separated from
the [task dependency projection](task-dependency-projection.md) because the two
are reviewed against different risks, and because these rules are the part that
must not be traded away when the projection's shape changes.

It does not own the projection's field set, its derivation, its ordering, its
bounding rules, or its event behaviour. Those are the projection design's, and
this design changes none of them: redaction empties two fields on an entry, and
the dangling-edge drop removes an entry whose task no longer exists. Neither
changes the verdict.

Adjacent contracts used but not owned:

- [Task dependency projection](task-dependency-projection.md) — the field set,
  the edge-end ref type, and the rule fixing which responses attach the
  projection.
- [Response bounds](task-dependency-response-bounds.md) — the query bound this
  rule's query-free admission test is written against.
- [Isolated web-app contributions](isolated-web-app-contributions.md) — the
  canvas scope binding this rule evaluates against.
- [Capability approval](capability-approval.md) — the effective-authority
  intersection behind `api_read:tasks`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-TASK-DEPS-003` | [Redaction rule](#redaction-rule) |
| `REQ-PLUGINS-TASK-DEPS-006` | [Edge ends outside the response](#edge-ends-outside-the-response) |

## Redaction rule

The projection adds no capability. It is a field set on the already-approved
`api_read:tasks` resource; adding fields changes neither a plugin manifest nor
its recorded digest, so no re-approval is triggered and the effective-authority
intersection is unchanged.

The one new authority question is the edge end. An end carries another task's
title and state, and a task-scoped canvas is refused that task directly, so an
unredacted ref would hand it resource content the scope denies. Redaction
resolves it:

- The admission test for an end is the same scope predicate that decides direct
  readability, so there is one rule and no second notion of visibility to keep in
  step. It is evaluated **only over data the response has already loaded** -- a
  read per edge end would be a per-task query and would break the bound in
  `AC-PLUGINS-TASK-DEPS-004.1`. An end whose predicate inputs are not in hand is
  treated as not admitted, so the query bound and the scope rule fail in the same
  direction rather than trading against each other.
- An end the scope would not admit keeps `id` and, on a predecessor, `status`.
  `title` and `state` are emptied.
- `status` is retained deliberately. It is the three-value resolution verdict,
  and the aggregate it feeds, `blocked_reason`, is already about the bound task
  and already visible. `state` is dropped because it is the richer raw task
  state, which is a genuine addition about a task the scope denies.
- The entry is emptied rather than dropped, so the edge's existence and the list
  length stay truthful. A canvas can already infer that an edge exists from
  `blocked`, so hiding the entry would make the count wrong without hiding
  anything.
- No flag is added to tell a redacted entry apart, and consumers are documented
  as rendering an empty `title` as an unlabelled node. The design does **not**
  claim the two are indistinguishable: a task that genuinely has an empty title
  still reports a non-empty `state`, and a redacted entry reports neither. The
  flag is omitted for vocabulary, not concealment -- its false value invites
  reading as an authority statement, and what it carries is diagnostic only.

By scope kind, the query-free rule resolves as follows, and admits no other end
(`AC-PLUGINS-TASK-DEPS-003.8`):

| Scope | Admits | Why it is decidable without a read |
| --- | --- | --- |
| Instance | Every end | The predicate admits every task; nothing to check. |
| Workspace | An end whose workspace is known equal to the bound workspace | The ref carries the end's workspace; see below. |
| Task | No end | An end is never the bound task itself. |
| Repository | An end also returned in the response as a directly readable task | The predicate needs the task's repository set, which is a per-task read. |
| Session | An end also returned in the response as a directly readable task | The predicate needs the task's session list, which is a per-task read. |

**Where the end's workspace comes from.** The admission test may issue no read,
so the workspace must arrive with the derivation or not at all. It does: resolving
edge-end detail already loads the full task row for every end in the batch in one
query, and that row carries the workspace id, which the derivation discards today
when it collapses the row into the slim ref. Carrying it costs no extra query, and
is change 4 of the five listed under
[Edge order](task-dependency-projection.md#edge-order).

**Which type carries it, and where it is dropped.** The carrier is the edge-end
ref type itself, at every in-process layer: the shared derivation's ref struct
gains a workspace-id field, and the mirrored SDK ref type declares the same field.
It is dropped at exactly two points, both of them encoders:

1. The ref's `toProto` mapping, which assigns no proto field for it, because the
   ref message declares none.
2. The canvas JSON adapter's field set, which emits `id`, `title`, `state` and
   `status` for an entry and nothing else.

So "never encoded on either wire" means dropped on the way *out* of the SDK type,
not dropped *from* it. The canvas adapter sits downstream of the SDK mapping and
reads the workspace off the SDK ref it already holds, which is what makes
`AC-PLUGINS-TASK-DEPS-003.8`'s workspace row decidable at the one layer that holds
the canvas binding; it then drops the field while building the JSON entry. A
design that dropped the field at the SDK boundary instead would leave the adapter
with no predicate input at all.

Naming the carrier matters because the alternative is not a slower implementation
but a quietly different contract: without it,
`AC-PLUGINS-TASK-DEPS-003.3`'s "predicate inputs unavailable, so not admitted"
becomes the only legal outcome, workspace scope collapses into the repository and
session rule, and every out-of-page end is redacted while every acceptance
criterion still passes as literally written.

Publishing the field instead of dropping it is rejected for the reason redaction
exists: it would hand every canvas the workspace of tasks it cannot read.

The repository and session rows are the load-bearing ones. Their predicates
resolve a task's repositories and sessions with a read *per task*, which the
admission test may not issue, so the only end they can admit for free is one the
response already proved readable by including it. This under-admits: an end the
canvas could fetch directly may still be redacted. That is the correct direction
for a scope rule to err, and the cost is a title, not an edge -- the entry, the
count, and the verdict are unaffected.

The workspace row previously read "redacts nothing, since an edge cannot cross a
workspace". That justification does not hold. The cross-workspace check lives in
the *write* validator, and it does not reject a pair in which either end carries
no workspace, so a stored edge can name an end whose workspace is empty or
different. The validator is therefore not a blanket exemption
(`AC-PLUGINS-TASK-DEPS-003.4`), and workspace scope admits on known equality
rather than on assumption.

The gRPC surface redacts nothing (`AC-PLUGINS-TASK-DEPS-003.5`). It has no canvas
scope to redact against and no narrower binding to enforce. The reason is *not* a
workspace filter: `Tasks().Get` applies none at all -- it resolves a task by id
and returns it -- and the write and preview RPCs reach tasks by id too, so only
`Tasks().List` narrows by workspace. The correct ground is disclosure parity. A
caller holding `api_read:tasks` can already read any of those tasks directly by
id, so an edge end carrying that task's title and state reveals nothing the
surface withholds; redacting there would cost a title and buy no
confidentiality.

## Edge ends outside the response

A graph built from one page has ends the page does not contain. Three cases, all
of which resolve without a second contract:

- **Archived end.** An archived predecessor is `pending`, never `resolved`,
  because archival is neither success nor failure. The default task list hides
  archived tasks, so such an end is named by the ref but absent from `items`. A
  non-redacted ref carries id, title, and state, which is enough to render the
  node; a consumer needing the whole task reads it by id or lists with
  `include_archived=true`. A read by id is not filtered by archived state, so
  that path works.
- **Out-of-page end.** Same shape as the archived case, for an end that simply
  fell on another page. No special handling.
- **Removed end.** An edge whose predecessor row is gone is dropped from
  `depends_on` and does not contribute to the verdict, so a failed cleanup cannot
  block a dependent forever.

  The dependent direction is not symmetric today, and this is change 2 of the
  four the [projection design](task-dependency-projection.md#edge-order)
  enumerates in the shared derivation. The upstream dangling-edge drop is
  applied to the predecessor list only -- its regression test asserts on that
  list alone -- so an edge whose *dependent* row is gone still yields an entry,
  carrying the derivation's internal `missing` marker as its status. Emptying
  `status` on `blocks` entries at the mapping layer keeps that marker off the
  wire and closes the public vocabulary, but it leaves a ref with a dead id in
  the list, which `AC-PLUGINS-TASK-DEPS-006.4` forbids. The drop must therefore
  be applied to both directions in the shared derivation, and its regression test
  extended to assert on `Blocks` as well.

## Test strategy

- Scope redaction: per scope kind against the table above -- instance admitting
  every end, workspace admitting only a known-equal workspace (including an end
  whose workspace is empty, which is redacted), task scope redacting every end,
  repository and session admitting only an end the same response returns as a
  task. Redaction leaves the verdict, count, and order unchanged, and issues no
  additional read, asserted with the query-count test's counting fake.
- The workspace carrier survives to the adapter: a canvas-surface read on a
  workspace-scoped canvas admits an out-of-page end in the bound workspace and
  redacts one outside it, which fails if either encoder's drop point is moved
  upstream of the adapter.
- Neither wire carries the workspace of an edge end: the proto ref message has no
  such field, and the canvas JSON entry has exactly the four documented keys.
- The gRPC surface redacts nothing: the same task read over gRPC returns every
  end's `title` and `state`, on the single-task read as well as the list read.
- Dangling ends in both directions: an edge whose predecessor row is gone and one
  whose dependent row is gone are both omitted, and no entry carries the internal
  `missing` marker.
- Vocabulary: no entry reports a non-empty status outside the closed set,
  including for an absent end; `blocks` entries report an empty one.
- Archived predecessor reports `pending`, appears in the ref, and is absent from
  a default list.

## Related decisions

- [ADR 0043 — Plugin host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Isolated web-app contributions](isolated-web-app-contributions.md)
- [Task dependencies](../../tasks/system-design/task-dependencies.md)
