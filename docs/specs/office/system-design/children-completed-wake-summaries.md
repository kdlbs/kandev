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
That document is a sibling capability that may not have merged yet, so the
constraint is restated here in full rather than delegated: **four producers can
queue a children-completed run, at most one survives per completion wave, no
producer can observe which one won, and the surviving run must therefore deliver
a wake equivalent to the one any other producer would have delivered.** Nothing
below depends on being able to read that document.

The four producers, named here so this design stands on its own:

| Producer | Queues through | Assembles child summaries today |
| --- | --- | --- |
| `office/scheduler/reactivity.go` `cascadeChildrenCompleted` | `scheduler.RunContext` | No — that struct has no child-list field |
| `office/service/event_subscribers.go` `queueChildrenCompletedRun` | workflow engine trigger | Yes, from a database read, then discarded |
| `office/service/scheduler_wake_reconciler.go` `ParentWakeReconciler.buildPayload` | workflow engine trigger | Yes, from a database read, then discarded |
| `orchestrator/event_handlers_children_completed.go` `childCompletionPayload` | workflow engine trigger | Yes, from rows already in memory |

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

**Every length in this section is counted in Unicode code points, never in
bytes.** This is load-bearing rather than pedantic. SQL `SUBSTR` counts code
points in both SQLite and PostgreSQL, while Go's `len(s)` and `s[:n]` count
bytes, so mixing the two units makes a body SQL never cut trip the byte
threshold — reporting a complete comment as truncated — and lets a slice split a
UTF-8 sequence, putting invalid UTF-8 into a live agent prompt. The product ships
zh-cn, zh-hk, zh-tw and pt-pt, and agent comments routinely contain non-ASCII, so
this is the ordinary case rather than an exotic one. Both the comparison and the
slice operate on runes.

- `identifier` falls back to `?` when empty, as today
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.9).
- **The rendered line contains no line break except its terminator.** Every
  interpolated field — identifier, title, state, comment, URL — has any embedded
  CR or LF replaced with a single space before rendering, so one child is always
  exactly one line (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.1, -001.6). The comment
  gets this from `%q`, which escapes a newline rather than emitting it; the
  title, rendered unquoted, needs it applied explicitly. Scoping the guarantee to
  the comment alone would leave `001.1`'s one-line-per-child promise breakable by
  any title containing a newline.
- The comment segment is present only when the child has a most-recent comment
  **whose body is non-empty** (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.3, .4). The
  schema permits an empty body: `task_comments.body` is declared `TEXT NOT NULL`
  with no default and no non-empty `CHECK`, so `NOT NULL` stops a null and stops
  nothing else — an explicit `''` is storable. An empty body carries no
  information, so such a child renders exactly as a child with no comments does:
  the segment is omitted. Rendering `""` would spend a segment to say nothing.
- **A cap bounds the RENDERED field, marker included.** All three capped fields
  (comment, title, URL) share one rule: a value at or below its cap renders whole
  with no marker; a value above it renders as a leading slice followed by
  ` [truncated]`, and slice + marker never exceeds the cap. The marker is 12 code
  points, so the slice is at most cap − 12. This is stated because "capped at N
  followed by a marker" otherwise reads two ways — N then marker, or N including
  marker — and the two differ by 12 code points on every field, which is the
  difference between the size ceiling in [Persistence](#persistence) being exact
  and being wrong by up to 144 code points per line.
- `truncateComment` keeps its shape but counts runes: a body longer than 500 code
  points renders as its first 485 code points followed by ` [truncated]`, for 497
  in total; a body of 500 or fewer renders whole, with no marker
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5). See
  [Child query contract](#child-query-contract) for why that branch becomes
  reachable at all.
- The title is capped the same way at 200 code points: a longer title renders as
  its first 188 code points followed by ` [truncated]`. The task system does not
  bound title length, so without a cap neither the one-line shape nor the
  section's size ceiling means anything.
- The pull-request segment is present only when the child has at least one link.
  URLs are sorted ascending by URL string, then joined by `, `
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.7, .8, -003.7). At most 10 render; a child
  with more gets ` (+N more)` after the tenth. Sorting before capping is what
  makes *which* URLs appear deterministic rather than a property of whatever
  order the link projection returned. **Each URL is itself capped at 200 code
  points**, under the same rule: a longer URL renders as its first 188 code points
  followed by ` [truncated]`. Neither the stored `pr_url` column nor the `TaskPRLink.URL` field it
  populates bounds length, so without this cap
  AC-OFFICE-WAKE-CHILD-SUMMARIES-001.6 is unsatisfied for this field and the
  section has no size ceiling. Sorting and the 10-URL count both operate on the
  untruncated values, so the cap decides how a URL is displayed and never which
  URLs are selected. A cut URL is not a usable link, which is why the marker is
  required rather than a silent slice.

`ChildSummaryPrompt` gains a `PRLinks []string` field. The lead-in sentence, the
closing instruction and the truncation notice text are unchanged
(AC-OFFICE-WAKE-CHILD-SUMMARIES-001.11). The section heading is **not**: it
changes from `"\nCompleted children:\n"` to exactly `"\nChild tasks:\n"`,
because AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1a deliberately lists a child that is
no longer terminal, and a heading asserting completion would contradict the
`[state]` on that child's own line (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.12). The
literal is named here rather than left as "a neutral label" because
AC-OFFICE-WAKE-CHILD-SUMMARIES-003.8 requires byte-identical sections: the heading
is inside a byte-identity contract, so a test needs an oracle for it and a builder
must not have to invent the copy. The lead-in stays as it is because it describes
why the wake fired — every child did reach a terminal state — whereas the heading
labels a list describing the present.

## Child query contract

`Repository.GetChildSummaries` is the single read behind the section. After this
change it has exactly one caller — the prompt path — because
[Producer changes](#producer-changes) removes the other two. Five changes to it:

1. **It becomes one statement, not two.** Today it issues an un-transacted
   `COUNT(*)` and then a separate `SELECT … LIMIT`. Two statements are two
   snapshots, so a child archived between them leaves a stale total above the cap
   while the capped select already covers every live child — precisely the state
   AC-OFFICE-WAKE-CHILD-SUMMARIES-003.6 says shall never occur. The count moves
   into the select list as a scalar subquery over the same predicate, so every
   returned row carries the same live-child count and count and rows come from one
   statement's snapshot. When the statement returns no rows the
   section is empty and no count is read — which is also the correct outcome when
   the parent-existence guard below suppresses rows that do exist. A plain scalar subquery is used
   rather than `COUNT(*) OVER ()` so that no window-function support is assumed
   (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.6, -003.9, -004.8).
2. **Archived children are excluded**, in both the count subquery and the row
   predicate (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1, -003.6). The predicate is
   `archived_at IS NULL`, matching `GetChildSetKey`. No state predicate is added:
   the query already returns every child regardless of state, which is what
   AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1a requires.
3. **The statement guards on the parent existing.** `tasks.parent_id` is
   `TEXT DEFAULT ''` with no foreign key and no cascade anywhere in the schema, so
   a child row can outlive its parent and `WHERE t.parent_id = ?` alone would
   list children for a task that no longer exists. An
   `EXISTS (SELECT 1 FROM tasks p WHERE p.id = ?)` guard in the same statement
   makes a deleted parent yield no rows and a zero count, which is what
   AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3 requires, and costs no extra round trip.
   The guard tests existence only, not archival: an archived parent that is being
   woken anyway still gets its list.
4. **Ordering gains a tiebreak**: `ORDER BY t.created_at ASC, t.id ASC`.
   `created_at` alone is not unique, so today's order is whatever the engine
   returns for a tie, which also makes the 20-row cap non-deterministic
   (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.2, -003.4, -003.8). `tasks.id` is unique,
   so no third column is needed. This matches the existing convention in
   `RunnerProjection`, which already tiebreaks `position ASC, id ASC`.
5. **The last-comment subquery gains a tiebreak and one extra code point**:
   `ORDER BY c.created_at DESC, c.id DESC`, and
   `SUBSTR(c.body, 1, maxCommentChars + 1)`. Today the SQL slices to exactly 500
   while `truncateComment` fires only above 500, so the truncation marker is
   unreachable and a cut comment is presented as if complete. Reading one code
   point more than the display limit makes the branch fire for exactly the bodies
   that were cut — which holds only because both sides now count code points; see
   [Rendered line contract](#rendered-line-contract)
   (AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5, -003.3).

Everything the statement uses — `SUBSTR`, `COUNT`, `EXISTS`, `IS NULL`, `LIMIT`,
correlated subqueries — is common to SQLite and PostgreSQL, and it performs no
JSON extraction and no window function, so no dialect branch is introduced
(AC-OFFICE-WAKE-CHILD-SUMMARIES-003.9).

**This section's incremental read cost** is fixed at **two** database round
trips: this one statement, and one `ListTaskPRsByTaskIDs` call for the returned
ids. Two is what the child list section adds, not the total for an assembled
prompt — `buildPromptContext` already reads for other sections before it reaches
the child enricher, so a test asserting an absolute query count for the whole
assembly is testing the wrong thing. What AC-OFFICE-WAKE-CHILD-SUMMARIES-004.8
constrains is growth: neither read grows with the number of live direct children,
so the observable is an equal query count for a parent with 3 children and one
with 30. Both run only for the two children-completed reasons
(AC-OFFICE-WAKE-CHILD-SUMMARIES-002.6, -002.7).

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

`SchedulerService.cascadeChildrenCompleted` (`office/scheduler/reactivity.go`),
the fourth producer, queues a bare `scheduler.RunContext` that has no child-list
field and never assembled summaries to discard. It is unchanged and needs no
change. It is named here so a builder does not have to rediscover it to establish
that AC-OFFICE-WAKE-CHILD-SUMMARIES-002.4 and -002.5 already hold for it.

`engine.ChildSummary` and the payload field itself stay. Deleting a field from a
shared workflow-engine type is a separate contract change with a different owner,
recorded under the requirement's `## Out of scope`.

One deliberate behaviour change follows from the reconciler edit. Today a
`GetChildSummaries` failure inside `buildPayload` aborts that tick's dispatch;
after the change there is no such read, so the reconciler will dispatch where it
previously bailed. This strictly increases delivery of a wake that was already
judged due, which is what AC-OFFICE-WAKE-CHILD-SUMMARIES-002.9 requires. The
identity and readiness guards that decide *whether* to dispatch —
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
- **Parent gone, or no parent id on the run.** `buildPromptContext` calls the
  enricher only when it parsed a non-empty `task_id`
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3a); the existing call site is gated on the
  run reason but not yet on the id, so that gate is added. A parent that no longer
  exists is caught by the statement's `EXISTS` guard, not by an absence of child
  rows — orphaned children would supply rows, because nothing in the schema
  deletes them with their parent (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3). Both
  cases omit the section exactly as for a parent with no live children.
- **Retry.** The prompt is reassembled from scratch on each launch, so the
  section is re-derived rather than reused
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.6).
- **Concurrent child writes.** Reads are not serialised against child writes, and
  **two reads stand behind one line**: the child statement supplies identifier,
  title, state and comment from a single snapshot, and `ListTaskPRsByTaskIDs`
  supplies the URLs from another. Within each read a child reports either its
  pre-write or its post-write values, and assembly cannot fail; across the two, a
  line may pair pre-write task fields with post-write pull-request links
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-004.7). The section carries no atomicity
  guarantee across children and none across those two reads. Making it atomic
  would mean holding a transaction open across a port call, for a briefing that is
  advisory by construction.

## Persistence

No schema change and no migration. `runs.payload` gains no field; the run row is
unchanged. `runs.assembled_prompt` grows by the rendered section, bounded by 20
lines whose length is bounded in turn by the per-field caps in
[Rendered line contract](#rendered-line-contract): 200 code points of title, 500
of comment, and at most 10 pull-request URLs of at most 200 code points each —
each of those figures bounding the rendered field with its truncation marker
included. That puts the worst case a little under 60 KB and the ordinary case in
the low kilobytes, since real pull-request URLs run well under 100 characters and
most children have one. Those caps are why this is a bound rather than an estimate —
the task system limits title length, link count and URL length nowhere, so without
them the section would have no ceiling at all.

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
assembly, carrying **the parent task id only**. It does not carry the run id, and
must not be specified to: the enricher runs inside `buildPromptContext`, whereas
`assembleAgentPrompt` assigns `pc.RunID` only after that function returns, so no
run id is in scope at the failure site. Widening the enricher's signature to reach
one would be plumbing bought for a single warn line, when the parent task id
already identifies the failure and the run is correlatable from the surrounding
launch logs. [Failure and recovery](#failure-and-recovery) states the same field.

No new metric — the section is visible in `runs.assembled_prompt` on every run it
applies to, which is directly inspectable in the Office run detail prompt panel,
so a counter would add no evidence the prompt itself does not already carry.
