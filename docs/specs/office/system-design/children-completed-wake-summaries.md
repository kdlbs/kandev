---
status: draft
system: office
requirements:
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-001
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-002
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-003
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-004
---

# Children-Completed Wake Summaries System Design

## Purpose and boundaries

Office owns the assembly of the wake prompt a scheduler-launched agent receives.
This design covers the child list section of the `task_children_completed` wake:
where its data comes from, when it is read, and how it renders.

Adjacent contracts this design reads but does not own:

- The task system's parent/child relation, `tasks.state`, `tasks.archived_at`,
  `tasks.created_at`, and `task_comments`.
- The task-PR link projection reached through Office's `TaskPRLister` port.
- The workflow engine's `on_children_completed` trigger and its
  `engine.OnChildrenCompletedPayload` type. This design stops writing part of
  that payload but does not change its shape.

The producer-equivalence constraint from
[parent wake wave identity](parent-wake-wave-identity.md) — its
AC-OFFICE-WAKE-WAVE-IDENTITY-002.10 and its "Wake equivalence between producers"
section — is the governing constraint on this design, not a nearby concern.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-WAKE-CHILD-SUMMARIES-001` | [Rendered line contract](#rendered-line-contract) |
| `REQ-OFFICE-WAKE-CHILD-SUMMARIES-002` | [Where the data comes from](#where-the-data-comes-from), [Producer changes](#producer-changes) |
| `REQ-OFFICE-WAKE-CHILD-SUMMARIES-003` | [Child query contract](#child-query-contract) |
| `REQ-OFFICE-WAKE-CHILD-SUMMARIES-004` | [Failure and recovery](#failure-and-recovery) |

## Where the data comes from

The child list is derived at prompt assembly time from the parent's current
children. It is not carried on the run.

`SchedulerIntegration.buildPromptContext` already runs with a `context.Context`
and the parent task id parsed from the run payload, and already performs
claim-time reads for every other enriched section — handoff context, builder
comments, comment context. `enrichChildrenContext` is the one enricher that
takes no context and performs no read; it parses a `children` key that no
producer writes. This design makes it the same shape as its siblings: it takes
`ctx` and the parent task id, reads, and populates `PromptContext.ChildSummaries`
and `PromptContext.ChildSummariesTruncated`. `buildChildrenCompletedPrompt` and
`writeChildSummaryLine` keep their existing structure.

That choice is what satisfies AC-OFFICE-WAKE-CHILD-SUMMARIES-002.1
structurally. Because the section is produced once, after the producers have
already collapsed onto a single run, there is no per-producer path that could
render differently, and no fifth producer added later could regress it without
changing this code.

### Why not carry the summaries on the run payload

The payload route was the obvious reading of the existing reader, and it was
rejected for four independent reasons. Recording them here so a later round does
not re-derive them:

1. **It needs two carriers, not one.** The Office cascade producer queues through
   `scheduler.RunContext`, which is JSON-marshalled straight into the run row and
   has no child-list field. The other three go through the workflow engine, where
   `queueRunPayload` merges only comment fields and the workflow-authored action
   payload; the shipped `office-default` `on_children_completed` action declares
   no payload, and the trigger's typed payload never reaches the run row. Filling
   `children` from every producer means widening `RunContext` *and* teaching the
   engine to project a trigger payload into a queued run.
2. **The engine's type cannot carry what the prompt reports.**
   `engine.ChildSummary` has `TaskID`, `Status`, `Summary`, `PRLinks`. The
   rendered line reports identifier and title, which that type does not have. The
   payload route therefore also widens a workflow-engine type shared with other
   systems.
3. **A payload is a queue-time snapshot.** A run can be claimed long after it is
   queued. Reporting a child's state and last comment as they were at queue time
   contradicts AC-OFFICE-WAKE-CHILD-SUMMARIES-004.5, and does so in the direction
   that matters: the child's concluding comment is frequently written *after* the
   state change that queued the wake.
4. **Coalescing decides the content by arrival order.**
   `Repository.CoalesceRun` merges by `UPDATE runs SET coalesced_count =
   coalesced_count + 1, payload = ?`, overwriting the surviving row's payload with
   the incoming one; `task_children_completed` is not in
   `isTaskScopedCoalescingReason`. Under payload carriage the surviving run's
   briefing is whichever snapshot arrived last, which is the race outcome
   AC-OFFICE-WAKE-CHILD-SUMMARIES-002.1 exists to prevent.

One thing the payload route is *not* blocked by, measured rather than assumed:
`ParseRunPayload` unmarshals into `map[string]string`, and a nested array under
`children` does not break it. Go's decoder records the type error and continues,
so sibling string keys still populate and `task_id` still resolves; the nested
key lands as `""`. The payload route was rejected on the four points above, not
on a parsing failure.

## Rendered line contract

One line per child, from `writeChildSummaryLine`:

```text
- {identifier} ({title}) [{state}] — {"comment"} — {pr urls}
```

- `identifier` falls back to `?` when empty, as today
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.9).
- The comment segment is present only when the child has a comment
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.3, .4). It is rendered with `%q`, which
  escapes an embedded newline rather than emitting it, so a multi-line comment
  cannot split the line (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.6). Title is
  rendered unquoted, as today; this design does not change that, and the
  single-line guarantee is scoped to the comment body only.
- `truncateComment` keeps its existing rule — bodies longer than 500 characters
  render as the first 485 followed by ` [truncated]`. See
  [Child query contract](#child-query-contract) for why that branch becomes
  reachable (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5).
- The pull-request segment is present only when the child has at least one link,
  and lists the URLs joined by `, ` in ascending URL order
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.7, .8, -003.7).

`ChildSummaryPrompt` gains a `PRLinks []string` field. The section heading, the
truncation notice text, the lead-in sentence, and the closing instruction are
unchanged (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.11).

## Child query contract

`Repository.GetChildSummaries` is the single read behind the section. After this
change it has exactly one caller — the prompt path — because
[Producer changes](#producer-changes) removes the other two. Four changes to it:

1. **Archived children are excluded**, in both the count and the select
   (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1, -003.6). The predicate is
   `archived_at IS NULL`, matching `GetChildSetKey`.
2. **Ordering gains a tiebreak**: `ORDER BY t.created_at ASC, t.id ASC`.
   `created_at` alone is not unique, so today's order is whatever the engine
   returns for a tie, which also makes the 20-row cap non-deterministic
   (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.2, -003.4, -003.8). `tasks.id` is unique,
   so no third column is needed. This matches the existing convention in
   `RunnerProjection`, which already tiebreaks `position ASC, id ASC`.
3. **The last-comment subquery gains a tiebreak**: `ORDER BY c.created_at DESC,
   c.id DESC` (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.3).
4. **The comment slice takes one character more than the display limit**:
   `SUBSTR(c.body, 1, maxCommentChars + 1)`. Today the SQL slices to exactly 500
   and `truncateComment` only fires above 500, so the truncation marker is
   unreachable and a cut comment is presented as if complete. Reading 501
   characters makes the existing branch fire for exactly the bodies that were
   cut (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5).

Everything the query uses — `SUBSTR`, `COUNT`, `IS NULL`, `LIMIT`, correlated
subquery — is common to SQLite and PostgreSQL, and the query performs no JSON
extraction, so no dialect branch is introduced
(AC-OFFICE-WAKE-CHILD-SUMMARIES-003.9).

Read cost per assembled prompt is fixed at one child query plus one
`ListTaskPRsByTaskIDs` call for the returned ids, independent of child count
(AC-OFFICE-WAKE-CHILD-SUMMARIES-004.8). Both run only for the two
children-completed reasons (AC-OFFICE-WAKE-CHILD-SUMMARIES-002.6, -002.7).

## Producer changes

`engine.OnChildrenCompletedPayload.ChildSummaries` is written by three producers
and read by nobody: it is not consulted by the engine, and workflow conditions
cannot reach a trigger payload generically — the engine passes `Payload any` and
type-asserts it only for `OnCommentPayload`. Two of those three writes cost
database reads that are discarded, which
AC-OFFICE-WAKE-CHILD-SUMMARIES-002.5 removes:

- `Service.queueChildrenCompletedRun` (`office/service/event_subscribers.go`)
  stops calling `GetChildSummaries` and `lookupChildPRLinks`, and dispatches
  the trigger with no child summaries. Its `GetChildSetKey` call and its
  operation id are untouched.
- `ParentWakeReconciler.buildPayload`
  (`office/service/scheduler_wake_reconciler.go`) does the same and becomes
  infallible, so its error arm disappears.
- The orchestrator's `childCompletionPayload`
  (`orchestrator/event_handlers_children_completed.go`) builds its summaries
  from rows already in memory for the readiness check, costing no extra read. It
  is left alone; touching it would be churn with no measurable effect.

`engine.ChildSummary` and the payload field itself stay. Deleting a field from a
shared workflow-engine type is a separate contract change with a different owner,
recorded under the requirement's `## Out of scope`.

One deliberate behaviour change follows from the reconciler edit. Today a
`GetChildSummaries` failure inside `buildPayload` aborts that tick's dispatch;
after the change there is no such read, so the reconciler will dispatch where it
previously bailed. This strictly increases delivery of a wake that was already
judged due. The identity and readiness guards that decide *whether* to dispatch —
`GetChildSetKey`, its revalidation against the candidate's key, and the
transactional receipt — are unchanged, so the guard the parent capability relies
on is not the one being removed.

## Control flow

A producer queues the run, carrying no child data. The scheduler claims the run,
`assembleAgentPrompt` calls `buildPromptContext`, which parses `task_id` from the
payload and — for `task_children_completed` or the legacy `children_completed` —
calls the enricher with `ctx` and that id. The enricher reads the parent's live
direct children, looks up their PR links in one batch, populates
`PromptContext`, and `BuildPrompt` renders. The rendered prompt is persisted to
`runs.assembled_prompt` by `persistPromptArtifacts` and shown unchanged in the
Office run detail prompt panel.

## Failure and recovery

- **Child read fails.** The enricher logs at warn with the parent task id and
  returns, leaving `ChildSummaries` empty. `buildChildrenCompletedPrompt` then
  emits lead-in and closing instruction only. The run launches
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.1, -004.4). Warn rather than debug is
  deliberate: an empty section is exactly the defect this capability closes, and
  it is otherwise indistinguishable from a parent with no children.
- **PR lookup fails or `TaskPRLister` is unwired.** `lookupChildPRLinks` already
  returns an empty map and warns internally; lines render without the PR segment
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.2).
- **Parent gone.** The query returns no rows, so the section is omitted, exactly
  as for a parent with no live children
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3).
- **Retry.** The prompt is reassembled from scratch on each launch, so the
  section is re-derived rather than reused
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.6).
- **Concurrent child writes.** Reads are not serialised against child writes.
  Each row is read once, so a child reports either its pre-write or post-write
  values and assembly cannot fail; the section carries no cross-child atomicity
  guarantee (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.7).

## Persistence

No schema change and no migration. `runs.payload` gains no field; the run row is
unchanged. `runs.assembled_prompt` grows by the rendered section, bounded by 20
lines of at most roughly 600 characters each.

Runs queued before this change and claimed after it render the section normally,
because the section does not depend on anything the producer recorded. No
backfill exists or is needed.

## Security

No new trust boundary. Child titles and comment bodies are already user- and
agent-authored content that this agent can read through the task API under the
same scope; the wake prompt reports it to the parent agent, which is the actor
the children belong to. PR URLs are already surfaced on the task. Nothing here
is rendered into HTML.

## Observability

One structured log is added: a warn on child-read failure during prompt
assembly, carrying the run id and the parent task id. No new metric — the section
is visible in `runs.assembled_prompt` on every run it applies to, which is
directly inspectable in the Office run detail prompt panel, so a counter would
add no evidence the prompt itself does not already carry.
