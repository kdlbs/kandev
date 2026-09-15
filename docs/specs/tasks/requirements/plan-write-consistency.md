---
status: active
system: tasks
created: 2026-08-31
owners:
  - kandev
---

# Task plan write consistency Requirements

## Overview

A task plan write reads the current content and latest revision. It decides if the
write truncates content and if it can coalesce. These reads and the commit are not
serialized, so another writer can change the state before the commit.

This capability defines what a plan write must guarantee about the state it reports
on and the state it commits against. The `tasks` system owns it because the durable
artifact is the plan and its revision history, not the MCP surface that exposes the
warning. The missing-task boundary is owned by `REQ-TASKS-DOCUMENTS-001` and is
unchanged here.

## Terminology

- **Plan write:** any operation that upserts the task plan HEAD row and writes or
  merges a revision. Today: create, update, and revert.
- **Plan delete:** the operation that removes the task plan HEAD row. It is not a
  plan write, because it writes no revision, and it leaves that task's existing
  revisions in place.
- **HEAD:** the single `task_plans` row for a task, holding the current content.
- **Revision:** one `task_plan_revisions` row. Revisions are ordered by
  `revision_number`, which is unique per task (`UNIQUE (task_id,
  revision_number)`) and starts at 1.
- **Latest revision:** the revision with the highest `revision_number` for a task.
- **Coalesce:** merging a write into the latest revision in place, preserving that
  revision's number, author, and creation time, instead of appending. The
  eligibility window defaults to five minutes, from today's configuration.
- **Append:** inserting a new revision at `max(revision_number) + 1`.
- **Truncation:** a write whose content retains less than half the characters of
  the content it replaces, where the replaced content is at least 2000
  characters.
- **Replaced content:** the plan content that a write actually overwrites, that
  is, the HEAD content in effect at the instant the write commits.
- **Agent write path:** the MCP tools `create_task_plan_kandev` and
  `update_task_plan_kandev`.
- **Browser write path:** the WebSocket actions handled by `wsCreateTaskPlan` and
  `wsUpdateTaskPlan`.

## Requirements

### REQ-TASKS-PLAN-WRITE-CONSISTENCY-001: A truncation report describes the write that happened

**Intent:** The truncation warning exists so an agent that has accidentally
destroyed a plan surfaces the loss instead of rewriting from memory. Every claim it
makes must be true of the write that committed, or it sends the agent to a revision
that does not hold the lost content, or reports a drop that never occurred.

**User story:** As an agent that has just overwritten a task plan, I want the
warning's counts and named revision to describe the write I actually performed, so
that I can report the real loss and a human can recover the real content.

#### Acceptance criteria

- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.1:** When a plan write emits a truncation
  warning, the system shall derive the reported prior character count, new
  character count, dropped character count, and dropped percentage from the
  replaced content, measured in Unicode code points rather than bytes.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.2:** When a plan write emits a truncation
  warning that names a revision number, that number shall be the
  `revision_number` of the revision immediately preceding the committed write for
  that task, and that revision shall contain the replaced content.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.3:** When another plan write for the same
  task commits after a write reads plan state and before that write commits, the
  system shall decide truncation for the later-committing write against the
  replaced content rather than against the superseded content it first read.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.4:** When the replaced content retains at
  least half of its characters, or is shorter than 2000 characters, the system
  shall emit no truncation warning and shall not force a new revision on that
  account. Retaining exactly half shall not be treated as truncation.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.5:** When a plan write emits a truncation
  warning, the system shall append a new revision for that write and shall not
  coalesce it.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.6:** When the current plan content cannot
  be read, the system shall append a new revision, emit no truncation warning,
  emit the plan updated event rather than the plan created event, and shall not
  fail the write on that account.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.7:** When truncation has been detected but
  the preceding revision number cannot be established, the system shall append a
  new revision and emit the warning without naming a revision number. The warning
  shall not claim that the pre-write content is preserved or recoverable.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.8:** When a task has no plan, the system
  shall emit no truncation warning for the write that creates it.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-001.9:** When the current plan content cannot be
  read and the request supplies no title or no author, the system shall leave the
  stored title or author unchanged rather than substituting a default. The new
  revision, write result, and update event shall use the stored title and author.

### REQ-TASKS-PLAN-WRITE-CONSISTENCY-002: Revision history stays consistent under concurrent writes

**Intent:** A coalescing write merges into a revision chosen before it committed. If
another write appended in between, the merge lands on a revision that is no longer
the latest: HEAD holds one write's content, the highest-numbered revision holds
another's, and an older revision silently carries newer content. The history stops
being a readable record, and it is the only recovery path the warning points at.

**User story:** As a user recovering a plan from its revision history, I want an
accurate ordered record, so that reading revision N tells me what the plan contained
at revision N.

#### Acceptance criteria

- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.1:** For any two plan writes to the same
  task, the system shall evaluate each write's coalesce decision against the
  state that same write commits against.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.2:** When a plan write coalesces, the
  system shall merge into the revision that is the latest revision for that task
  at the instant the write commits.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.3:** After any plan write completes
  successfully, the task's HEAD content shall equal the content of that task's
  latest revision, while that task exists; deleting the task removes both.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.4:** When a plan write appends, the system
  shall assign `max(revision_number) + 1` for that task, or 1 when the task has no
  revisions, so that revision numbers remain unique, gapless, and strictly
  increasing in commit order.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.5:** When two plan writes for the same task
  are in flight concurrently, the system shall commit them in some serial order
  and shall not guarantee which order. Each write's own reported result shall be
  consistent with the state it committed against.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.6:** When plan writes for different tasks
  are in flight concurrently, the system shall not make either write wait on the
  other's plan write lock. This constrains only the serialization this capability
  introduces. It makes no claim about storage-level parallelism, which the single
  writer connection `internal/db.OpenSQLite` opens already precludes for every task
  alike; that is pre-existing behavior and out of scope here.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.7:** When the latest revision records a
  revert, the system shall append rather than coalesce, regardless of author or
  elapsed time.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.8:** When the configured coalesce window is
  zero or negative, the system shall append every write.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.9:** When a write's author kind or author
  name differs from the latest revision's, or the configured coalesce window has
  elapsed since that revision was last updated, the system shall append rather
  than coalesce.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.10:** When a plan write reads plan state
  after entering the serialized section, that read shall observe every plan write and
  plan delete for the same task committed before it, including one committed through
  a different database connection than the read uses.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.11:** When a task has no plan HEAD at the
  instant a write commits, the system shall append rather than coalesce, even when
  revisions survive from before that HEAD was deleted.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-002.12:** When a plan delete for a task is in
  flight concurrently with a plan write for that task, the system shall order the
  two serially: the delete shall not commit between that write's state read and
  that write's commit. Whichever of the two commits second shall observe the first,
  so an update whose plan was deleted first shall fail as it does today rather than
  recreate the plan.

### REQ-TASKS-PLAN-WRITE-CONSISTENCY-003: Every write path shares the same history guarantees

**Intent:** The browser path is one of the two writers that can corrupt the history,
so it must participate in the same serialization even though it does not need the
warning: a human editing in the browser has a visible diff and a revision list, and
an agent has neither.

**User story:** As a user editing a plan in the browser while an agent works on the
same task, I want both writes to produce a coherent history, so that neither
silently overwrites the other's revision.

#### Acceptance criteria

- **AC-TASKS-PLAN-WRITE-CONSISTENCY-003.1:** When a plan write originates from the
  browser write path, the system shall apply
  `REQ-TASKS-PLAN-WRITE-CONSISTENCY-002` to it in full.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-003.2:** When a plan write originates from the
  browser write path, the system shall emit no truncation warning and shall leave
  that path's response shape unchanged.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-003.3:** When a plan write originates from a
  revert, the system shall apply `REQ-TASKS-PLAN-WRITE-CONSISTENCY-002` to it in
  full, shall append rather than coalesce, and shall emit no truncation warning.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-003.4:** When a plan write originates from the
  agent write path, the system shall evaluate truncation for it.

### REQ-TASKS-PLAN-WRITE-CONSISTENCY-004: The write response contract is unchanged for unaffected writes

**Intent:** The warning was added without changing the plan DTO, so the browser
editor and every non-truncating caller keep the shape they have. Closing the race
must not change that.

**User story:** As a caller of a plan write, I want a write that did not truncate to
return exactly the payload it returns today, so that this change is invisible to
me.

#### Acceptance criteria

- **AC-TASKS-PLAN-WRITE-CONSISTENCY-004.1:** When a plan write emits no truncation
  warning, the system shall return the plan payload with the same fields and
  shape it returns today, adding no truncation-related field.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-004.2:** When a plan write on the agent write
  path emits a truncation warning, the system shall return the existing plan
  payload fields plus the warning text and, when established, the preceding
  revision number.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-004.3:** When a plan write is rejected because
  its task does not exist, the system shall behave as
  `AC-TASKS-DOCUMENTS-001.2` already requires, and shall create no plan data.

### REQ-TASKS-PLAN-WRITE-CONSISTENCY-005: Retries are absorbed and a failed write does not block the next one

**Intent:** Serializing plan writes lets one write block every later write to the
same task, and agents retry. Both need a stated answer: what a repeated write does
to the history, and what a failed or slow write does to the writes behind it.
Neither is answerable from today's behavior.

**User story:** As an agent retrying a plan write, I want the retry to behave
predictably and not strand the task, so that I do not create a misleading history or
hang every later write.

#### Acceptance criteria

- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.1:** Plan writes shall not be deduplicated
  by content. A write repeated with identical content shall be treated as an ordinary
  write under the coalesce rules in `REQ-TASKS-PLAN-WRITE-CONSISTENCY-002`: when the
  task still has a plan HEAD and the write reads it successfully, a repeat by the
  same author within the window merges into the latest revision and adds none, and a
  repeat outside the window or by a different author appends a revision whose content
  equals its predecessor's. When that read fails,
  `AC-TASKS-PLAN-WRITE-CONSISTENCY-001.6` governs; when the HEAD is absent,
  `AC-TASKS-PLAN-WRITE-CONSISTENCY-002.11` does. Both append.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.2:** When a truncating write is repeated
  with identical content after the first attempt has committed, the system shall
  emit no truncation warning for the repeat, because the content it replaces is
  the already-truncated content.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.3:** When a plan write fails at any point
  after it has begun, including a validation failure, a read failure, or a commit
  failure, the system shall leave the task able to accept a subsequent write.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.4:** When a plan write is rejected for a
  missing or empty task identifier, the system shall reject it before entering the
  serialized section, and shall not serialize such requests against each other.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.5:** No single plan write or plan delete
  shall hold more than one plan write lock at a time, so that plan writes cannot
  deadlock against each other. This bounds one operation, not the system.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.6:** Beyond mutual exclusion, the system
  shall not guarantee any acquisition order or fairness between writes waiting on
  the same task.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.7:** When a plan write's caller cancels its
  context while that write is waiting to enter the serialized section, the system
  shall not abandon the wait on that account. The write shall enter the section in
  its turn and then fail when it reaches its write transaction, releasing the
  section for the writes behind it. Cancellation while queued is not honoured, and
  this adds no acquisition-order guarantee beyond
  `AC-TASKS-PLAN-WRITE-CONSISTENCY-005.6`.
- **AC-TASKS-PLAN-WRITE-CONSISTENCY-005.8:** When a plan write's transaction has
  committed and the read the system performs afterwards fails or finds no row, the
  system shall report the write as successful and shall report no plan identity it
  did not read.

## Verification

Concurrency procedures are in the system design's "Test strategy".

- **AC-...-001.3, 002.1, 002.2, 002.3** require a deterministic concurrent-write
  test through the ordinary write paths that forces the interleaving rather than
  racing for it: one write held after its state read, a second write for the same
  task observed to queue rather than commit, then both released and each asserted
  against the state it actually decided on. Neither an unsynchronized pair of
  goroutines nor a second write committed directly into the repository is
  sufficient evidence.
- **AC-...-002.6** requires a write for one task reaching its own commit while a
  write for a different task is held inside the section.
- **AC-...-002.10** requires at least one test built with genuinely separate writer
  and reader pools, as production does through `internal/db.OpenSQLite` and
  `OpenSQLiteReader`. Existing helpers pass one handle as both and cannot observe
  it.
- **AC-...-002.11** requires a plan delete then a same-author create inside the
  window, asserting the create appends rather than merging into a surviving revision.
- **AC-...-002.12** requires the first bullet's forced interleaving with a plan
  delete as the second operation.
- **AC-...-005.7** requires a write cancelled while queued, asserting it errors and
  that a later write to that task succeeds.
- **AC-...-001.4** requires boundary cases at exactly 2000 replaced characters and at
  exactly half retained; **AC-...-001.1** requires a case whose byte length and code
  point length differ.
- **AC-...-005.3** requires a write failing inside the serialized section followed by
  a successful write to the same task; asserting only that the error is returned does
  not cover it.
- **AC-...-005.8** requires the post-write read forced to fail, and separately to
  find no row, each asserting success and no fabricated plan identity.
- **AC-...-005.1** requires both halves: a same-author repeat inside the window
  adding no revision, and a repeat outside it adding one.

## Out of scope

- **Cross-process writers.** These requirements assume a single backend process owns
  the database file, which `internal/db.OpenSQLite` already assumes with its single
  writer connection. A second writing process would defeat any in-process
  serialization. Making plan writes safe across processes is a separate contract.
- **Truncation protection for the browser write path.** A browser write that
  shrinks a plan can still coalesce into the latest revision and overwrite it in
  place. That is today's behavior, which this capability neither introduces nor
  changes; `AC-TASKS-PLAN-WRITE-CONSISTENCY-003.2` only declines to add the warning.
  Extending force-append to shrinking browser writes changes what a human sees in
  their own revision list and needs its own decision. A follow-up would need: which
  shrink threshold applies to a human editor, whether the editor surfaces anything,
  and whether undo semantics change.
- **The truncation thresholds themselves.** The 2000 character floor and the
  one-half retain ratio are inherited unchanged; retuning them is not part of this
  capability.
- **Reading a past revision through MCP.** The warning states that prior content is
  not fetchable through the MCP plan tools. Adding one is a separate capability.
- **The `task_documents` migration.** `REQ-TASKS-DOCUMENTS-001` generalizes plans
  into multiple documents. This capability constrains the existing plan tables only.
- **Empty-content validation asymmetry.** The agent write path rejects empty content
  and the browser write path does not. That difference is pre-existing.

## Prior art

Two sources were consulted before this contract was written.

**Our own prior reasoning (wiki).** Unavailable: no wiki vault is configured here, so
this contract was written without whatever the wiki holds on read-modify-write races
or coalescing. Anyone who can reach the vault should re-run the query before treating
this section as complete.

**What other products shipped (`saas-kb`, `category: ai_sdlc`).** Returned nothing
useful. Two queries were run, on concurrent document edit conflict detection and
versioning, and on agent plan overwrite protection and revision restore. Every hit
scored at noise level (best 0.0164), mostly release changelogs; the closest adjacent
material, Augment Code's "Checkpoints" (workspace snapshots), documents no concurrent
write contract.

**What we are doing differently, and why.** The corpus's one adjacent pattern is
recovery by workspace snapshot, which sidesteps the race by never merging, at the
cost of history volume and coarser recovery granularity. Kandev chose the opposite
for plans, per-document revision history plus in-place coalescing of rapid
same-author edits, and that choice creates this problem: coalescing is a destructive
merge, so the recovery path can be overwritten by the write it protects against. This
capability keeps coalescing and instead requires the merge target and the truncation
report to be decided against the state the write actually commits against, within a
single process.
