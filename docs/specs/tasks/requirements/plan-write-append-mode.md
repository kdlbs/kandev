---
status: draft
system: tasks
created: 2026-09-01
owners:
  - kandev
---

# Task plan append-mode write Requirements

## Overview

`update_task_plan_kandev` replaces the whole plan document. There is no partial write at
any layer, so a caller adding a section must read the whole plan and send it back with
the addition attached. One write therefore costs the size of the entire plan, and a run
costs the square of the number of writes in it.

The plan is also a task's only durable memory across a context reset, so it grows
monotonically for as long as the task runs. On this instance the largest stored revision
is 158,393 characters, so the two effects compound in production, not in theory.

This capability adds a second write mode in which the caller submits only the fragment
it wants to add and the system composes the stored document. It changes what a caller
must send and what the system must guarantee about composing it, not what a plan is, how
revisions are numbered, or how the browser edits one.

The `tasks` system owns this capability because the durable artifact is the plan and its
revision history, not the MCP surface exposing the write. The missing-task boundary
stays owned by `REQ-TASKS-DOCUMENTS-001`.

Write serialization and truncation reporting belong to `plan-write-consistency`; the
stored-content size limit to `plan-content-size-limit`, both in this canon. This
capability composes inside the per-task write lock the first established, and
`REQ-TASKS-PLAN-APPEND-007` measures an append against the second.
`REQ-TASKS-PLAN-APPEND-003` still states in full what append requires of serialization,
so this contract stays readable and testable on its own.

## Prior art

Recorded in full, with both search receipts, in the system design under [Prior
art](../system-design/plan-write-append-mode.md#prior-art).

## Terminology

- **Plan write:** an operation that upserts the task plan HEAD row and writes or merges
  a revision.
- **HEAD:** the single `task_plans` row for a task, holding the current content.
- **Mode:** the requested composition behavior of an agent plan update, either `replace`
  or `append`.
- **Fragment:** the `content` a caller submits in `append` mode; an addition to the
  stored document, not the whole of it.
- **Composed content:** the content a plan write commits in `append` mode, formed from
  the stored content and the fragment.
- **Stored content:** the plan content in effect for that task at the instant the
  composing write reads it inside the serialized write path.
- **Agent write path:** the MCP tools `create_task_plan_kandev` and
  `update_task_plan_kandev`.
- **Browser write path:** the WebSocket actions handled by `wsCreateTaskPlan` and
  `wsUpdateTaskPlan`.
- **Truncation guard:** the existing check reporting a write that retains less than half
  the characters of the content it replaces.
- **Whitespace:** a character whose Unicode `White_Space` property is true. This set is
  normative wherever this document says "whitespace"; it includes U+0085 NEL and U+00A0
  no-break space, so a hand-rolled `" \t\r\n"` test does not satisfy this contract.
- **Whitespace-only line:** a line whose characters are all whitespace, excluding the
  line terminator.
- **Size limit:** the maximum stored plan content a single write may commit, owned by
  `plan-content-size-limit` and unchanged here.

## Requirements

### REQ-TASKS-PLAN-APPEND-001: A caller selects the write mode explicitly and an
unrecognized mode changes nothing

**Intent:** The default must stay what every existing caller relies on, and an
unrecognized value must not be interpreted: treating it as `replace` would let one typo
overwrite a plan with a fragment, the destructive outcome this capability exists to
remove.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-001.1:** When an agent plan update omits the mode or supplies
  an empty mode, the system shall treat the write as `replace`.
- **AC-TASKS-PLAN-APPEND-001.2:** When an agent plan update supplies the mode `replace`
  or `append`, the system shall perform that mode.
- **AC-TASKS-PLAN-APPEND-001.3:** When an agent plan update supplies any other mode
  value, including a value differing only in letter case such as `Append` or `APPEND`,
  the system shall reject the request as a validation error, name the two accepted
  values in the error, leave the stored plan content, title and revision history
  unchanged, and create no revision.
- **AC-TASKS-PLAN-APPEND-001.4:** When an agent plan update supplies no content, or
  supplies content that is empty, the system shall reject the request as a validation
  error in either mode and create no revision.
- **AC-TASKS-PLAN-APPEND-001.5:** When an agent plan update in `append` mode supplies a
  fragment consisting only of whitespace as this document defines it, the system shall
  reject the request as a validation error and create no revision, because such a
  fragment can only add separator noise to a document from which it cannot be removed.
- **AC-TASKS-PLAN-APPEND-001.6:** When an agent plan update in `append` mode targets a
  task that has no plan, the system shall reject the request with the same
  plan-not-found outcome that `replace` produces for that task, and shall not create a
  plan.
- **AC-TASKS-PLAN-APPEND-001.7:** When one request violates more than one criterion, the
  system shall report exactly one failure, selected in this order: mode validity
  (AC-TASKS-PLAN-APPEND-001.3); task-reach authorization (AC-TASKS-PLAN-APPEND-005.6);
  content validity (AC-TASKS-PLAN-APPEND-001.4, AC-TASKS-PLAN-APPEND-001.5); plan
  existence (AC-TASKS-PLAN-APPEND-001.6); failure to read the stored content
  (AC-TASKS-PLAN-APPEND-003.5); the size limit (AC-TASKS-PLAN-APPEND-007.2). In every
  case the stored plan content, title and revision history are unchanged and no revision
  is created. Missing-task is not in this order; it stays owned by
  `REQ-TASKS-DOCUMENTS-001`.

### REQ-TASKS-PLAN-APPEND-002: An append composes the stored document with a separator
that cannot join two lines

**Intent:** Downstream consumers count headings. A fragment beginning with a heading
appended onto the last line of the stored content yields a line that is no longer a
heading, so any consumer counting `##` headings silently reads one fewer. Stated over
headings generally, not one consumer's heading text, so the rule outlives any counter
built on it. The separator is contract, not implementation.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-002.1:** When a plan write in `append` mode commits, the
  composed content shall consist of the stored content, then a separator, then the
  fragment, in that order. The only characters the system may add are the separator
  itself; the only ones it may remove are the edge whitespace that
  AC-TASKS-PLAN-APPEND-002.2, AC-TASKS-PLAN-APPEND-002.3 and AC-TASKS-PLAN-APPEND-002.5
  each require it to remove. Beyond those it shall perform no insertion, reordering or
  removal, and shall not examine or alter any interior character of either body. This
  criterion frames its siblings rather than contradicting them.
- **AC-TASKS-PLAN-APPEND-002.2:** When both the stored content and the fragment contain
  a non-whitespace character, the composed content shall contain exactly one empty line
  between the last non-empty line of the stored content and the first non-empty line of
  the fragment, regardless of how many line endings the stored content ends with or the
  fragment begins with. Leading spaces and tabs on the fragment's first non-empty line
  shall be preserved.
- **AC-TASKS-PLAN-APPEND-002.3:** When the stored content is empty or contains only
  whitespace, the composed content shall be the fragment with its leading empty and
  whitespace-only lines removed, shall preserve leading spaces and tabs on the
  fragment's first non-empty line, and shall not begin with a blank line.
- **AC-TASKS-PLAN-APPEND-002.4:** When a plan write in `append` mode commits, the
  composed content shall preserve the fragment's trailing characters as submitted, and
  the system shall neither add nor remove a trailing line ending.
- **AC-TASKS-PLAN-APPEND-002.5:** When a plan write in `append` mode commits, the
  composed content shall preserve every character of the stored content that precedes
  its trailing whitespace. Trailing whitespace on the stored content's final line may be
  dropped.
- **AC-TASKS-PLAN-APPEND-002.6:** When a plan write in `append` mode supplies a title,
  the system shall apply it exactly as `replace` does; when it supplies none, the system
  shall leave the stored title unchanged.

### REQ-TASKS-PLAN-APPEND-003: An append reads and commits against the same state, and
never commits a fragment alone

**Intent:** An append is a read-modify-write. If the read runs outside the serialization
that orders plan writes, two concurrent appends read the same stored content and the
second commit overwrites the first, losing a section while both callers are told the
write succeeded. The replace-only API prevented that by construction; it must not return
one layer down, invisible.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-003.1:** When a plan write in `append` mode commits, the stored
  content it composed from shall be the plan content in effect for that task at the
  instant that same write commits, not content read before another write to the same
  task committed.
- **AC-TASKS-PLAN-APPEND-003.2:** When two plan writes in `append` mode for the same
  task run concurrently and both report success, the resulting plan content shall
  contain both fragments in full.
- **AC-TASKS-PLAN-APPEND-003.3:** When plan writes for the same task run concurrently,
  the resulting plan content shall equal the result of performing those writes one after
  another in some order. The system shall not guarantee which order, and shall not
  guarantee that the order matches the order in which the callers submitted them.
- **AC-TASKS-PLAN-APPEND-003.4:** When a plan write in `append` mode runs concurrently
  with one in `replace` mode for the same task, the system shall apply
  AC-TASKS-PLAN-APPEND-003.3 to that pair. A replace committing between an append's read
  and that append's commit would otherwise be discarded entirely, leaving content
  matching no serial order. Serializing appends against each other alone does not
  satisfy this criterion.
- **AC-TASKS-PLAN-APPEND-003.5:** When the stored content for an `append` cannot be
  read, the system shall fail the write, create no revision, leave the stored plan
  unchanged rather than committing the fragment as the whole document, and report the
  failure distinguishably from AC-TASKS-PLAN-APPEND-001.6's plan-not-found outcome,
  exposing no storage detail.
- **AC-TASKS-PLAN-APPEND-003.6:** When the same `append` request is submitted more than
  once, the system shall append the fragment once per accepted request. An append is not
  idempotent and repeated fragments shall not be deduplicated.

### REQ-TASKS-PLAN-APPEND-004: The truncation guard does not judge an append

**Intent:** The guard catches a replace that dropped content. An append cannot drop
content, since the composition contains the stored content in full, so running the guard
there yields only a false warning and a needless revision split, teaching an agent to
distrust an otherwise reliable warning.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-004.1:** When a plan write in `append` mode commits, the system
  shall emit no truncation warning, whatever the ratio between the fragment's length and
  the stored content's length.
- **AC-TASKS-PLAN-APPEND-004.2:** When a plan write in `append` mode commits, the system
  shall not force a new revision on the truncation guard's account, and shall leave the
  write eligible for the existing coalescing rules.
- **AC-TASKS-PLAN-APPEND-004.3:** When a plan write in `append` mode commits, the
  response shall carry no truncation warning field and no prior revision number field,
  matching a non-truncating write's shape today.

### REQ-TASKS-PLAN-APPEND-005: Replace-mode behavior and every other plan surface are
unchanged

**Intent:** This capability adds a mode. A regression in the default would cost more
than the change saves, and surfaces never asked to change must not drift.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-005.1:** When a plan write runs in `replace` mode, the stored
  content, stored title, revision numbering, revision coalescing, truncation detection,
  forced-revision behavior, emitted events and response shape shall be identical to
  those produced before this capability existed. The **wording** of the truncation
  warning is excluded from this freeze and owned by REQ-TASKS-PLAN-APPEND-006; its
  trigger condition, the counts it reports and the response field carrying it are frozen
  here.
- **AC-TASKS-PLAN-APPEND-005.2:** When a caller supplies a non-empty mode to
  `create_task_plan_kandev`, the system shall **reject** the request as a validation
  error naming `update_task_plan_kandev` as the tool that offers `append`, store
  nothing, and leave stored content, title and revision history unchanged. It shall not
  ignore the value, which would commit the fragment as the entire plan. An absent or
  empty mode is treated as absent, leaving that tool's behavior unchanged, consistent
  with AC-TASKS-PLAN-APPEND-001.1.
- **AC-TASKS-PLAN-APPEND-005.3:** The browser write path shall have no mode field in its
  payload contract and shall **ignore** an unrecognized field rather than reject it —
  that path's existing behavior for any unknown field, and therefore what "unchanged"
  means for it. It never sends a mode, so it takes the `replace` default. The asymmetry
  with AC-TASKS-PLAN-APPEND-005.2 is deliberate.
- **AC-TASKS-PLAN-APPEND-005.4:** When a plan write in `append` mode commits, the system
  shall emit the same events, in the same order, that a `replace` write committing the
  same resulting content emits.
- **AC-TASKS-PLAN-APPEND-005.5:** When a plan write in `append` mode commits, it shall
  create or coalesce exactly one revision as a `replace` write does, and the stored
  revision content shall be the composed content, not the fragment.
- **AC-TASKS-PLAN-APPEND-005.6:** When a plan write in `append` mode runs, the system
  shall apply the same task-reach authorization `replace` applies, and shall not read
  the stored content of a task the caller cannot reach.

### REQ-TASKS-PLAN-APPEND-006: Every agent-facing text describing plan writes admits
that append exists

**Intent:** A caller decides between the two modes from the text the system shows it.
Several texts today assert there is no append mode, spread across both plan tools'
descriptions and `content` parameters and the truncation warning shown after a write
destroyed content. Left alone they argue against the mode; the warning does so at the
worst moment, when a
caller has just lost content and is steered from the remedy, and the create tool while
prescribing the read-then-resend pattern this capability removes. Softened too far, they
drop the warning that
keeps the replace path safe. Enumerating these texts has twice missed one, so
AC-TASKS-PLAN-APPEND-006.9 binds the class instead of a list.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-006.1:** The `update_task_plan_kandev` description shall name
  both modes, state which is the default, and state that `replace` submits the whole
  document while `append` submits only an addition.
- **AC-TASKS-PLAN-APPEND-006.2:** The `update_task_plan_kandev` description shall
  retain, scoped to `replace`, the warning that the write overwrites the entire document
  and that sending only a section deletes the rest.
- **AC-TASKS-PLAN-APPEND-006.3:** The `update_task_plan_kandev` description shall state
  that `append` places a blank line between the stored content and the fragment.
- **AC-TASKS-PLAN-APPEND-006.4:** The `update_task_plan_kandev` description shall state
  that an append is not idempotent and that resubmitting it adds the fragment again.
- **AC-TASKS-PLAN-APPEND-006.5:** The `update_task_plan_kandev` description shall state
  that `append` does not require the caller to read the plan first.
- **AC-TASKS-PLAN-APPEND-006.6:** When a plan write in `append` mode succeeds, the
  acknowledgement returned to the caller shall report the size of the stored document
  after the write rather than the size of the submitted fragment.
- **AC-TASKS-PLAN-APPEND-006.7:** The truncation warning shall not state or imply that
  no partial update or append mode exists.
- **AC-TASKS-PLAN-APPEND-006.8:** The truncation warning shall name `append` as the way
  to add a section without resubmitting the whole document, while continuing to state
  where the pre-write content lives, that the MCP plan tools cannot fetch it, and that
  the caller must not rewrite the plan from memory.
- **AC-TASKS-PLAN-APPEND-006.9:** No agent-facing text describing a plan write shall
  state or imply that no partial update or append mode exists, or direct a caller who
  wants to add a section to read the plan and resubmit it. This binds every tool and
  parameter description on the agent write path, not only those above. On
  `create_task_plan_kandev` each shall still state that **this** tool replaces the whole
  plan and name `update_task_plan_kandev` in `append` mode as the way to add a section,
  and none shall declare a `mode` parameter there, whose only mode behavior is the
  rejection in AC-TASKS-PLAN-APPEND-005.2.

### REQ-TASKS-PLAN-APPEND-007: An append is measured against the document it will store

**Intent:** The size limit measures what the caller submitted. In `append` the caller
submits a fragment but the system stores the composition, so measuring the submission
would let a small fragment commit an oversized document, defeating an existing limit.

#### Acceptance criteria

- **AC-TASKS-PLAN-APPEND-007.1:** When a plan write in `append` mode is admitted, the
  system shall evaluate the size limit against the composed content, not against the
  submitted fragment.
- **AC-TASKS-PLAN-APPEND-007.2:** When the composed content would exceed the size limit,
  the system shall reject the write, store nothing, create no revision, and leave the
  stored plan content and title unchanged. Composed content whose size is exactly the
  limit shall be accepted, matching the existing check's boundary.
- **AC-TASKS-PLAN-APPEND-007.3:** When the system rejects an append for exceeding the
  size limit, the size it reports shall be that of the composed content, so a caller is
  not told a fragment far below the limit exceeded it.
- **AC-TASKS-PLAN-APPEND-007.4:** When the system evaluates the size limit for an
  `append`, it shall do so against the same stored content that write composed from,
  inside the region that serializes plan writes for the task, so the value admitted is
  the value committed.
- **AC-TASKS-PLAN-APPEND-007.5:** When a plan write runs in `replace` mode, the size
  limit's value, the point at which it is evaluated and the error it produces shall be
  unchanged.

## Out of scope

- **The plan size limit itself.** Its value and error are owned by
  `plan-content-size-limit`. REQ-TASKS-PLAN-APPEND-007 fixes only *which* content an
  append is measured against; nothing here raises, lowers or removes the ceiling, nor
  bounds growth below it.
- **Revision compaction and server-side collapse.** An append creates or coalesces a
  revision as any other write does; none is merged, pruned or rewritten.
- **Automatic collapse of spent detail.** Collapsing stays a deliberate
  read-then-replace by the caller: per `concepts/agents-md-pattern.md`, automating the
  full rewrite is what erodes detail, so only accumulation becomes cheap.
- **A section-patch or replace-a-named-heading mode.** Only whole-document replace and
  end-of-document append are defined.
- **An idempotency key for appends.** AC-TASKS-PLAN-APPEND-003.6 states the
  non-idempotence rather than removing it; deduplication needs a caller-supplied key and
  a store for it — a separate contract.
- **A mode on `create_task_plan_kandev` or on the browser write path.** Fixed by
  AC-TASKS-PLAN-APPEND-005.2 and AC-TASKS-PLAN-APPEND-005.3.
- **Reading a past revision through MCP.** Unchanged: no plan tool returns one.
- **Prepend, or insertion at any position other than the end.**
