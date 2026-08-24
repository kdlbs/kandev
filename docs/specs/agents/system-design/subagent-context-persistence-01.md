---
status: draft
system: agents
requirements:
  - REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001
created: 2026-08-12
updated: 2026-08-14
owners:
  - nova28
---
# Subagent context persistence System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

> **Current implementation boundary (2026-08-15).** The execution-aware
> `task_session_subagents` shape is created directly by schema initialization.
> The predecessor two-column shape and the `subagent_context_execution_since`
> activation key were not shipped on the supported base branch, so the current
> implementation does not include a compatibility rebuild or shape-probing
> migration for them. Historical amendment text below records the earlier
> design discussion; the supported migration work is the historical-message
> backfill and its two activation keys. See
> [ADR-2026-08-15-subagent-context-schema-boundary](../../../decisions/2026-08-15-subagent-context-schema-boundary.md).

## Amendment log

**Amendment 1 (2026-08-14) — row identity gains an execution dimension.**

The original contract keyed the upsert on `(task_session_id, tool_call_id)` and
justified that as "session-scoped by design". Code review of PR
[#2617](https://github.com/kdlbs/kandev/pull/2617) by a repository maintainer
established that this key is **wrong, not merely narrow**: `tool_call_id` is
unique only within one *agent execution*, so a late frame from a finished
execution can overwrite a different, newer execution's row.

This is not hypothetical, and the amendment does not rest on the reviewer's
authority. It was confirmed from the code:

- `allocateCodexEmittedToolCallIDLocked`
  (`internal/agentctl/server/adapter/transport/acp/adapter_tools.go:448-475`)
  de-duplicates emitted tool-call IDs against `a.codexEmittedToolCallIDs`, an
  **in-memory map owned by one adapter instance**. A new execution constructs a
  new adapter with an empty map, so a wire `tool_call_id` the agent reuses is
  re-emitted verbatim with no disambiguating suffix. Uniqueness is therefore
  guaranteed **per execution**, never per session.
- Each execution is a fresh `uuid.New().String()`
  (`internal/agent/runtime/lifecycle/manager_execution.go:572`,
  `manager_launch.go:526`), so executions rotate on every launch and resume
  within one long-lived session.
- The exposure widened when the PR's own fix made late frames from
  already-completed executions *recorded* rather than dropped
  (`internal/orchestrator/event_handlers_streaming.go:325-333`). Before that
  fix such a frame was discarded and could not clobber anything; after it, it
  can. The root defect (no execution identity in the key) predates that fix.

What this amendment changes: `agent_execution_id` is added to the data model and
to the upsert key (AC-30 … AC-34), and the ACs that named the two-column key are
restated — **AC-2a, AC-3, AC-13, AC-14, AC-22, AC-28**. Sections carrying an
amended contract are marked *(amended by Amendment 1)*. Everything else in this
document is unchanged and still binding.

### Amendment 1, revision 1 (2026-08-14) — failure and detection semantics

Spec Review returned **FIX FIRST** on Amendment 1 with eleven findings, two
independent outside-voice legs concurring on three of them. The key change itself
was confirmed sound and is unaltered. What was missing was **failure and detection
semantics**: under a migration runner that swallows errors, the spec did not say
what "the migration succeeded" means, what happens when it does not, or what
durable datum marks the point from which each guarantee holds.

Revision 1 adds only that, plus five smaller corrections. It changes no existing
AC's intent:

- **A third activation key, `subagent_context_execution_since`** (AC-24), because
  `subagent_context_capture_since` is write-once and was written by the
  *pre-amendment* build — it marks the start of capture, not the amendment
  boundary, so it cannot separate a legacy `'unknown'` row from a live frame that
  arrived without an execution id. The old claim that it could was false and is
  removed.
- **A failure contract for the backfill** (AC-24a) — an activation key is never
  written after a failed backfill.
- **A failure contract for the AC-33 rebuild** (AC-33a), and AC-33 scoped to both
  dialects, because `recreateTable` returns `(false, nil)` on PostgreSQL by
  construction; the precedent it cites covers only the SQLite half.
- **AC-5's exception list enumerated** (it claimed AC-9 was its only exception; at
  least six other criteria override it), and AC-1's `source` requirement reconciled
  with `source`'s write-once guarantee.
- **AC-7 given a detection rule** for `total_tokens` and `duration_ms`, which are
  plain `int64` and cannot express "not reported" the way `ToolUseCount *int` can.
- Corrections to AC-8, AC-11, AC-15, AC-21, AC-26, AC-28 and the index rationale.

Passages carrying revision 1 are marked *(revision 1)*.

### Amendment 1, revision 2 (2026-08-14) — activation semantics and the two unnamed backfill sources

Spec Review returned **FIX FIRST** on revision 1 with twelve findings, from a
cross-vendor leg (codex) and a cross-model leg (fable) plus the reviewer's own
pass. Three of them were found independently by all three voices. No existing
AC's intent changes; every fix below is additive.

Revision 1 added failure semantics. What it did not do was say **when each
activation key gets written on each kind of installation**, and it left two
backfill column sources unnamed. Revision 2 fixes exactly that, plus six smaller
corrections:

- **The activation keys are now written off the OBSERVED END STATE, not off
  whether a migration fired** (AC-24, AC-33b). Revision 1 tied
  `subagent_context_execution_since` to AC-33, whose trigger is a table that
  "predates Amendment 1" — so a fresh database, and any database that predates
  the feature entirely, never wrote it, and AC-24b then permanently declared
  those installations un-amended. That was the single worst defect in revision 1:
  it inverted the detection signal on the majority of future installations.
- **All four key states are now defined** (AC-24c), not just one.
- **`subagent_context_backfill_through` is derived from the source predicate, not
  from what one attempt inserted** (AC-24). Revision 1's tightening was wrong on a
  retry, where `ON CONFLICT DO NOTHING` inserts nothing and the literal reading
  published the empty string.
- **The keys carry nanosecond precision and comparisons are defined at equality**
  (AC-24), because they were being compared against a microsecond-precision column.
- **Migration ordering is stated** (AC-33c): the AC-33 shape change runs before the
  AC-21 backfill, and the backfill does not run against a stale shape.
- **AC-28 defines what to do when `capture_since` is absent** — the state AC-24a
  creates and AC-20 points at — and **AC-28/AC-29 now compare an execution-agnostic
  distinct-key count**, so the extra rows Amendment 1 deliberately creates can no
  longer bank headroom that hides a stopped writer.
- **The backfill's `tool_status` and `turn_id` sources are named** (AC-21). Both were
  verified at the writer: `metadata.status` is written from
  `payload.Data.ToolStatus`, and the message's `turn_id` column is written from the
  orchestrator's resolved turn id.
- Plus a duplicate-source tiebreak (AC-21), `turn_id` NULL fill-forward (AC-2a), the
  later-reported-zero rule (AC-7a), `task_id` added to AC-5's exception table, and a
  narrowed observability claim in the acceptance-criteria preamble.

Passages carrying revision 2 are marked *(revision 2)*.

### Amendment 1, revision 3 (2026-08-14) — the activation keys as instants, and the excess direction

Spec Review returned **FIX FIRST** on revision 2 with nine findings, from a
cross-vendor leg (codex), a cross-model leg (fable) and the reviewer's own pass.
No existing AC's intent changes; every fix below is additive.

Revisions 1 and 2 built the activation keys up as a *detection story* — when each
is written, what each absence means. What neither reached is the layer underneath:
**the keys are strings in a text column, and nothing said how a string becomes a
comparison against a timestamp column.** That gap is not theoretical. It is live in
the shipped pre-amendment writer, and it was measured rather than argued:

- **The comparison is broken today, and the boundary snaps to midnight.**
  `task_session_messages.created_at` stores `2026-07-30 01:39:37.271235+00:00`
  (space separator, microseconds, `+00:00` offset — sampled from the reference
  store). `subagent_context_capture_since` is written
  `time.Now().UTC().Format(time.RFC3339)` and `subagent_context_backfill_through`
  via `rfc3339Timestamp`, both producing `2026-07-30T01:39:37Z`. Compared as text,
  `' '` (0x20) sorts before `'T'` (0x54), so **every row on the key's own calendar
  day reads as "before" the key regardless of its actual time.** Measured in SQLite
  with the real formats: same instant → `0`; a row 23h59m *after* the key, same day
  → `0`; a key from the previous day → `1`. Only the date part is doing any work.
  AC-24 now requires a timestamp comparison, not a string comparison (AC-24e).
- **Second precision, not nanosecond.** Both shipped key writes emit seconds —
  `time.RFC3339` and `strftime('%Y-%m-%dT%H:%M:%SZ', …)`. `rfc3339Timestamp`
  (`base_migrations.go`) is the helper a builder naturally reaches for and it
  **cannot** produce the precision AC-24 mandates. Named so it is not reached for.
- **`backfill_through = ''` is the fresh-install default and had no defined
  meaning** (AC-24f). The shipped statement already `COALESCE(…, '')`s it, AC-24
  mandates it, and AC-24's own RFC3339Nano requirement contradicted it.
- **The `MAX` runs as a separate statement after the INSERT** — three independent
  `r.migrate.Apply` calls, no transaction — so "at that same instant" was asserted
  and not mechanised. AC-24d now fixes the evaluation order (SR30).
- **AC-28's excess direction was undefined**, and closing it naively would have
  reintroduced the banking hole revision 2 removed: if excess and shortfall net
  against each other in one signed difference, banked excess still absorbs missed
  writes. AC-28 now measures the two directions as **separate anti-joins**, so
  neither can cancel the other and AC-29 holds unconditionally.
- Plus: a negative metric no longer blanks a learned measurement (AC-9, the same
  rule revision 2 gave the reported zero), AC-12 defers to AC-5 after settlement,
  AC-1a is scoped to commit order, AC-33b's key write is gated on the clauses that
  are actually schema-observable, and a stale revision-1 row is removed from
  *Failure modes*.

Passages carrying revision 3 are marked *(revision 3)*.

### Amendment 1, revision 4 (2026-08-14) — the keys as written values, and two queries that did not run

Spec Review returned **FIX FIRST** on revision 3 with ten findings, from a cross-vendor leg
(codex), a cross-model leg (fable) and the reviewer's own pass. No existing AC's intent
changes, and **revision 4 adds no new acceptance criterion** — every fix is a clause inside
a criterion that already existed.

Revisions 1–3 built the activation keys up as a *detection story* (when each is written),
then as *comparable instants* (AC-24e). What none of them reached is the layer underneath
both: **the keys are values that something has to produce, on installations that already
exist.** Four of the ten findings are that one theme:

- **`capture_since` was defined as an instant nothing can observe.** AC-24 called it "the
  instant the backfill statement committed successfully" while AC-24d requires the key write
  to be atomic with that insert — and the commit instant is knowable only *after* commit,
  when an atomic write is no longer possible. A builder had to invent transaction-start,
  pre-commit sample, or a database clock, and those publish measurably different boundaries.
  AC-24 point 4 now fixes the sample point and states which direction of error is safe.
- **Keys the shipped build already wrote had no disposition.** Revision 3 measured them at
  second precision and mandated RFC3339Nano, while AC-24 makes the keys write-once — two
  requirements that cannot both hold on any already-activated installation. AC-24 point 5
  settles it: write-once wins, the key stands, and the residual second is disclosed.
- **A partial key pair is already reachable on the installed base.** The pre-amendment build
  applies the insert and both key writes as three independent `r.migrate.Apply` calls and
  guards re-runs on `capture_since` alone, so a database can hold `capture_since` without
  `backfill_through` — permanently, because the guard then suppresses every retry. AC-24d
  now requires the guard to be **both** keys, which repairs the state with machinery the
  spec already had.
- **AC-24e mandated a precision it named no mechanism for.** The repository's own
  `timestampColumn` renders SQLite's side as `julianday()`, a double whose ULP here is
  roughly 40 µs — coarser than the microsecond column — and, worse, revision 3's mandated
  same-day test *passes* against it. AC-24e now sets the floor at the column's own
  precision, forbids `julianday`, and *Verification* adds the one-microsecond boundary test
  that actually discriminates.

The other six: AC-28's two normative anti-joins **did not parse on PostgreSQL** (no
derived-table alias) and **bypassed the JSON normalization AC-23b exists to mandate**, so a
single legacy `''` metadata row would disable the health check on either dialect; AC-25's
"unknown execution identity **by construction**" is falsified by the self-heal state
revision 3 itself introduced, where real UUIDs precede the key; AC-29's `MAX(observed_at)`
had no window and no empty-result rule; AC-28's excess anti-join and its prose described
different sets; and the observability taxonomy's AC-25 exclusion was never extended to the
consumer clauses revisions 1–3 added.

Passages carrying revision 4 are marked *(revision 4)*.

## Why

When an agent fans out to subagents (Claude's `Task` tool, OpenCode's `task`,
Cursor's `Task`, Auggie's `sub-agent-<type>:`), Kandev already recognises the
fan-out and already receives the child's identity and usage on the completion
frame. It renders them, counts them in memory, and then keeps no queryable
record of them.

The consequence is a specific, measured distortion. In the reference store
(`~/.kandev/data/kandev.db`, snapshotted 2026-08-12), **sessions per card is
exactly 1.0 for every step**, while 253 subagent invocations exist across 38
sessions and 127 turns — including **42 turns that fanned out to exactly three
subagents**. A step that spawned three parallel reviewers is, in the store,
indistinguishable from one long conversation. Every per-context cost figure is
therefore wrong in the same direction, and the Ops Cost analysis of the Review
step's cache cost had to *infer* a three-way fan-out by reading the step's
prompt, because nothing in the store recorded it.

## What this feature is, and is not

**It is persistence and a parent link.** The data already arrives on a frame
Kandev parses (`internal/agentctl/server/adapter/transport/acp/subagent.go`) and
is already carried in a typed payload
(`streams.SubagentTaskPayload`). This spec adds a durable, queryable row per
subagent invocation, linked to the session and turn that spawned it. It adds no
new instrumentation, no new agent-side capability, and no new wire fields.

**It is not a session.** A subagent context is deliberately *not* a
`task_sessions` row. See *The design question*, below — that decision is part of
the contract, not an implementation detail.

**It is not a UI feature.** No frontend surface changes. The existing subagent
card already renders from message metadata and continues to do so, unchanged.

**It is not cost attribution.** Token counts are stored exactly as the agent
reported them, with provenance. No pricing, no dollars, no rollup into
`task_sessions.cost_subcents`.

### One correction to the framing

The originating analysis states the data is "never persisted". That is
imprecise, and the imprecision matters to the design. The normalized payload
*is* written today — as opaque JSON at
`task_session_messages.metadata.normalized.subagent_task` (253 such rows exist
in the reference store). What does not exist is a **relational fact**: no
stable row identity, no first-class parent link, no column a query can group by,
and no independent record — the JSON lives inside a row that is cascade-deleted
with its session.

This matters twice over:

1. It makes a **backfill possible**, which this spec takes (see *Backfill*).
2. It makes the message table an **independent expected-count source** for
   writer health (see *Writer health*).

---

## The design question: a row in `task_sessions`, or its own table?

**Answer: its own table, `task_session_subagents`.** The parent link is a
foreign key from that table to `task_sessions`, not a `parent_session_id` column
added to `task_sessions`.

The reasoning, from the code as it stands:

**A subagent has none of the things a `task_sessions` row asserts.** It has no
workspace and no `workspace_path`; no executor, `executor_profile_id`, or
`task_environment_id`; no worktree or `base_branch`; no `agent_profile_id`; no
`executors_running` row; no ACP session Kandev can `session/load` or resume; no
`state` machine (`CREATED` → `RUNNING` → …); no launch, stop, or recover path.
A row in `task_sessions` implies all of those, and every one would be a lie.

**`task_sessions` is read by paths that would be actively harmed.** Session
rows feed lifecycle recovery (`RecoverInstances`), the orchestrator, session-tab
listings, WebSocket subscription scoping, the auth scoper
(`AuthorizeSessionAccess` resolves session → task → workspace → owner), the boot
payload, and cost rollups. Inserting non-launchable rows there forces a new
"is this real" predicate into every one of them, and any path that forgets it
becomes a defect.

**It would silently break the very series this feature exists to fix.** The
downstream extract's `dim_session` is `SELECT … FROM task_sessions`. Adding
subagent rows to that table would change what "sessions per card" *means*
mid-series, with no schema versioning to signal it — which is precisely the
discontinuity the cross-cutting rule for this work forbids. A separate table is
purely additive: the existing series stays comparable, and the new fact is
opt-in for any consumer that wants it.

The name follows the existing sibling convention: `task_session_messages`,
`task_session_turns`, `task_session_commits`, `task_session_subagents`.

---

## Sampled input shapes

Every shape below was read from the live store on 2026-08-12, not assumed.

### Population by terminal status (n = 253)

| status | n | agent_id | child_session_id | model | total_tokens | tool_use_count | duration_ms |
|---|---|---|---|---|---|---|---|
| `async_launched` | 190 | 190 | 0 | 190 | **0** | **0** | **0** |
| `completed` | 57 | 57 | 0 | 49 | 57 | 57 | 57 |
| `started` | 5 | 0 | 5 | 0 | 0 | 0 | 0 |
| *(absent)* | 1 | 0 | 0 | 0 | 0 | 0 | 0 |

Three facts drive the whole contract:

1. **75% of subagent invocations report no usage at all.** `async_launched` is
   Claude's `run_in_background` dispatch; the dispatch is terminal for the
   `Task` tool, the subagent runs out-of-band and writes to an `outputFile`
   Kandev never reads. These rows will never gain tokens, duration, or a tool
   count. Storing `0` for them would fabricate 190 measurements.
2. **`child_session_id` is empty for every Claude row** and is populated only by
   OpenCode (5 rows, all non-terminal). It cannot be the identity.
3. **`tool_call_id` is present on all 253 rows and unique across all 253.**
   `agent_id` is unique among the 247 rows that carry one, but is absent for
   three of the four supported agents. `tool_call_id` is the identity —
   *(amended by Amendment 1)* **paired with the execution that emitted it**.
   The observed uniqueness across 253 rows is a property of this sample, not a
   guarantee: see *Execution identity, as sampled* below for why it does not
   hold in general.

### Other measured facts

- **Fan-out distribution per turn:** 1→65 turns, 2→12, **3→42**, 4→5, 5→2, 8→1.
- **Nesting:** exactly 1 of 253 rows carries a `parent_tool_call_id` — a
  subagent that spawned a subagent. Rare, real, and must be specified.
- `turn_id` and `task_session_id` are non-empty on all 253 rows.
- `subagent_type` is missing on 3 rows.
- No two subagent rows within a turn share a `created_at` (microsecond
  precision), so ties are not observed — but they are not prevented either.
- Observed window: 2026-08-03 15:32:20Z to 2026-08-12 13:24:58Z.

### The payload as parsed

`streams.SubagentTaskPayload` carries `Description`, `Prompt`, `SubagentType`,
`Status`, `AgentID`, `Model`, `ChildSessionID`, `DurationMs`, `TotalTokens`,
`ToolUseCount *int`, `ResultText`, `IsAsync`, `OutputFile`,
`CanReadOutputFile`. `ToolUseCount` is already a pointer specifically so a
reported `0` is distinguishable from "not reported" — that distinction is
load-bearing here and must survive into the column.

### Execution identity, as sampled *(added by Amendment 1)*

Read from the code on 2026-08-14, because the amendment adds a column and a
spec may not assume a field exists.

**It is already on the frame.** `lifecycle.AgentStreamEventPayload` carries
`ExecutionID string \`json:"execution_id"\`` — documented in place as
"Lifecycle execution ID; stable across the payload's lifetime"
(`internal/agent/runtime/lifecycle/event_types.go:231`). Both publish sites set
it from `execution.ID`
(`lifecycle/events.go:226`, `lifecycle/manager_streaming.go:287`), and the
orchestrator already reads it on this exact path: `recordSubagentContextFromFrame`
is called from handlers that pass the same `payload`, and
`shouldDropCompletedExecutionStreamEvent` logs it as `agent_execution_id`
(`internal/orchestrator/event_handlers_streaming.go:855-867`). **No new wire
field, no new instrumentation** — this amendment stays inside the original
scope note.

**Its shape.** `execution.ID` is a `uuid.New().String()`
(`lifecycle/manager_execution.go:572`, `manager_launch.go:526`). A reserved
sentinel that is not a UUID therefore cannot collide with a real value.

**It can be empty.** `shouldDropCompletedExecutionStreamEvent` returns early
when `payload.ExecutionID == ""`, so the empty case is reachable in code rather
than merely conceivable. AC-31 defines it rather than leaving it to a builder.

**Historical messages do not carry it.** `task_session_messages`
(`internal/task/repository/sqlite/base_schema.go:679-694`) has columns
`id, task_session_id, task_id, turn_id, author_type, author_id, content,
requests_input, type, metadata, created_at, updated_at` — and no execution
identity, in the row or in the JSON the backfill reads. The backfill therefore
*cannot* derive a real value; AC-31's sentinel is a necessity, not a
convenience.

**Resume does not split a subagent across executions.** A subagent's frames
reaching two different executions would turn execution-scoping into a
row-splitting defect. It cannot happen on the replay path: `handleACPUpdate`
suppresses `ToolCall` and `ToolCallUpdate` (among others) while
`isLoadingSession` is set during `session/load`
(`adapter/transport/acp/adapter_updates.go:136-160`), so replayed history never
reaches the orchestrator. A live execution's own Task tool call dies with its
process and emits nothing into its successor. One subagent invocation therefore
belongs to exactly one execution, which is what makes the three-column key safe.

**The existing constraint is an inline table constraint, not an index.**
`UNIQUE (task_session_id, tool_call_id)` is declared inside `CREATE TABLE`
(`base_schema.go:650`). SQLite cannot alter a table constraint in place, so
changing the key on a database that already has the table requires a table
rebuild; the repository already owns that pattern (`recreateTableNamed`,
`base_migrations.go:857`, `:911`). AC-33 states the required end state and
leaves the mechanism to Build.
