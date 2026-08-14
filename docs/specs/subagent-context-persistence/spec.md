---
status: draft
created: 2026-08-12
updated: 2026-08-14
owner: nova28
---

# Subagent context persistence

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

## Data model

A new table, created on fresh databases in the schema-init DDL **and**
introduced on existing databases by an idempotent migration in
`runMigrations()`, per the repository's schema rules.

| Column | Type | Null? | Meaning |
|---|---|---|---|
| `id` | TEXT PK | no | Kandev-generated UUID. Not an ordering key. |
| `task_session_id` | TEXT | no | Parent session. FK → `task_sessions(id)` ON DELETE CASCADE. |
| `task_id` | TEXT | no | Denormalised parent task, mirroring `task_session_turns`. |
| `turn_id` | TEXT | yes | Turn that spawned it. NULL when no turn was active. |
| `agent_execution_id` | TEXT | no | *(added by Amendment 1)* The lifecycle execution that emitted the frame (`AgentStreamEventPayload.ExecutionID`). Part of the upsert key. Never NULL: the reserved sentinel `'unknown'` is stored when no execution identity is available (AC-31). |
| `tool_call_id` | TEXT | no | Agent-supplied invocation identity. Unique only **within one execution** — see *Constraints and indexes*. |
| `parent_tool_call_id` | TEXT | yes | Set when a subagent spawned this subagent. NULL for top-level. |
| `subagent_type` | TEXT | yes | e.g. `security-reviewer`. NULL when not reported. |
| `description` | TEXT | yes | Short label from `rawInput.description`. |
| `agent_id` | TEXT | yes | Provider-supplied child agent id. NULL for providers that omit it. |
| `child_session_id` | TEXT | yes | OpenCode child ACP session. NULL elsewhere. |
| `model` | TEXT | yes | Resolved child model, verbatim (e.g. `claude-opus-5[1m]`). |
| `agent_status` | TEXT | yes | The **agent-reported subagent** status, verbatim (`completed`, `async_launched`, `started`, …). NULL when never reported. |
| `tool_status` | TEXT | yes | The **ACP tool-call** status of the launching `Task` call, stored verbatim. Non-terminal values include `pending` and `in_progress`; the six terminal values are enumerated in AC-11. NULL when never reported. |
| `is_async` | INTEGER | no | 1 for a detached dispatch, else 0. Default 0. |
| `total_tokens` | INTEGER | **yes** | As reported. **NULL when not reported.** |
| `tool_use_count` | INTEGER | **yes** | As reported. **NULL when not reported; 0 only when the agent reported 0.** |
| `duration_ms` | INTEGER | **yes** | As reported. **NULL when not reported.** |
| `source` | TEXT | no | `live` or `backfill`. Provenance. Default `live`. |
| `observed_at` | TIMESTAMP | no | When Kandev first observed the launch frame. |
| `settled_at` | TIMESTAMP | yes | When a terminal status was first observed. NULL while outstanding. |
| `updated_at` | TIMESTAMP | no | Last write to this row. |

Constraints and indexes *(amended by Amendment 1)*:

- `UNIQUE(task_session_id, agent_execution_id, tool_call_id)` — the upsert key.
  Uniqueness is **execution-scoped**, because that is the widest scope in which
  a `tool_call_id` is actually unique. The original two-column key assumed
  session scope; *Execution identity, as sampled* shows why that assumption is
  false, and the *Amendment log* records the review that caught it.

  The column order is the key's own contract, not incidental: `task_session_id`
  leads so the constraint's backing index is usable as a leftmost-prefix scan for
  session-scoped reads (AC-13), rather than being usable only for full-key
  lookups. *(revision 1)* An earlier draft went further and said this meant the
  key needed no second index — which contradicted the index list two bullets
  below, where `INDEX(task_session_id)` is still declared. The claim is narrowed
  to what is true: **the key order means no NEW index is required for AC-13's
  reads.** The pre-existing single-column index is retained, in both the
  fresh-database DDL and the AC-33 rebuild, so that a migrated database and a
  fresh one have byte-identical index sets (AC-33 clause 4) — dropping a
  now-redundant index is a separate, behaviour-neutral change with its own
  measurement, and silently doing it inside a correctness migration would make
  the AC-33 replay test encode an arbitrary choice.

  Two rows that differ **only** in `agent_execution_id` are the intended
  outcome, not a duplicate. They record two genuinely different invocations
  that happened to reuse an ID, and collapsing them is precisely the data loss
  this key prevents.
- FK `task_session_id` → `task_sessions(id)` ON DELETE CASCADE.
- **`agent_execution_id` carries no foreign key**, for the same reason
  `turn_id` does not. Execution rows live in `executors_running` and are
  removed when the executor stops (`agent_execution_id` was itself dropped from
  `task_sessions` by an earlier migration, `base_migrations.go:857`), so an FK
  would delete the measurement when the thing it measured finished — the exact
  failure the `turn_id` decision already rejected. It is a plain value column
  that happens to be part of the key.
- **`turn_id` carries no foreign key.** `task_session_messages.turn_id`
  cascades from `task_session_turns`, which would mean deleting a turn silently
  deletes the fan-out record for that turn. The measurement must outlive the
  turn row, so `turn_id` is a plain nullable column.
- `INDEX(task_session_id)`, `INDEX(task_id)`, `INDEX(turn_id)`.

Deliberately **not** stored: `prompt`, `result_text`, `output_file`. The prompt
and result are free text already retained on the message row; duplicating them
here doubles the store's largest text payload and adds a second copy of
prompt content to any extract. This table is structure, not content.

### Two statuses, deliberately separate

`agent_status` and `tool_status` are different measurements from different
layers and are **not** merged into one column:

- `agent_status` is the child's own report, read from
  `_meta.claudeCode.toolResponse.status` (or the provider's equivalent). Its
  vocabulary is provider-defined and open — `async_launched` is a value only
  Claude produces, and it means *dispatched, will never report usage*.
- `tool_status` is the ACP status of the launching `Task` tool call, which
  Kandev already classifies via `isTerminalToolStatus`. Its vocabulary is
  closed and provider-neutral.

Terminality (AC-11) keys off `tool_status`, because that is the signal the
orchestrator already treats as authoritative everywhere else. `agent_status` is
stored verbatim and is never used to decide whether the row has settled.

### Column-level normalization rules

These apply to every write path, live and backfill:

- **An empty string is stored as NULL**, never as `''`, for every nullable TEXT
  column. `SubagentTaskPayload` uses `""` for "absent"; the column uses NULL, so
  that `COUNT(model)` counts models rather than blanks.
- `is_async` defaults to `0` and is set to `1` only on a positive report; it is
  never set back to `0` by a later frame.
- `source` defaults to `'live'`.
- *(added by Amendment 1)* `agent_execution_id` is **NOT NULL and never
  empty**. It is the reserved literal `'unknown'` whenever no execution
  identity is available, and the frame's `ExecutionID` otherwise. It is
  write-once: once a row exists, no later frame changes its
  `agent_execution_id`, because doing so would move the row to a different
  identity. `'unknown'` cannot collide with a real value, which is always a
  UUID.

  **There is exactly one sentinel, deliberately shared** by all three sources
  of "no execution identity" — the backfill, rows written before this amendment,
  and a live frame whose `ExecutionID` was empty. A second sentinel would look
  tidier and would be a bug: the backfill's `ON CONFLICT DO NOTHING` (AC-22)
  only suppresses a duplicate when the backfilled row lands on the *same* key
  as the row already there, so a backfill sentinel distinct from the
  pre-amendment live sentinel would insert a second row for a subagent already
  recorded.

  *(revision 1)* **What sharing the sentinel does and does not preserve.** An
  earlier draft claimed `observed_at` against `subagent_context_capture_since`
  separates pre-amendment rows from a live frame that genuinely arrived without
  an execution id. **That claim was false and is withdrawn.**
  `subagent_context_capture_since` is write-once (AC-24) and was written by the
  build that shipped *before* this amendment, so it marks the start of capture,
  not the amendment boundary; after AC-33 re-sentinels every legacy row, a
  pre-amendment row and a post-amendment empty-`ExecutionID` row are
  indistinguishable — both `source = 'live'`, both `observed_at` after
  `capture_since`, both `'unknown'`.

  What actually separates them is the third activation key
  `subagent_context_execution_since` (AC-24), written by the AC-33 migration. A
  `'unknown'` row whose `observed_at` precedes it is a legacy row that never had
  an execution dimension; a `'unknown'` row whose `observed_at` follows it is a
  live frame that genuinely arrived without an execution id, and that case also
  increments the `unknown_execution` counter (AC-31). `source` continues to
  separate backfilled from live. Without that third key the distinction is not
  recoverable from the database at all, and AC-31's whole purpose — making a
  regression that silently stops propagating execution identity *observable* —
  would hold only for the lifetime of one process's expvar counters.
- `observed_at`, `settled_at`, `updated_at` are UTC wall-clock instants from the
  backend process (`time.Now().UTC()`), at the same precision as the sibling
  session tables.
- `task_id` is required (NOT NULL). WHEN a frame carries no task id, the row is
  not written and AC-2's `skipped_no_identity` path applies — a session-scoped
  row with no card cannot be joined to anything the feature exists to answer.
- Reported metrics are stored verbatim within `int64`; there is no upper clamp.
  The largest observed `total_tokens` is 275,717.

---

## Acceptance criteria

EARS-style. *(observability claim narrowed in revision 2.)* Each criterion is
verifiable, on one of three surfaces, named here so a builder does not hunt for a
database-observable outcome that a given criterion does not have:

- **Data** — most criteria: the outcome is a query against
  `task_session_subagents`, `task_session_messages`, or `kandev_meta`, or the value
  of an exported counter.
- **Process** — AC-1a, AC-20, AC-27, AC-33a: the outcome is a `WARN` log line, boot
  completing rather than aborting, or the enclosing message write surviving. A test
  asserts these with a log observer or by completing a boot; they are not queries.
- **Structural** — AC-14, AC-19, AC-23a, AC-23b, and *(revision 3)* AC-24d's
  evaluation-order clause, AC-24e, and AC-33b's second paragraph: these constrain *how*
  the implementation is built (a single atomic upsert with no read-then-write;
  `internal/db` error classification rather than string matching; one unbounded
  `INSERT … SELECT`; the dialect-aware JSON helpers; the `MAX` evaluated no later than
  the insert; a key parsed and bound as a timestamp rather than compared as a string; an
  end state observed from the live schema rather than inferred from a `fired` boolean)
  rather than what the resulting
  data looks like. Two implementations can produce identical rows and counters while
  only one satisfies them, which is the point: each rules out a mechanism that is
  correct today and fragile under change.

Revision 1 claimed every criterion was "observable from the database or from a
logged/exported counter without reading source", which was not true of the eight
above — four of them cannot be observed from data at all. **AC-25 is deliberately
excluded from this taxonomy**: it is published consumer semantics, a rule for
readers of this store rather than a behaviour of it, and the database can expose
provenance but cannot prove a consumer disclosed it.

*(revision 4)* **The same exclusion covers the consumer-facing clauses revisions 1–3
added, which were left inside the taxonomy by oversight.** Specifically: **AC-24b** in its
entirety; the *"Meaning, and what a consumer SHALL do"* column of **AC-24c**'s four-state
table; and **AC-24f**'s consumer clauses ("a consumer SHALL NOT read it as 'no boundary is
known'", and SHALL NOT treat post-`capture_since` history as unknown fan-out). Each is a
rule for a reader of this store, and this spec ships no reader — § *Out of scope* excludes
every read surface — so none of them is verifiable on any of the three surfaces above, and
a builder hunting a database-observable outcome for them is hunting the thing the taxonomy
exists to prevent.

What IS observable, and what *Verification* accordingly asserts, is the **state each
criterion classifies**: that the key is present or absent, and that the value written is
the instant or the `''` sentinel. The reading laid on that state is contract for
consumers, exactly as AC-25 is. AC-24's own SHALL-write clauses, AC-24d's atomicity and
AC-24f's "SHALL hold the empty string" remain squarely **Data** and are unaffected.

### Capture

**AC-1** *(amended by Amendment 1)* — WHEN the orchestrator observes a tool-call
frame whose normalized kind is `subagent_task`, and a non-empty session id, task
id and `tool_call_id` are all present, THEN a `task_session_subagents` row SHALL
exist for `(task_session_id, agent_execution_id, tool_call_id)` — where
`agent_execution_id` is the frame's execution id or `'unknown'` per AC-31.

*(revision 1)* WHEN that frame **creates** the row, THEN `source` SHALL be
`'live'` and `observed_at` SHALL be Kandev's observation time for that frame.
WHEN that frame **merges onto a row that already exists** — including a row the
backfill wrote — THEN `source` and `observed_at` SHALL both be left unchanged,
because both are write-once (see *Persistence guarantees*) and record where and
when the row was **first** established, not what most recently touched it.

This is stated because the two readings are observably different and the earlier
wording implied the wrong one. A live frame merging onto a backfilled row is
reachable: both sides key on the shared `'unknown'` sentinel, so they collide.
Relabelling that row `'live'` would destroy the provenance split AC-25 requires
consumers to disclose, and moving its `observed_at` forward would silently
re-date a measurement. The row keeps `source = 'backfill'` and its original
`observed_at`; the live frame still fills every other column per AC-4 and AC-5.

**AC-1a** — `observed_at` SHALL be the instant Kandev first observed a frame it
could *recognise* as a subagent, not the instant the subagent began work. For
Claude and OpenCode the initial `tool_call` carries no `rawInput`, so
recognition occurs on a later `tool_call_update`; the column is therefore a
Kandev observation time and SHALL NOT be published as a subagent start time.

*(revision 3)* **Precisely: `observed_at` is the observation time carried by the frame
that CREATED the row.** Where two frames for one key are processed concurrently, that is
the first frame to **commit**, which under reordering is not necessarily the numerically
earliest observation. `observed_at` SHALL NOT be recomputed as the minimum of the
observations seen, because AC-1 makes it write-once and a merging frame leaves it
unchanged.

The distinction is worth one sentence because the alternative reading is a test that
fails a correct implementation: "first observed" and "first committed" diverge only under
the concurrency AC-14 and AC-15 already contemplate, and a builder trying to satisfy the
stricter reading would add a `LEAST(observed_at, excluded.observed_at)` to the conflict
clause — which AC-1 forbids in the same breath as it forbids relabelling `source`. The
skew is bounded by how far apart two frames for one tool call can commit, which is
irrelevant at the resolution AC-29's `MAX(observed_at)` and AC-28's `:since` window
actually use.

**AC-2** — WHEN a subagent frame carries an empty `tool_call_id`, an empty
session id, or an empty task id, THEN the system SHALL NOT write a row, SHALL
increment the `skipped_no_identity` writer counter, and SHALL NOT fail the
enclosing message write or turn.

*(clarified by Amendment 1)* This skip set is exactly those three fields and
SHALL NOT be extended to `agent_execution_id`. An absent execution id is
handled by AC-31's sentinel, not by dropping the row: the other three fields
are what make a row joinable to anything, whereas a row with no execution
identity is still a correct, attributable record of a fan-out that occurred.
Skipping it would manufacture exactly the "absence of a row is not absence of a
subagent" hazard AC-25 exists to prevent.

**AC-2a** *(amended by Amendment 1)* — WHEN a `(task_session_id,
agent_execution_id, tool_call_id)` that already has a row is observed again
under a *different* `turn_id`, THEN the existing row SHALL be updated per AC-3
and its original `turn_id` SHALL be preserved. A second row SHALL NOT be
created, and the fan-out SHALL NOT be re-attributed to the later turn. A frame
bearing a different `agent_execution_id` is a different key and is governed by
AC-32, not by this criterion.

*(revision 2)* **"First observation wins" governs re-attribution between two
different NON-EMPTY turns. It does not freeze a NULL.** WHEN the stored `turn_id` is
NULL and a later frame for the same key carries a non-empty turn id, THEN the stored
`turn_id` SHALL be set to that value.

The two readings differ observably and the case is ordinary production behaviour, not
a corner. A NULL `turn_id` records that Kandev could not resolve a turn when it first
recognised the subagent — the orchestrator passes a literal `""` at two of its
`recordSubagentContextFromFrame` call sites, and its turn lookup returns `""` whenever
no turn is active or the peek errors. That is an absence of information, not an
assertion that the fan-out belonged to no turn. Freezing it would leave a real
fan-out permanently unattributable to the turn that spawned it, which is the single
measurement this feature exists to produce. Filling it forward is also what AC-4's
rule implies in mirror image: an absent value never blanks a stored one, and a stored
absence never blocks a real value.

**AC-3** *(amended by Amendment 1)* — WHEN a later frame for an
already-recorded `(task_session_id, agent_execution_id, tool_call_id)` arrives,
THEN the system SHALL update that same row and SHALL NOT create a second row.

**AC-4** — WHEN a frame reports a field that the stored row already holds
non-NULL, and the frame's value for that field is absent or empty, THEN the
stored value SHALL be preserved unchanged. (This mirrors `applySubagentResult`,
which never blanks a value already learned from an earlier frame.)

**AC-5** *(revision 1)* — WHEN a frame reports a non-empty value for a field,
THEN that value SHALL replace the stored value, EXCEPT for the fields enumerated
below, which are governed by their own criteria and SHALL NOT be replaced by this
rule.

The exception list is **exhaustive and normative**. An earlier draft named AC-9
as the only exception, which was wrong — at least six other criteria already
override AC-5, and AC-5 is what a builder codes the upsert's `SET` list from. The
fields AC-5 does **not** govern:

| Field | Governed by | Behaviour |
|---|---|---|
| `total_tokens`, `tool_use_count`, `duration_ms` when the reported value is negative *(revision 3)* | AC-9 | Read as "not reported": stored NULL on creation, and SHALL NOT blank a stored non-NULL value; `anomalous_value`++ either way |
| `total_tokens`, `duration_ms` when the reported value is exactly `0` *(revision 2)* | AC-7a + AC-4 | Read as "not reported"; SHALL NOT blank a stored non-NULL value |
| `turn_id` | AC-2a | First observation wins **between two non-empty turns**; a stored NULL is filled forward *(revision 2)* |
| `tool_status` once terminal | AC-12 | Frozen at the terminal value |
| `settled_at` | AC-11 | Write-once at first terminal observation |
| `source` | AC-1, AC-22, *Persistence guarantees* | Write-once at row creation |
| `observed_at` | AC-1, AC-1a | Write-once at row creation |
| `task_id` *(revision 2)* | AC-1, AC-2 | Write-once at row creation; a denormalised parent identity, never rewritten by a later frame |
| `agent_execution_id` | AC-30, AC-31, AC-32 | Write-once; it is part of the row's identity |
| `is_async` | *Column-level normalization rules* | Sticky: set to 1 on a positive report, never back to 0 |
| every other column, on a frame arriving **after** `settled_at` is set *(revision 3)* | AC-12 | Not an exception: AC-5 continues to govern unchanged. Only `tool_status` and `settled_at` freeze at settlement |

Every other column — `parent_tool_call_id`, `subagent_type`, `description`,
`agent_id`, `child_session_id`, `model`, `agent_status`, a non-terminal
`tool_status`, and the three metric columns when the reported value is neither
negative nor (for `total_tokens` and `duration_ms`) exactly zero *(revision 2)* —
follows AC-5's replace rule, subject to AC-4 (an absent or empty reported value never
blanks a stored one).

**AC-6** — WHEN a frame reports a subagent whose `parent_tool_call_id` is
non-empty, THEN the row SHALL be written with that `parent_tool_call_id`, and
SHALL NOT be suppressed for being nested.

### Nulls, zeroes, and absence

**AC-7** — WHEN an agent does not report `total_tokens`, `tool_use_count`, or
`duration_ms`, THEN the corresponding column SHALL be NULL. The system SHALL NOT
write `0` for an unreported value. (This is not hypothetical: 190 of 253
observed rows report none of the three.)

**AC-7a** *(added by revision 1)* — **How "did not report" is detected**, which
differs by field and is NOT uniform:

- `tool_use_count` is `ToolUseCount *int` on `streams.SubagentTaskPayload`. A nil
  pointer is "not reported" (NULL); a non-nil pointer to 0 is a reported zero
  (AC-8). The distinction is carried by the type.
- `total_tokens` and `duration_ms` are `TotalTokens int64` and `DurationMs int64`
  on the same struct — **plain values, not pointers**, both `json:"…,omitempty"`.
  They cannot express the distinction: an agent that reported 0 and an agent that
  reported nothing both arrive as `0`. THEREFORE, for these two fields
  specifically, a reported value of exactly `0` SHALL be stored as **NULL**.

The accepted cost is named rather than hidden: a subagent that genuinely consumed
zero tokens, or completed in under a millisecond, is recorded as unmeasured rather
than as a measured zero. That is the correct direction of error here. 190 of 253
observed invocations are `async_launched` dispatches that arrive as `0` for all
three fields and are known non-measurements; storing those zeroes would drag every
future token-per-subagent average down by roughly a factor of four (see § *The one
shape a builder must not get wrong*), whereas losing a true zero costs one row's
worth of precision on a quantity that is zero anyway. A builder SHALL NOT "fix"
this by inferring absence from a sibling field, by reading `agent_status`, or by
adding a pointer to the payload — the last is new instrumentation and out of scope.

*(revision 2)* **A later frame reporting `0` SHALL NOT blank a stored non-NULL
`total_tokens` or `duration_ms`.** Because this criterion defines `0` as "not
reported" for these two fields, such a frame is an *absent* value in AC-4's terms, and
AC-4's rule applies unchanged: an absent value never blanks a value already learned.

This is spelled out because the chain that establishes it runs through three criteria
and AC-5's exception table calls itself exhaustive — so a builder coding the upsert's
`SET` list from that table alone would write NULL over a real measurement, and AC-12
explicitly contemplates the later frames that would trigger it. The zero rule is now
a row in that table. The outcome: a completed subagent's token count, once learned,
survives every subsequent frame.

**AC-8** *(citation corrected in revision 1)* — WHEN an agent reports
`tool_use_count` as exactly `0` — that is, `SubagentTaskPayload.ToolUseCount` is a
non-nil pointer whose value is 0 — THEN the column SHALL be `0`, distinguishable
by query from an unreported count.

The earlier wording cited "the payload's `ToolUseCountKnown`". That field exists,
but on `SubagentTaskResult` in the ACP adapter, not on
`streams.SubagentTaskPayload` — which contradicted this document's own
§ *The payload as parsed*. The observable outcome is unchanged.

**AC-9** — WHEN a reported numeric value for `total_tokens`, `tool_use_count`,
or `duration_ms` is negative, THEN the system SHALL store NULL for that field,
SHALL increment the `anomalous_value` writer counter, and SHALL leave every
other field of the frame unaffected. A negative count is not a measurement.

*(revision 3)* **A negative reported value SHALL NOT blank a stored non-NULL
`total_tokens`, `tool_use_count` or `duration_ms`.** Because this criterion's own
premise is that a negative is not a measurement, such a frame is an *absent* value in
AC-4's terms and AC-4's rule applies unchanged: an absent value never blanks a value
already learned. "Stored NULL" governs the column's value when nothing better is known —
it is not a licence to overwrite something better that is. The counter still increments,
because the frame really did carry a negative and that is what `anomalous_value` counts.

This is the same rule AC-7a gives the reported zero, and it is spelled out for the same
reason: AC-5's exception table calls itself exhaustive, so a builder coding the upsert's
`SET` list from the row above would write NULL over a real measurement. The two rows now
read alike. It matters because AC-12 explicitly contemplates frames arriving after
settlement, so the destructive frame is an ordinary late frame rather than a corner case:
without this clause one malformed `-1` permanently erases the token count of a completed
subagent, which is the single measurement this feature exists to produce.

**AC-10** — WHEN a subagent is never observed reaching a terminal `tool_status`,
THEN `agent_status` and `tool_status` SHALL hold their last observed values (or
NULL if none was ever reported) and `settled_at` SHALL be NULL. The row SHALL
still exist. (5 observed rows are in exactly this state.)

**AC-10a** — WHEN a frame reports an `agent_status` value Kandev does not
recognise, THEN it SHALL be stored verbatim and SHALL NOT affect `settled_at`.
`agent_status` has an open, provider-defined vocabulary.

### Terminality and ordering

**AC-11** *(enumeration corrected in revision 1)* — WHEN a frame's **ACP
`tool_status`** is first classified terminal by `isTerminalToolStatus`, THEN
`settled_at` SHALL be set to that observation time and SHALL NOT be modified by
any later frame.

The terminal set is exactly these **six** values: `complete`, `completed`,
`success`, `error`, `failed`, `cancelled`. An earlier draft enumerated only
`completed`, `failed`, `cancelled`, which was a strict subset of what the cited
function actually accepts — a builder coding that parenthetical would leave a
subagent whose tool status is `success`, `complete`, or `error` permanently
unsettled, with AC-10 then declaring that state correct and hiding the defect.
Where the function and any enumeration in this document disagree, **the function
is authoritative**; the list is reproduced here so the contract is readable, and
a test SHALL pin the two together so a future edit to one fails the other.

`agent_status` SHALL NOT be used to decide terminality — in particular,
`async_launched` is a Claude *agent* status meaning "dispatched, no usage will
follow", and its own tool call is terminal in the ACP sense, so those rows
settle via `tool_status` like any other.

**AC-12** — WHEN a non-terminal frame arrives for a row whose `settled_at` is
already set, THEN `settled_at` SHALL remain set, `tool_status` SHALL remain the
terminal value, and the frame MAY still fill columns that are currently NULL.
(Out-of-order and duplicate frame delivery is an observed reality on this
stream.)

*(revision 3)* **Settlement freezes exactly two columns — `tool_status` and
`settled_at` — and nothing else. Every other column continues to follow AC-5 and AC-4
after settlement, exactly as before it.** The "MAY still fill columns that are currently
NULL" clause above is a *permission*, granted so a builder does not read the freeze as
covering the whole row; it is not an exhaustive statement of what a post-settlement frame
does, and it does not narrow AC-5.

Stated because the three criteria could be read two ways and the readings differ
observably on every settled subagent. AC-5's exception table is normative and lists no
post-settlement entry, so `model`, `agent_status`, `description` and the metric columns
replace as usual; AC-14 independently makes the last committed frame win for exactly
those columns. Read narrowly, though, AC-12 alone says a post-settlement frame may only
fill NULLs — under which a subagent that reports its model *after* its tool call settled
would keep a NULL `model` forever. AC-5 and AC-14 govern; this clause removes the
ambiguity rather than changing either.

**AC-13** *(amended by Amendment 1)* — WHEN subagent contexts are read for a
session or a turn, THEN the order SHALL be
`observed_at ASC, tool_call_id ASC, agent_execution_id ASC`.

The amendment invalidated this criterion's original justification and the
replacement is not cosmetic. AC-13 previously ordered by
`observed_at ASC, tool_call_id ASC` and defended `tool_call_id` as a total
tiebreak *because* it was unique within the session by construction. Under the
three-column key that is no longer true: two rows in one session may now share a
`tool_call_id` and differ only by execution, so the old ordering is
non-deterministic for exactly the collision this amendment exists to record.
`agent_execution_id` is appended as the final tiebreak, restoring totality —
the triple `(observed_at, tool_call_id, agent_execution_id)` is unique within a
session by construction, since the last two are the non-session components of
the UNIQUE key.

`id` is a generated UUID and SHALL NOT be used as a tiebreak.

### Concurrency

**AC-14** *(amended by Amendment 1)* — WHEN two frames for the same
`(task_session_id, agent_execution_id, tool_call_id)` are processed
concurrently, THEN the write SHALL be a single atomic upsert statement whose
conflict target is those three columns, and exactly one row SHALL result. The
implementation SHALL NOT use a read-then-write sequence for field merging; the
fill-forward and terminality rules are expressed inside the upsert's conflict
clause.

*(revision 1)* **What is and is not order-independent.** The write-once and
monotonic guarantees hold regardless of commit order: `settled_at` (AC-11),
terminal `tool_status` (AC-12), `turn_id` (AC-2a), `source` and `observed_at`
(AC-1), `agent_execution_id` (AC-30), sticky `is_async`, and AC-4's rule that an
absent value never blanks a stored one. Those are the invariants this AC binds.

For the remaining columns — the ones AC-5 governs — the **last committed frame
wins**, and that outcome is by definition commit-order dependent. An earlier
draft said the result held "regardless of which frame commits first", which was
not achievable and not intended: two frames each reporting a different non-empty
`model` cannot both win. Last-write-wins is correct here because frames for one
tool call carry progressively more complete data, so the later frame is the
better observation. Stated explicitly so a builder does not attempt to invent an
ordering discipline — a max-of, an observation-time comparison, or a
terminal-frame priority — that this contract does not ask for.

Two callers on the same row remain a single-row outcome, as before. What
changed is which frames are "the same row": two frames sharing a
`tool_call_id` but differing in `agent_execution_id` are now two independent
inserts that do not conflict, do not merge, and do not order against each other
(AC-15 governs them).

**AC-15** *(amended by Amendment 1; corrected in revision 1)* — WHEN two frames
whose keys differ in `tool_call_id` **or** in `agent_execution_id` are processed
concurrently (the observed 3-, 4-, 5- and 8-way fan-outs, and the cross-execution
collision of AC-32), THEN both rows SHALL be written, and neither SHALL be
merged into, overwritten by, or lost to the other.

*(revision 1)* The original wording said "neither SHALL block the other", which
is not satisfiable and is now removed. Kandev's SQLite writer runs on a single
connection (`internal/db/sqlite.go`, `SetMaxOpenConns(1)`), so concurrent writes
**do** serialize by design; blocking is the mechanism, not a defect. The
observable contract is the one stated above: two rows exist afterwards, each
carrying its own frame's data. Nothing in this criterion should be read as a
requirement about write parallelism.

**AC-16** — WHEN the parent session row is deleted, THEN its
`task_session_subagents` rows SHALL be removed by the foreign key cascade,
consistent with `task_session_messages` and `task_session_turns`.

### Migration

**AC-17** — WHEN `runMigrations()` runs against a database that predates this
feature, THEN the table, its indexes and its constraints SHALL be created, and
existing data SHALL be unaffected.

**AC-18** — WHEN `runMigrations()` is invoked twice in succession against the
same database, THEN the second invocation SHALL succeed and SHALL leave the
schema and the row set unchanged. (Mirrors
`task_external_id_migration_test.go`: seed a pre-migration row, call
`runMigrations` twice, assert idempotence.)

**AC-19** — WHEN the migration runs against PostgreSQL, THEN it SHALL apply and
replay with the same result as SQLite, using `internal/db` error classification
rather than local error-string matching, per ADR 0027.

**AC-20** — WHEN a statement in this migration fails, THEN — because the
repository's migration runner swallows errors
(`internal/db/migratelog.go:33`) — the failure SHALL be observable as a `WARN`
log carrying the migration's name, and the writer's health signals (AC-24,
AC-25) SHALL make the resulting absence of rows detectable rather than silent.

### Backfill

**AC-21** *(amended by revision 1; sources named in revision 2)* — WHEN the
migration runs, THEN it SHALL insert one row with `source = 'backfill'` for every
existing `task_session_messages` row whose `metadata.normalized.kind` is
`subagent_task` **and whose `task_session_id`, `task_id` and
`metadata.tool_call_id` are all non-empty**.

*(revision 2)* **Every column's source is named below.** Revision 1 said each column
derives "from `metadata.normalized.subagent_task` and `metadata.tool_call_id`", which
is true of most columns and **false of two** — and in both cases the two readings a
builder could pick produce observably different databases:

| Target column | Source on `task_session_messages` |
|---|---|
| `task_session_id`, `task_id` | the row's own columns of the same name |
| `turn_id` | *(revision 2)* the row's own **`turn_id` column**, empty string normalized to NULL |
| `tool_call_id` | `metadata.tool_call_id` |
| `parent_tool_call_id` | `metadata.parent_tool_call_id` |
| `agent_status` | `metadata.normalized.subagent_task.status` |
| `tool_status` | *(revision 2)* the **top-level `metadata.status`** |
| `subagent_type`, `description`, `agent_id`, `child_session_id`, `model`, `is_async`, `total_tokens`, `tool_use_count`, `duration_ms` | the same-named fields under `metadata.normalized.subagent_task` |
| `observed_at` | the row's `created_at` column |
| `settled_at` | the row's `updated_at` column, per AC-23a |
| `agent_execution_id` | not derivable; the `'unknown'` sentinel per AC-31 |

**`turn_id` comes from the message's own column, not from the JSON.** There is no
`turn_id` under either JSON path, so a builder following revision 1 literally finds
nothing and writes NULL for every backfilled row — silently discarding the historical
fan-out-per-turn population, including the 42 three-way turns this document's *Why*
section uses as the entire justification for the feature. The column is populated
from the same turn id the orchestrator resolves when it writes the message, which is
the same value the live path records.

**`tool_status` comes from the top-level `metadata.status`, and `agent_status` from
`metadata.normalized.subagent_task.status`. They are different fields and must not be
conflated.** This is the distinction § *Two statuses* exists to defend, and on the
backfill path revision 1 left it to a coin flip: the only status the spec named lives
under `normalized.subagent_task`, so one builder maps that onto `tool_status` —
which AC-11 flatly forbids, and which would wrongly settle the ~57 `completed` rows —
while another finds no ACP tool status at all and leaves `tool_status` and
`settled_at` NULL for all 253 rows. The correct source is the top-level key, which
the message writer populates from the frame's ACP tool status: the identical value
`isTerminalToolStatus` classifies on the live path, in the same closed vocabulary
AC-11 enumerates. A builder SHALL NOT substitute `agent_status` for it under any
circumstances.

**AC-21b** *(added by revision 2)* — WHEN two or more qualifying messages share the
same `(task_session_id, metadata.tool_call_id)`, THEN the backfill SHALL derive the
row from the message with the greatest **`created_at`**, tiebroken by the greatest
**`id`**, and SHALL insert exactly one row for that key.

Nothing constrains `task_session_messages` to be unique on that pair — `tool_call_id`
lives inside a JSON document, so no database constraint can express it, and this
document's own *Sampled input shapes* notes that the observed uniqueness "is a
property of this sample, not a guarantee." AC-22's `ON CONFLICT DO NOTHING` keeps the
statement from failing but names no winner, and AC-23a mandates a single
`INSERT … SELECT` with no `ORDER BY`. Without a named tiebreak, which message supplies
`observed_at`, `settled_at` and every derived column is non-deterministic between two
correct implementations and between two replays of the same one. Newest-first is the
right direction because a later message carries the more complete tool-call state;
`id` is named as the second key only to make the order total, and both are real
columns on the source table.

**AC-21a** *(added by revision 1)* — WHEN a qualifying message is missing any of
those three identities, THEN the backfill SHALL skip it: no row, and the
statement SHALL NOT fail.

The filter is stated explicitly because it is not implied by the column types and
the two readings differ observably. `task_session_messages.task_id` is declared
`TEXT DEFAULT ''` while `task_session_subagents.task_id` is `NOT NULL`, and `''`
satisfies `NOT NULL` in both dialects — so an unfiltered `INSERT … SELECT` would
happily write a `task_id = ''` row rather than failing loudly. Such a row is
exactly what AC-2 refuses on the live path, and for the same reason: a
session-scoped row with no card cannot be joined to anything this feature exists
to answer. The backfill and the live path SHALL agree on which frames are
recordable. These skips are part of AC-28's named allowance on the shortfall
side; they do not increment `skipped_no_identity`, which is a live-path counter.

**AC-22** *(amended by Amendment 1)* — WHEN the backfill encounters a
`(task_session_id, agent_execution_id, tool_call_id)` that already has a row,
THEN it SHALL leave the existing row untouched (`ON CONFLICT DO NOTHING`), so a
live write always wins over a backfilled one and a replay is a no-op. Because
the backfill always writes the `'unknown'` sentinel (AC-31) and that sentinel is
shared with pre-amendment live rows, a backfill replay conflicts with its own
prior rows and a backfill never duplicates a subagent already captured live
without an execution id.

WHEN a subagent was captured live **with** a real execution id and the backfill
later derives a row for the same `(task_session_id, tool_call_id)`, THEN the
keys differ and both rows SHALL exist. This is a named, accepted consequence of
execution-scoping rather than an oversight: the two rows are distinguishable by
`source`, AC-25 already forbids comparing `backfill` and `live` rows
like-for-like, and the alternative — matching a backfilled row onto a live row
by a two-column prefix — would reintroduce exactly the cross-execution
clobber this amendment removes. The backfill is a one-time statement, guarded
*(revision 4)* by the presence of **both** backfill keys (AC-24d), so this overlap is
bounded to installations that ran live capture before their backfill.

**AC-23** — WHEN the backfill derives a value, THEN AC-7 through AC-9 and the
empty-string-to-NULL rule SHALL apply identically: an absent JSON field becomes
NULL, never `0` and never `''`.

**AC-23a** *(source clarified in revision 2)* — The backfill SHALL be a single
unbounded `INSERT … SELECT` with no `LIMIT` and no batching, and SHALL set
`settled_at` from the message's `updated_at` when the derived `tool_status` is
terminal, else NULL. **"The derived `tool_status`" is the value AC-21 names — the
top-level `metadata.status` — classified against AC-11's six terminal values.** It is
never `agent_status`. It performs
one full scan of `task_session_messages` at first boot after upgrade — a real,
accepted one-time cost on a large store (the reference store is 328 MB), taken
once because a partial or resumable backfill would produce exactly the
mid-series discontinuity AC-25 exists to prevent.

**AC-23b** — WHEN the backfill runs on PostgreSQL, THEN it SHALL use the
dialect-aware JSON helpers already present in `base_migrations.go`
(`jsonColumn`, `jsonText`), which normalise empty and `'null'` metadata
documents so a single malformed row cannot abort the statement.

### Activation point and provenance

**AC-24** *(amended by revision 1)* — WHEN a migration completes **successfully**
for the first time on an installation, THEN `kandev_meta` SHALL hold the
corresponding key below, written once and never overwritten.

"Successfully" means the statement executed without error — **not** that it was
attempted. This has to be said because the repository's migration runner swallows
errors (AC-20): "the migration ran" and "the migration worked" are different
facts on this codebase, and only the second one may publish an activation point.

| Key | Written by | Value |
|---|---|---|
| `subagent_context_capture_since` | the AC-21 backfill migration | An instant sampled **inside** the backfill's transaction, at or after the AC-21 `INSERT` and before the key writes (point 4 below), from which `live` capture is authoritative. |
| `subagent_context_backfill_through` | the AC-21 backfill migration | The `created_at` of the newest message **matching AC-21's predicate at that same instant**, or the empty string when no message matched. |
| `subagent_context_execution_since` *(added by revision 1)* | the AC-33b end-state check | The instant at which the AC-33 end state was **observed to hold**, i.e. the point from which `agent_execution_id` carries a real execution identity on the live path. |

**Five things about that table are load-bearing.** Points 1–3 *(revision 2)* were wrong or
absent in revision 1; points 4–5 *(revision 4)* concern the key as a **written value** —
where its instant comes from, and what happens to one that already exists.

**1. `backfill_through` is derived from the SOURCE PREDICATE, not from what one
attempt inserted.** Revision 1 defined it as "the newest message the backfill
*inserted a row for*". That is wrong on a retry, and AC-24a makes retries a required
behaviour: the second run's `ON CONFLICT DO NOTHING` (AC-22) inserts nothing, so the
literal reading publishes the **empty string** — which AC-25 tells consumers means
*nothing before this point has known fan-out*, discarding a backfill that in fact
succeeded. Deriving it from the predicate instead is correct on the first run and on
every retry, and it is self-healing: whatever subset a prior attempt managed to
insert, the key still names the true high-water mark of what the backfill covers.

**2. Both keys are written only if the INSERT succeeded, and neither is written if
the other cannot be** (AC-24d). AC-24a already forbids writing a key after a failed
backfill; revision 1 did not say the three writes relate to each other at all.

**3. The keys carry nanosecond precision, and the comparison is defined at
equality.** All three values SHALL be RFC3339 with nanosecond precision
(`time.RFC3339Nano`), not second precision. They are compared against `observed_at`,
which is at the sibling session tables' sub-second precision, so a second-precision
key makes up to a second's worth of rows unclassifiable — and under AC-25 and AC-31
each such row is either a harmless legacy row or a live regression, which is not a
distinction to leave to rounding. The comparison is: a row is **at or after** a key
when `observed_at >= key`, and **before** it when `observed_at < key`. Equality
therefore lands on the *after* side, deterministically, in every criterion that
classifies rows by these instants.

*(revision 3)* Nanosecond precision is a requirement on the **value written**, and the
shipped pre-amendment writer does not meet it: `subagent_context_capture_since` is
written `time.Now().UTC().Format(time.RFC3339)` and `subagent_context_backfill_through`
via the repository's `rfc3339Timestamp` helper, whose SQLite form is
`strftime('%Y-%m-%dT%H:%M:%SZ', …)` — **both second precision**. `rfc3339Timestamp` is
the helper a builder reaches for and it cannot express this criterion; a builder SHALL
NOT use it for these keys. Precision alone is not sufficient, however — see AC-24e,
which governs how the value is compared, and without which the precision here is
decoration.

**4. `capture_since` is SAMPLED; it is not read off the commit** *(revision 4)*. The
commit instant is unobtainable from inside the transaction AC-24d requires — it is known
only *after* commit, when an atomic key write is no longer possible — so this criterion
cannot mean the literal commit, and revision 3 left a builder to invent which nearby
instant it did mean. It means this: `capture_since` SHALL be an instant sampled by the
backend process **inside the backfill's transaction, at or after the AC-21
`INSERT … SELECT` completes and before the key writes**. It SHALL NOT be sampled at
transaction start.

The direction matters, and it is the **opposite** of `backfill_through`'s. `capture_since`
publishes "from here on, absence of a row is absence of a subagent" (AC-25), so an instant
that is too EARLY over-claims completeness across a window nothing covered, while one that
is too LATE merely leaves a window disclaimed — which AC-25 already handles and keeps the
rows for. Sampling at transaction start therefore over-claims by the entire scan: AC-23a's
full scan of a 328 MB store, so seconds or minutes, not microseconds. Sampling after the
`INSERT` bounds the over-claim to the commit itself, and that residual is accepted and
disclosed here rather than engineered away, because closing it would mean writing the key
outside the transaction and giving up AC-24d.

**5. A key already present is accepted at whatever precision it carries** *(revision 4)*.
Point 3's precision requirement binds the key writes **this build performs**. The
pre-amendment build already activated some installations, writing `capture_since` via
`time.Now().UTC().Format(time.RFC3339)` and `backfill_through` via `rfc3339Timestamp` —
both second precision, both measured. Those keys SHALL NOT be rewritten, repaired, or
"upgraded": they are write-once (§ *Persistence guarantees*), and moving a published
boundary is a worse failure than the precision it would fix, because a consumer that
already read the old value has no way to learn it changed. The residual on such an
installation is up to one second of boundary ambiguity, in the direction AC-25 already
governs, and it is a **disclosed limitation, not a defect to close**. *Verification* scopes
the precision assertion to keys this build writes, for the same reason.

The four combinations these keys can present in are enumerated in AC-24c. The empty
value `backfill_through` takes when nothing matched is governed by AC-24f.

**AC-24a** *(added by revision 1)* — WHEN the AC-21 backfill statement fails,
THEN NEITHER `subagent_context_capture_since` NOR
`subagent_context_backfill_through` SHALL be written, the failure SHALL be
logged at `WARN` per AC-20, and the backfill SHALL be attempted again on the
next boot.

Writing an activation key after a failed backfill is specifically forbidden, and
it is forbidden for two compounding reasons rather than one. First, the key
publishes a recovery boundary that never happened: AC-25 tells consumers that
activity after `subagent_context_backfill_through` has known fan-out, so the key
would assert coverage over exactly the window it exists to disclaim. Second, the
backfill keys are *also* the guard that makes the backfill one-time (AC-23a) — writing
one after a failure permanently suppresses the retry, so the gap can never close on its
own. The runner's swallow-plus-`WARN` contract means a failure does not abort boot; it
does **not** mean the failure is invisible to the key write.

*(revision 4)* This is why AC-24d makes the guard **both** keys rather than
`capture_since` alone. Under the single-key guard the hazard above is not merely
hypothetical: the pre-amendment build writes `capture_since` in its own statement, so a
`backfill_through` write that fails afterwards leaves the retry suppressed forever by a key
that was written honestly. The two-key guard closes that without weakening this criterion —
neither key is written after a *failed insert*, and a pair that is nonetheless incomplete is
completed on the next boot.

**AC-24b** *(added by revision 1)* — WHEN `subagent_context_execution_since` is
absent while `subagent_context_capture_since` is present, THEN a consumer SHALL
read that as "this installation captures subagent contexts but has not
successfully applied Amendment 1", and SHALL NOT interpret any
`agent_execution_id` value on that installation as an execution identity. This is
the durable, queryable signal that the amended shape is not in place (AC-33a).

*(revision 2)* Read precisely, the key's absence means **the AC-33 end state was not
observed to hold** — not merely that a rebuild did not fire. Those were conflated in
revision 1, and the conflation is what made this criterion fire on healthy
installations: a fresh database reaches the end state with no rebuild at all, so
under the old reading it was permanently and wrongly declared un-amended. AC-33b
now writes the key off the observed end state, which is what makes this criterion's
absence-reading true.

**AC-24c** *(added by revision 2)* — **All four states of the two consumer-facing
keys are defined**, because AC-24a and AC-33a can each produce a state revision 1 did
not describe. WHEN a consumer reads `kandev_meta`, THEN:

| `capture_since` | `execution_since` | Meaning, and what a consumer SHALL do |
|---|---|---|
| present | present | Fully activated. AC-25's disclosure rules apply as written. |
| present | absent | Capture works; Amendment 1 has **not** successfully applied (AC-24b). No `agent_execution_id` on this installation may be read as an execution identity. |
| absent | present | The execution dimension is trustworthy, but the **backfill has not succeeded** (AC-24a) and no coverage boundary exists. The consumer SHALL treat **all** history as unknown fan-out, and the AC-28 health comparison SHALL NOT be run (see AC-28's absent-key branch). The backfill retries on the next boot. |
| absent | absent | The feature has not successfully activated on this installation. Rows MAY exist from live capture, and each one is a valid measurement, but **no** absence is evidence of anything and no boundary may be published. |

A consumer SHALL NOT infer any of these states from the presence or absence of rows.
The keys are the signal precisely because rows can exist in all four states.

*(revision 3)* **`subagent_context_backfill_through` is deliberately not a dimension of
this table.** AC-24d binds it to `capture_since` — both are written or neither is — so it
adds no state beyond the two columns above. *(revision 4)* That binding holds for pairs
this build writes; a partial pair predating it **is** reachable on the installed base, and
AC-24d **repairs** it on the next boot rather than adding it here as a fifth state a
consumer would have to branch on. A consumer therefore still sees only these four. What it *does* add is a value dimension: when
present it is either an instant or the empty-string sentinel, and those two readings are
governed by AC-24f. A consumer that has resolved a row of this table still has to branch
on that.

**AC-24d** *(added by revision 2)* — WHEN the backfill migration runs, THEN the
AC-21 `INSERT … SELECT` and both of its `kandev_meta` key writes SHALL be atomic
with respect to each other: either the insert commits and both keys are written, or
neither key is written. A state in which rows were inserted but only one key exists
SHALL NOT be an allowed outcome.

This is stated because revision 1 required each key not to be written after a
failure (AC-24a) but never related the three writes to one another, and the partial
state is the one that corrupts a published boundary rather than merely delaying it.
Note that atomicity alone would not have been sufficient: even with all three writes
in one transaction, a retry recomputes `backfill_through`, which is why AC-24 also
derives it from the source predicate rather than from the rows one attempt inserted.

*(revision 3)* **The `MAX` that derives `backfill_through` SHALL be evaluated over the
same snapshot as the AC-21 `INSERT … SELECT`, or before it — never after it.** Atomicity
is not the same guarantee as a shared snapshot: on PostgreSQL's default READ COMMITTED
each statement in a transaction takes its own snapshot, so a message committed between
the insert and the `MAX` advances the published high-water mark past a row the backfill
never inserted. The key would then assert coverage over exactly the window it exists to
disclaim, and AC-25 tells consumers that activity after it has known fan-out. The
asymmetry is what makes the rule cheap: evaluating the `MAX` first can only
**under-claim** — the key names an instant the backfill definitely covered, and AC-25's
guarantee stays true — whereas evaluating it afterwards can over-claim, which is
unrecoverable because the key is write-once. The shipped pre-amendment writer takes it
afterwards, as a third independent `r.migrate.Apply` with no enclosing transaction, so
this is a change to real behaviour and not a restatement.

*(revision 4)* **A partial key pair that predates this rule is REPAIRED by the activation
guard, not left standing.** This criterion forbids the state going forward, but it says
nothing about the installed base — and the pre-amendment build makes the state reachable
there today. It applies the `INSERT` and the two key writes as three independent
`r.migrate.Apply` calls with no enclosing transaction, and it guards re-runs on
`subagent_context_capture_since` **alone** (`subagentContextBackfillActivated`,
`base_migrations.go`). So an installation can already hold exactly one of the two, and the
`capture_since`-only case is permanent: the guard suppresses the retry on every subsequent
boot and `backfill_through` is never written. THEREFORE the backfill's activation guard
SHALL be "**both** backfill keys present", never `capture_since` present.

That one change repairs both partial states and introduces no new mechanism, because every
part of the repair is already required elsewhere: the guard does not skip; the
`INSERT … SELECT` is a no-op via AC-22's `ON CONFLICT DO NOTHING`; `backfill_through` is
recomputed from AC-21's source predicate rather than from what this attempt inserted
(AC-24 point 1), so it lands on the true high-water mark; and whichever key is already
present is preserved by its own `ON CONFLICT DO NOTHING`, keeping AC-24's write-once. The
cost is one extra full scan on one boot per affected installation, after which both keys
are present and the guard skips exactly as before. Note this is the same self-healing
property AC-24 point 1 was introduced for — it is being applied to a second state rather
than added as a new behaviour.

**AC-24e** *(added by revision 3)* — **Every comparison between an activation key and a
timestamp column SHALL be a timestamp comparison, not a string comparison.** The key
SHALL be parsed and bound as a timestamp value of the column's type, or both sides SHALL
be normalized to one lexical form before comparing. A raw key string SHALL NOT be
compared against a timestamp column's stored form. This governs every criterion and
query that classifies rows against these instants — AC-25, AC-28, AC-31, and the AC-28
health queries as written.

This is a measured defect in the shipped writer, not a hypothetical. The two sides are
in different formats and always have been:

| Side | Real value | Produced by |
|---|---|---|
| Column (`observed_at`, `created_at`) | `2026-07-30 01:39:37.271235+00:00` | the driver's `TIMESTAMP` storage form — space separator, microseconds, `+00:00` offset |
| Key (`kandev_meta.value`) | `2026-07-30T01:39:37Z` | `time.RFC3339` / `strftime('%Y-%m-%dT%H:%M:%SZ', …)` — `T` separator, `Z` suffix |

Compared as text, the two agree on the first ten characters and then diverge at the
separator, where `' '` (0x20) sorts before `'T'` (0x54). **The boundary therefore snaps
to midnight of the key's date: every row on that calendar day reads as *before* the key
whatever its actual time.** Evaluated in SQLite against the real formats:

```sql
SELECT '2026-07-30 01:39:37.271235+00:00' >= '2026-07-30T01:39:37Z';  -- 0  (same instant)
SELECT '2026-07-30 23:59:59.999999+00:00' >= '2026-07-30T00:00:00Z';  -- 0  (23h59m AFTER)
SELECT '2026-07-30 01:39:37.271235+00:00' >= '2026-07-29T23:59:59Z';  -- 1  (previous day)
```

Only the date part does any work. That is up to a full day of rows landing on the wrong
side of a boundary whose entire purpose is to separate a harmless legacy row from a live
regression (AC-25, AC-31) — and it silently truncates AC-28's `:since` window by up to a
day, in the direction that hides missing rows. The nanosecond requirement in AC-24
addresses rounding at the boundary and is necessary, but it is not sufficient and does
not touch this: two values can both carry nanoseconds and still compare wrongly if one is
a string in the other's non-format.

*(revision 4)* **What the comparison has to ACHIEVE, and the helper that does not achieve
it.** Revision 3 said what the comparison must not be — a raw string comparison — without
saying what it must reach, which left the obvious mechanism looking compliant. The
comparison SHALL resolve at **no coarser than the compared column's own storage precision,
which is microseconds, and SHALL be monotonic across the whole comparable range.**

The repository's existing helper does not meet that bar, and it is precisely the one a
builder reaches for here, exactly as `rfc3339Timestamp` was on the write side:
`timestampColumn` (`base_migrations.go`) renders SQLite's side as `julianday()`, a
double-precision day count whose ULP at this epoch is roughly 40 µs, so two values tens of
microseconds apart compare **equal**. A builder SHALL NOT use `julianday` for these
comparisons. Normalizing both sides to one fixed-width lexical UTC form, or binding the key
as a native timestamp on PostgreSQL, both clear the bar; beyond that the mechanism is
Build's. *Verification* requires a one-microsecond boundary assertion specifically because
the same-day test above passes against a `julianday` implementation, and a criterion whose
own test cannot fail the mechanism it forbids is not yet a criterion.

**AC-24's RFC3339Nano requirement is a FLOOR, not a claim that the extra digits are
reachable.** A key carrying sub-microsecond digits never compares exactly equal to a
microsecond column value, so AC-24's "equality lands on the *after* side" convention
applies vacuously to such a key. That is harmless — the boundary is still exact to the
microsecond, which is the precision the data actually has — and it is not a licence to
write a coarser key.

**AC-24f** *(added by revision 3)* — **WHEN no message matched AC-21's predicate, THEN
`subagent_context_backfill_through` SHALL hold the empty string, which is a sentinel and
not a timestamp.** It is explicitly exempt from AC-24's RFC3339Nano requirement, SHALL
NOT be parsed or compared as an instant under AC-24e, and SHALL be read by consumers as:
*the backfill matched nothing, so no pre-capture history is claimed as covered and none
needs to be — there was none to recover.* A consumer SHALL NOT read it as "no boundary is
known" and SHALL NOT, on the strength of it, treat post-`capture_since` history as
unknown fan-out.

This has to be stated because it is the state of **every fresh installation** — the
majority of installations this feature will ever run on — and revision 2 left it to a
coin flip. AC-24 mandates the empty string and the shipped statement already produces it
via `COALESCE(…, '')`, while AC-24's own next paragraph requires all three values to be
RFC3339Nano, which `''` is not. AC-25's consumer rule is keyed on "activity that predates
`subagent_context_backfill_through`" and had no branch for it: compared lexically every
timestamp exceeds `''`, so one consumer concludes all history has known fan-out and
another concludes no boundary exists and distrusts everything. Those are opposite
readings of the same store. `capture_since` remains the boundary that matters on such an
installation, exactly as AC-25 already says.

**AC-25** — The published contract for consumers SHALL be: a session whose
activity predates `subagent_context_backfill_through` has **unknown** fan-out,
not zero fan-out; rows with `source = 'backfill'` are limited to what survived
in message metadata and SHALL NOT be compared like-for-like against `live`
rows without disclosing the provenance split. Absence of a row is never
evidence of absence of a subagent before `subagent_context_capture_since`.

*(extended by Amendment 1)* The same rule governs the execution dimension: a row
whose `agent_execution_id` is `'unknown'` asserts that its execution is
**unrecorded**, never that it is shared. Consumers SHALL NOT group or count
distinct executions by that column without excluding the sentinel, and SHALL NOT
read two `'unknown'` rows as belonging to one execution.

*(revision 1)* The execution dimension has its own activation point, and it is
**not** `subagent_context_capture_since`. A row whose `observed_at` precedes
`subagent_context_execution_since` (AC-24) predates Amendment 1 and has unknown
execution identity by construction; a `'unknown'` row after that instant is a
live frame that genuinely arrived without one. Consumers comparing execution
counts across that boundary SHALL disclose the split, exactly as AC-25 already
requires for the `backfill`/`live` split. Where
`subagent_context_execution_since` is absent entirely, AC-24b applies and no
`agent_execution_id` on that installation may be read as an execution identity.

*(revision 4)* **"By construction" above governs the `'unknown'` population only. A
NON-SENTINEL `agent_execution_id` is always a real execution identity, whichever side of
`subagent_context_execution_since` its row falls on**, and a consumer SHALL NOT discard or
distrust one for preceding the key.

The qualification is required because revision 3's own AC-33b introduced a state that
falsifies the unqualified reading. A boot can commit the AC-33 rebuild and then fail to
write the key — the two are not bound the way AC-24d binds the backfill pair, and the
runner swallows the error (AC-20) — so every row written across the remainder of that
boot's uptime carries a genuine UUID while preceding the key the *next* boot writes. Under
the unqualified reading a consumer would treat those real execution ids as unknown; under
revision 2's rule two paragraphs above ("SHALL NOT discard, distrust, or exclude rows
merely for preceding a key") it would keep them. Two competent consumers, opposite queries,
over exactly the population the key exists to classify.

This is the same completeness-not-validity distinction revision 2 already drew, applied to
the execution dimension: `subagent_context_execution_since` bounds **when the `'unknown'`
sentinel stops meaning "this installation had no execution dimension"**, and nothing more.
It never bounds the validity of a value that is present. The self-heal window therefore
under-claims — some genuinely-identified rows sit before the key — in exactly the direction
AC-24d's `MAX`-first rule chooses for the same reason.

*(revision 2)* **These keys bound COMPLETENESS, never VALIDITY.** A row observed
before an activation key is still a real, correct measurement of a fan-out that
happened; what the key withholds is the guarantee that *every* such fan-out has a
row. Consumers SHALL NOT discard, distrust, or exclude rows merely for preceding a
key.

This has to be said because AC-24a makes a window of exactly that shape reachable
and the earlier wording invited the wrong reading. When a backfill fails and retries
on a later boot, `subagent_context_capture_since` is stamped at the instant the
retry succeeded, so every row the **live** path captured between the failed boot and
that retry precedes the key — correct rows, on the wrong side of a boundary that
means "coverage begins here." Under the withheld-completeness reading they are kept
and counted, and only completeness is disclaimed for that window. Under the
discard-them reading a working writer's output is thrown away because a migration
was slow to succeed. The first reading is the contract.

### Writer health

**AC-26** *(amended by Amendment 1; unit defined in revision 1)* — The system
SHALL expose counters, under the existing `expvar` convention used by `routing_*`
and `subproc_*`, for: `attempted`, `persisted`, `skipped_no_identity`,
`anomalous_value`, `failed`, and `unknown_execution` (AC-31).

**The counting unit is one observed frame, not one row and not one SQL
statement.** Every recognized `subagent_task` frame that reaches the writer
increments exactly one of `attempted`'s outcomes, and a frame that merges onto an
existing row increments `persisted` exactly as a frame that creates one does.
Stated because the unit is otherwise ambiguous and the two readings differ by
roughly the fan-out's frame count:

- `attempted` = frames that reached the writer carrying a recognized payload.
- `skipped_no_identity` = of those, the ones rejected by AC-2 before any SQL ran.
- `persisted` = of those, the ones whose upsert committed. **This counts writes,
  not rows**, so `persisted` is expected to exceed `COUNT(*)` on the table by a
  wide margin (each subagent is typically observed on several frames), and the
  two SHALL NOT be reconciled against each other. AC-28's row-count comparison is
  the reconciliation; the counters are not.
- `failed` = of those, the ones whose upsert returned an error (AC-27).
- `anomalous_value` = frames carrying at least one negative metric (AC-9). A
  qualifier, not an outcome. *(revision 2)* It is counted only for frames that
  reached the write, i.e. **not** for frames AC-2 skipped: AC-2 rejects on identity
  before any metric is examined, so a skipped frame increments
  `skipped_no_identity` and nothing else, whatever its payload contained.
- `unknown_execution` = live frames written with the `'unknown'` sentinel
  (AC-31). A qualifier, not an outcome.

So `attempted` = `skipped_no_identity` + `persisted` + `failed`, exactly, while
`anomalous_value` and `unknown_execution` overlap the others freely.

**AC-27** — WHEN a persist attempt fails, THEN the system SHALL log at `WARN`
with the session id and tool-call id, SHALL increment the `failed` counter, and
SHALL NOT fail the enclosing message write, the turn, or the agent stream.
Telemetry never breaks the product path.

**AC-28** — The expected-versus-observed health check SHALL be answerable from
the database alone, with no counter access, as:

*(queries corrected in revision 1 — they now carry the activation predicate the
prose always required, and the PostgreSQL form)*

> ***(revision 3) Which formulation is normative.*** This criterion carries three
> renderings of the same idea, accumulated across revisions, and only one of them is the
> contract. **The two directed anti-joins in *Two directions, never netted* are
> normative** — those are the queries to implement, test and alarm on. The two `COUNT`
> pairs immediately below establish *what* is being compared (distinct
> `(session, tool_call_id)` keys on each side, scoped by `:since`) and are retained as
> the explanation; the filtered expected count later in this criterion shows *which*
> messages qualify, and its predicate is carried verbatim into the shortfall anti-join.
> A builder implementing the `COUNT` pair as the check has implemented a signed
> difference, which revision 3 exists to replace — see the banking argument below.
> Stated explicitly because a criterion that shows three queries and marks none of them
> authoritative is exactly the defect the stale *Failure modes* row was.
>
> ***(revision 4) Two consequences of that hierarchy.*** First, **only the normative
> anti-joins carry the `msg` CTE**; the explanatory queries below are written unwrapped for
> readability. That is a presentation choice, not an exemption — any query from this
> criterion that is actually EXECUTED SHALL normalize `metadata` the way the anti-joins do,
> because a `''` metadata row raises on both dialects (see *Two directions, never netted*).
> Second, **what Build ships for this criterion is the two anti-joins as executable queries
> exercised by tests on both dialects** — that is the surface AC-28a's "SHALL NOT be run"
> branch and AC-24c's key-state assertions act on. No production endpoint, scheduled check
> or alerting integration is in scope; wiring a monitor to the shortfall query is a
> deployment concern, and § *Out of scope* already excludes a read API.

```sql
-- SQLite. :since is kandev_meta.subagent_context_capture_since.
-- Both sides count DISTINCT (session, tool_call_id) keys (revision 2).
SELECT COUNT(DISTINCT task_session_id || ':' || json_extract(metadata,'$.tool_call_id'))
  FROM task_session_messages
 WHERE json_extract(metadata,'$.normalized.kind') = 'subagent_task'
   AND created_at >= :since;
-- versus (revision 2: DISTINCT on the execution-agnostic key, see below)
SELECT COUNT(DISTINCT task_session_id || ':' || tool_call_id)
  FROM task_session_subagents
 WHERE observed_at >= :since;
```

```sql
-- PostgreSQL: same two counts, dialect-appropriate JSON extraction.
SELECT COUNT(DISTINCT task_session_id || ':' || (metadata::jsonb #>> '{tool_call_id}'))
  FROM task_session_messages
 WHERE metadata::jsonb #>> '{normalized,kind}' = 'subagent_task'
   AND created_at >= :since;
-- versus (revision 2: DISTINCT on the execution-agnostic key, see below)
SELECT COUNT(DISTINCT task_session_id || ':' || tool_call_id)
  FROM task_session_subagents
 WHERE observed_at >= :since;
```

The `:since` predicate is part of the contract, not decoration. Without it the
comparison spans history the backfill could not see — sessions deleted before the
upgrade left no message row and no subagent row, but sessions deleted *between*
the two counts, and messages whose sessions cascaded away, skew the sides
differently — so an unscoped comparison drifts permanently and reads as writer
failure. AC-19's dialect-parity expectation applies to this check as much as to
the migration: `json_extract` does not exist on PostgreSQL, so a SQLite-only
health query is a health query that cannot run on half the supported deployments.

*(amended by Amendment 1; comparison corrected in revision 2; direction split in
revision 3)* These two counts SHALL **agree** for activity at or after
`subagent_context_capture_since`, up to the named allowances below.

*(revision 3)* **The two directions SHALL be measured separately, as directed set
differences, and SHALL NOT be reduced to one signed number.** The actionable signal is
the SHORTFALL set — expected keys with no matching row — and it is the only one that
alarms. The EXCESS set is attributed and does not alarm. See *Two directions, never
netted* below for the queries and the reason.

**Why the observed side counts DISTINCT keys rather than rows.** Tool-call
*messages* are looked up by `(session_id, tool_call_id)` only
(`getToolCallMessageWithRetry`, `internal/task/service/service_messages.go:483`), so
a cross-execution collision updates one message row, while this table correctly holds
one row **per execution** (AC-32). Counting rows therefore makes the two sides
structurally incomparable, and Amendment 1 is what introduced the asymmetry.

Revision 1 handled that by relaxing the check to
`COUNT(task_session_subagents) >= COUNT(messages)` and treating only a shortfall as
evidence of a stopped writer. *(revision 2)* **That relaxation was unsound and is
replaced.** Every cross-execution collision banks one row of permanent headroom, and
headroom absorbs later missed writes: with N banked rows the writer can stop dead and
the comparison stays "healthy" for the next N subagent messages. AC-29 promises the
comparison diverges when the writer stops while message writes continue, and under
the `>=` rule that promise was false in exactly the configuration Amendment 1 exists
to create — the hole growing in proportion to how often the defect being recorded
actually occurs.

Collapsing the observed side to `COUNT(DISTINCT task_session_id || ':' ||
tool_call_id)` removes the excess from the comparison at the source: one subagent
counts once however many executions recorded it, which is exactly one per message
row. Equality is restored as the expectation, a shortfall regains its meaning, and
AC-29 becomes true again. The `':'` separator is not required for correctness — both
operands are fixed-length UUIDs and cannot collide — but it costs nothing and removes
the question permanently.

**Both sides count distinct keys, not rows, and the symmetry is required.** The
message side can over-count for the mirror-image reason: nothing constrains
`task_session_messages` to be unique on `(task_session_id, tool_call_id)` (AC-21b),
so two messages can describe one subagent. If only the observed side were collapsed,
every such duplicate would present as a permanent one-row shortfall on a perfectly
healthy writer — the same false-alarm failure that got the `>=` relaxation introduced
in the first place. Counting distinct keys on both sides makes the comparison
symmetric: each side answers "how many distinct subagents does this table know
about", and those are the numbers that must agree.

#### Two directions, never netted *(revision 3)*

Revision 2 restored equality as the expectation but named allowances only on the
shortfall side, leaving what a persistent **excess** means undefined. It is not an
academic state: AC-27 deliberately keeps the subagent write independent of the message
write, so a failed message write with a successful upsert leaves a permanent +1; and
AC-1a puts recognition on a later `tool_call_update`, so a message created just before
`capture_since` whose recognizing frame lands just after it counts on one side only.
Under a bare "SHALL agree" such an installation reads unhealthy forever, and a check that
fires on correct behaviour gets muted — the exact failure the `>=` relaxation was
reaching for, arriving from the other direction.

**The fix is not to tolerate excess.** Tolerating it as slack in a single signed
difference would reintroduce SR19's banking hole verbatim: with N banked excess keys the
writer could stop dead and the difference would read zero for the next N subagents.
Instead the comparison is split, so neither direction can cancel the other:

```sql
-- SQLite. :since is kandev_meta.subagent_context_capture_since, bound as a timestamp
-- (AC-24e). The `msg` CTE is AC-23b's normalization: it maps NULL / '' / 'null' metadata
-- to '{}' so one legacy row cannot abort the scan (revision 4). The CTE is byte-identical
-- on PostgreSQL; only the extraction changes (AC-19).
WITH msg AS (
  SELECT m.task_session_id, m.task_id, m.created_at,
         (CASE WHEN m.metadata IS NULL OR m.metadata = '' OR m.metadata = 'null'
               THEN '{}' ELSE m.metadata END) AS meta
    FROM task_session_messages m
)
-- SHORTFALL — expected keys with no subagent row. THIS is the alarm.
SELECT COUNT(*) FROM (
  SELECT msg.task_session_id, json_extract(msg.meta,'$.tool_call_id') AS tool_call_id
    FROM msg
   WHERE json_extract(msg.meta,'$.normalized.kind') = 'subagent_task'
     AND msg.created_at >= :since
     AND COALESCE(msg.task_session_id,'') <> ''
     AND COALESCE(msg.task_id,'') <> ''
     AND COALESCE(json_extract(msg.meta,'$.tool_call_id'),'') <> ''
   GROUP BY 1, 2
  EXCEPT
  SELECT s.task_session_id, s.tool_call_id
    FROM task_session_subagents s
   WHERE s.observed_at >= :since
) AS shortfall;

-- EXCESS — subagent keys with no accounting message. Attributed, does NOT alarm.
WITH msg AS (
  SELECT m.task_session_id, m.created_at,
         (CASE WHEN m.metadata IS NULL OR m.metadata = '' OR m.metadata = 'null'
               THEN '{}' ELSE m.metadata END) AS meta
    FROM task_session_messages m
)
SELECT COUNT(*) FROM (
  SELECT s.task_session_id, s.tool_call_id
    FROM task_session_subagents s
   WHERE s.observed_at >= :since
   GROUP BY 1, 2
  EXCEPT
  SELECT msg.task_session_id, json_extract(msg.meta,'$.tool_call_id')
    FROM msg
   WHERE json_extract(msg.meta,'$.normalized.kind') = 'subagent_task'
     AND msg.created_at >= :since
) AS excess;
```

`EXCEPT` is set-based and deduplicates on both dialects, so this subsumes revision 2's
distinct-key collapse rather than replacing it: a cross-execution collision (AC-32)
contributes one key however many rows recorded it, and two messages describing one
subagent (AC-21b) contribute one key however many messages exist. The PostgreSQL form is
identical with `msg.meta::jsonb #>> '{tool_call_id}'` and `#>> '{normalized,kind}'`
substituted for `json_extract`, per AC-19.

*(revision 4)* **Three things about the SQL above are load-bearing and were wrong or
unstated before.**

**The derived tables are ALIASED, and they have to be.** PostgreSQL rejects a subquery in
`FROM` without an alias, so revision 3's `SELECT COUNT(*) FROM ( … EXCEPT … );` did not
parse on PostgreSQL at all — while the surrounding prose claimed the PostgreSQL form was
"identical with the extraction substituted", which was therefore false of the normative
queries themselves. `AS shortfall` / `AS excess` are accepted by both dialects, so the
identical-modulo-extraction claim above is now true rather than aspirational.

**Every `metadata` reference goes through the `msg` CTE, for exactly AC-23b's reason.** The
repository writes `NULL`, `''` and `'null'` for "no metadata" — that is what `jsonColumn`
(`base_migrations.go`) exists to absorb — and an unwrapped extraction raises on such a row:
`json_extract('')` is malformed JSON on SQLite and `''::jsonb` errors on PostgreSQL. A
single legacy row would therefore disable the health check on either dialect. That is a
worse failure than any it detects, because the check whose job is noticing that this writer
stopped would itself stop, silently, in the same direction. The CTE is the same
normalization AC-23b already requires of the backfill, applied to the same rows.

**The excess direction's message half deliberately does NOT carry the three identity
predicates the shortfall half carries.** This asymmetry is intentional, not an oversight.
The shortfall side asks "which messages should have produced a row", so it must exclude
exactly what AC-2 and AC-21a exclude. The excess side asks the opposite question — "is
there any message that accounts for this row" — so it must subtract the **widest** message
set available. Filtering it would report excess for a subagent row whose message exists but
whose own `task_id` column is empty for an unrelated reason, which is a real row correctly
written from a frame that did carry a task id. A builder SHALL NOT mirror the filter onto
the excess side for symmetry's sake.

**A non-zero SHORTFALL is evidence of a stopped or failing writer and is the check.**
A non-zero EXCESS is expected on any installation that has taken a message-write failure
or straddled the activation boundary; it SHALL be reported separately for attribution and
SHALL NOT be raised as writer failure, and it can never mask a shortfall because it is
counted in the other direction. Cross-execution collisions no longer appear in either
direction at all, which is what makes AC-29's guarantee unconditional.

Where a deployment wants the collision count itself — rows minus distinct subagents, the
quantity revision 2 published — it remains available and is orthogonal to both directions
above:

```sql
-- Cross-execution collisions (AC-32). Expected > 0 wherever a tool_call_id was reused.
SELECT COUNT(*) - COUNT(DISTINCT task_session_id || ':' || tool_call_id)
  FROM task_session_subagents
 WHERE observed_at >= :since;
```

**AC-28a** *(added by revision 2)* — WHEN `subagent_context_capture_since` is
**absent**, THEN the AC-28 comparison SHALL NOT be run, and the key's absence SHALL
itself be the health signal, read per AC-24c.

This branch has to exist because revision 1 created the state and left the query
undefined in it. AC-20 promises that a failed migration is made detectable by the
health signals; AC-24a requires `capture_since` to be **absent** after a failed
backfill; and AC-28's own prose disowns the unscoped comparison as one that "drifts
permanently and reads as writer failure." So in precisely the failure state AC-20
points at, the prescribed query had no value to bind to `:since` and either could not
run or had to fall back to the form the spec rejects. The resolution is that the
detection in that state is *the missing key*, not a shortfall — which is a stronger
signal anyway, since it is present from the first boot after the failure rather than
accumulating.

**AC-28b** *(added by revision 2)* — WHEN a shortfall is observed, THEN it SHALL be
confirmed by a second evaluation before being treated as evidence of a failing
writer, OR both counts SHALL be taken within a single read transaction.

AC-28's claim that a shortfall means a stopped writer "and nothing else" is not true
of two independently-executed statements against a live store: rows and messages are
written continuously and a session cascade-deleting between the two counts skews them
differently, so a healthy writer can present a transient shortfall.

*(revision 3)* The anti-join form largely retires this hazard rather than merely
mitigating it: each direction is now a **single** statement, so both of its sides are read
under one snapshot and the two-statement skew this criterion was written against cannot
arise within a direction. The confirm-on-second-evaluation rule is retained for the
residual case — a session cascade-deleting mid-statement, and any deployment whose
monitor still evaluates the two directions separately — but a builder using the queries as
written satisfies it by construction with the single-statement reading. A check that
fires on correct behaviour gets muted, and a muted check is the same as no check —
which is the outcome this feature exists to prevent.

*(revision 1)* **The shortfall side needs its own allowance, for the mirror-image
reason.** A frame carrying a session id but no task id writes its message and is
then correctly skipped by AC-2, so it produces a permanent, benign shortfall that
is indistinguishable from a dead writer if the expected count is taken naively.
The expected count SHALL therefore exclude what AC-2 and AC-21a exclude, which is
answerable from the database alone:

```sql
-- SQLite: expected count, AC-2 skips removed. Same shape on PostgreSQL with
-- metadata::jsonb #>> '{...}' substituted for json_extract.
SELECT COUNT(DISTINCT task_session_id || ':' || json_extract(metadata,'$.tool_call_id'))
  FROM task_session_messages
 WHERE json_extract(metadata,'$.normalized.kind') = 'subagent_task'
   AND created_at >= :since
   AND COALESCE(task_session_id,'') <> ''
   AND COALESCE(task_id,'') <> ''
   AND COALESCE(json_extract(metadata,'$.tool_call_id'),'') <> '';
```

With that expected count, a shortfall is evidence of a stopped or failing writer
and nothing else, which is what makes the check actionable. A health check that
fires on correct behaviour gets muted, and a muted check is the same as no check —
which is the outcome this feature exists to prevent.

The message count remains a valid independent expectation **because the message
write is
load-bearing for the UI**: a broken message write is noticed by a human within
one turn, whereas a broken context write is noticed by nobody. That asymmetry
is what makes the comparison a real check rather than two readings of the same
failure.

**AC-29** *(scope corrected in revision 2)* — WHEN the writer has stopped while
message writes continue, THEN the AC-28 comparison SHALL diverge, and the divergence
SHALL be attributable in time via `MAX(observed_at)` on `task_session_subagents`. No
separate heartbeat row is written, precisely so that the health signal cannot itself
be the thing that silently stops.

*(revision 2)* This guarantee is made **against AC-28's distinct-key count**, and it
holds only because of that. Under revision 1's `>=` comparison it was false: banked
cross-execution excess absorbed the first N missed writes, so a stopped writer
produced no divergence at all until the backlog was consumed. The distinct-key count
does not accumulate excess, so the first missed subagent is the first divergence.

*(revision 3)* **"Diverge" means the SHORTFALL direction becomes non-zero** — the first
anti-join in *Two directions, never netted*. Naming the direction is what makes this
guarantee unconditional: because the shortfall is a directed set difference rather than a
signed count, no amount of banked excess from any source (cross-execution collisions, a
failed message write, an activation-boundary straddle) can offset it, so the first missed
subagent is the first divergence on every installation rather than only on one with no
excess. A monitor SHALL alarm on the shortfall query, not on the equality of two counts.
This criterion is the reason AC-28's comparison changed, rather than the other way
round — the requirement that a stopped writer be noticed is the fixed point, and the
query is what had to move.

*(revision 4)* **The attribution half is scoped, and its empty result has a defined
meaning.** The `MAX(observed_at)` SHALL be taken over rows **at or after
`subagent_context_capture_since`** — the same `:since` and the same timestamp comparison the
shortfall query uses (AC-24e) — not over the whole table. Unscoped it returns the newest of
whatever exists, so on an installation whose only rows are backfilled or legacy it reports
an instant that never established live-writer health at all, and reads as a healthy writer.

WHEN that scoped `MAX` returns **NULL** — no row has been observed since activation — THEN
it SHALL be read as *"the writer has produced nothing since this installation activated"*,
which is the strongest available divergence signal rather than an absence of information.
It SHALL NOT be reported as "no divergence", "unknown", or a healthy state. This is the
reachable case where the very first post-activation subagent is the one missed, and it is
exactly the case a naive `MAX` reports most reassuringly.

### Execution identity *(added by Amendment 1)*

**AC-30** — WHEN the orchestrator records a subagent context from a frame whose
`ExecutionID` is non-empty, THEN the row's `agent_execution_id` SHALL be that
value verbatim, and the row SHALL be keyed on `(task_session_id,
agent_execution_id, tool_call_id)`.

**AC-31** — WHEN a subagent context is written and no execution identity is
available — the frame's `ExecutionID` is empty, or the write is the backfill,
whose source rows carry no execution identity — THEN `agent_execution_id` SHALL
be the reserved literal `'unknown'`. It SHALL NOT be NULL, SHALL NOT be the
empty string, and SHALL NOT be omitted from the insert. WHEN this occurs on the
**live** path specifically, the system SHALL additionally increment an
`unknown_execution` writer counter alongside the AC-26 counters, so that a
regression which silently stops propagating execution identity is observable
rather than merely absorbed by the sentinel.

`unknown_execution` is **additive, not exclusive**: a live frame with an empty
`ExecutionID` still increments `attempted` and, on success, `persisted`, exactly
as any other write does. It is a qualifier on a write that happened, not an
alternative outcome like `skipped_no_identity`. Stated because the counter set
otherwise reads as mutually exclusive outcomes, and a builder choosing the other
reading would break the `attempted` = `skipped_no_identity` + `persisted` +
`failed` identity that AC-26 requires.

*(revision 1)* Because expvar counters reset with the process, the counter is not
the durable half of this signal. `subagent_context_execution_since` (AC-24) is:
a `'unknown'` row observed **after** that instant is a live frame that arrived
without an execution id, and one observed before it is a legacy row that never
had the dimension. The counter tells an operator that it is happening now; the
key lets a consumer tell which population a row belongs to months later.

This mirrors AC-7's NULL-not-zero rigor for the metric columns: there, a
fabricated `0` would corrupt an average; here, a fabricated *shared* identity
would silently re-merge rows the key exists to keep apart. The sentinel is
honest about being unknown, the counter makes "unknown" countable, and AC-25's
disclosure rule extends to it — a `'unknown'` row asserts that the execution is
unrecorded, never that the rows share an execution.

**AC-32** — WHEN two executions in the same session emit the same
`tool_call_id`, and a frame from the earlier execution arrives **after** the
later execution's row exists, THEN the earlier frame SHALL create or update its
own row keyed on its own `agent_execution_id`, and the later execution's row
SHALL be unchanged in every column — including `tool_status`, `settled_at`, and
the three metric columns.

This is the defect Amendment 1 exists to close, so it is stated as an
observable outcome and not only as a key change. It is reachable in production:
late frames from completed executions are recorded rather than dropped
(`event_handlers_streaming.go:325-333`), and `tool_call_id` uniqueness is only
per-execution (*Execution identity, as sampled*). Verification requires a
regression test at the repository layer driving exactly this interleaving.

**AC-33** *(amended by revision 1)* — WHEN `runMigrations()` runs against a
database whose `task_session_subagents` table predates Amendment 1, THEN after
the migration the table SHALL have:

1. the `agent_execution_id` column, NOT NULL;
2. the three-column `UNIQUE(task_session_id, agent_execution_id, tool_call_id)`;
3. **no** remaining two-column `UNIQUE(task_session_id, tool_call_id)`;
4. all three secondary indexes — on `task_session_id`, `task_id`, `turn_id` —
   present, matching the fresh-database DDL exactly *(revision 1)*;
5. every pre-existing row surviving with all column values intact, timestamps
   included, and `agent_execution_id` set to `'unknown'`;
6. `subagent_context_execution_since` written per AC-24. *(revision 2)* The key is
   **not** conditional on this criterion's WHEN: AC-33b writes it on every
   installation where the key-bearing clauses are observed to hold, including those
   where no rebuild was ever needed. *(revision 3)* Those are clauses 1–3; clauses 4
   and 5 remain required outcomes of this criterion but do not gate the key write.

**This end state SHALL hold on PostgreSQL as well as SQLite** *(revision 1)*.
The dialect scope is stated because the two mechanisms are genuinely different
and the earlier draft's pointer covered only one of them:

- On **SQLite**, the two-column key is an inline table constraint
  (`base_schema.go`), which SQLite cannot alter in place, so the table must be
  rebuilt. `recreateTableNamed` is the repository's pattern for that.
- On **PostgreSQL**, `recreateTable` returns `(false, nil)` immediately — it is
  SQLite-only by construction. A migration wired only through it therefore
  **silently no-ops on PostgreSQL**: no error, no `WARN`, not even a "migration
  applied" line, leaving the two-column key in place. PostgreSQL needs its own
  path, which drops the auto-named constraint by looking it up in `pg_constraint`
  by its column set (names truncate at 63 bytes and cannot be hardcoded) and adds
  the new one. **The repository already owns this exact shape** — add a column and
  widen a two-column UNIQUE to three columns, on both dialects — in
  `migrateTaskEnvironmentReposAllowMultiBranch` and its `…Postgres` twin
  (`base_migrations.go`). That is the precedent to follow, not the two
  `recreateTableNamed` call sites that merely drop a column.

The mechanism beyond that is Build's to choose. Fresh databases SHALL obtain the
same shape from the schema-init DDL. Per the repository's migration rules, the
column and the key change SHALL be introduced by an idempotent migration and
SHALL NOT rely on `CREATE TABLE IF NOT EXISTS` alone, which is a no-op on an
existing database.

**AC-33a** *(added by revision 1)* — WHEN the AC-33 migration fails or does not
apply, THEN boot SHALL NOT abort, the failure SHALL be logged at `WARN`,
`subagent_context_execution_since` SHALL NOT be written (AC-24b), the table SHALL
be left in its pre-amendment shape rather than a partial one, and the migration
SHALL be attempted again on the next boot.

This is the branch the earlier draft left to a coin flip, and the two guesses had
opposite consequences, so it is settled here rather than in Build. Not aborting
boot is the correct half: this is a telemetry table, and AC-27's principle —
telemetry never breaks the product path — does not stop being true because the
failure happened in a migration rather than a write. The rebuild runs inside a
transaction, so a failure rolls back and the pre-amendment table survives intact;
a half-migrated table is not an allowed outcome.

The resulting degraded state is fully specified and **must not be silent**: the
amended upsert's three-column conflict target cannot match the surviving
two-column constraint, so every live write fails, and each one increments
`failed` and logs at `WARN` per AC-27. That state is therefore detectable three
independent ways — the missing `subagent_context_execution_since` key (AC-24b),
a climbing `failed` counter, and the AC-28 shortfall — which is the standard this
feature is held to: if this writer stops, something notices.

**AC-33b** *(added by revision 2; observation scope fixed in revision 3)* — WHEN
`runMigrations()` completes a pass in which the **key-bearing clauses** of the AC-33 end
state hold, THEN `subagent_context_execution_since` SHALL be written per AC-24 if absent,
and left unchanged if already present. This applies regardless of *how* the end state came
to hold: whether a rebuild applied it in this pass, an earlier boot applied it, or the
schema-init DDL created the table already-amended and no rebuild was ever needed.

*(revision 3)* **The key-bearing clauses are AC-33's clauses 1, 2 and 3, and only those:**
the `agent_execution_id` column exists and is NOT NULL; the three-column
`UNIQUE(task_session_id, agent_execution_id, tool_call_id)` exists; and no two-column
`UNIQUE(task_session_id, tool_call_id)` remains. Revision 2 wrote "clauses 1–5", which is
not a decidable condition and left a builder to invent one: **clause 5 is not observable
at all** — it asserts that rows which existed *before* this pass survived intact, and a
migration holds no record of the pre-state to compare against — while clause 4 (the index
set matches the fresh-database DDL) is observable but is not what this key attests to.

Clauses 4 and 5 remain **required outcomes** of AC-33 with their own verification; they
are simply not preconditions of this key write. The split is principled rather than
convenient: `subagent_context_execution_since` publishes exactly one fact — that
`agent_execution_id` on this installation carries a real execution identity (AC-24b) —
and clauses 1–3 are precisely the schema facts that make that true. A missing secondary
index is a parity and performance defect, not a reason to tell every consumer to discard
the execution dimension.

**The migration SHALL determine this by OBSERVING the live schema** — querying for
the `agent_execution_id` column and the constraint set — and SHALL NOT infer it from
whether a rebuild helper reported that it fired. `recreateTable` returns
`(false, nil)` in three materially different situations: the table does not exist,
the trigger phrase is absent because the shape is *already correct*, and the dialect
is PostgreSQL. A `fired` boolean therefore cannot separate "already correct" from
"did nothing and is still wrong", and gating a durable published key on it is gating
it on a value that does not carry the fact.

This criterion exists because revision 1 attached the key to AC-33, whose WHEN is
scoped to a table that "predates Amendment 1" — with the key write sitting inside
that scope as clause 6. Two whole classes of installation never satisfy it:

- a **brand-new database**, which AC-33 itself says obtains the amended shape from
  the schema-init DDL;
- a database that **predates the feature entirely**, where `CREATE TABLE IF NOT
  EXISTS` creates the table already-amended, after which it no longer "predates
  Amendment 1" either.

On both, the backfill still succeeded and wrote `capture_since` while
`execution_since` was never written by anything — and AC-24b then instructs every
consumer to read that combination as "has not successfully applied Amendment 1" and
to discard every `agent_execution_id` on the installation. On installations where
every row carries a genuine execution identity. The keys are write-once and no
rebuild will ever fire on them, so the mislabelling was permanent, and it applied to
the majority of installations this feature will ever run on. It inverted the durable
half of revision 1's own detection story.

*(revision 3)* **There is a fourth state, and it is the one that makes schema observation
load-bearing rather than stylistic: the key write itself failing after a successful
rebuild.** Nothing binds `subagent_context_execution_since` to the AC-33 rebuild the way
AC-24d binds the two backfill keys to their insert, and the runner swallows errors
(AC-20). So boot 1 can commit the rebuild and then fail to write the key, leaving an
installation whose schema is fully amended and whose key is absent — which AC-24b reads as
"has not successfully applied Amendment 1". Boot 2 SHALL self-heal it: the end state holds,
no rebuild fires, no `CREATE TABLE` fires, and the key SHALL be written anyway.

This is the state that separates a conforming implementation from one gated on
"rebuild fired **or** the table was created this pass" — that gate satisfies every other
case in this criterion and fails only this one. It is therefore the case a test has to
construct, which is why *Verification* now requires it explicitly rather than leaving it
implied by the SHALL-observe sentence.

**AC-33c** *(added by revision 2)* — WHEN `runMigrations()` runs, THEN the AC-33
shape change and the AC-33b key write SHALL be attempted **before** the AC-21
backfill within the same pass. AND WHEN the AC-33 end state does not hold, THEN the
backfill SHALL NOT run, SHALL NOT write either of its keys, and SHALL be attempted
again on the next boot (AC-24a).

The ordering is a contract, not an implementation detail, because the other order is
a permanent failure rather than a transient one. AC-31 requires the backfill's INSERT
to carry `agent_execution_id` and forbids omitting it. Consider an installation that
took the pre-amendment table but whose backfill *failed* — so `capture_since` is
absent and AC-24a requires a retry — and which then upgrades to the amended binary.
If the retried backfill ran first, its INSERT would reference a column AC-33 has not
yet added, the statement would fail, `capture_since` would stay unwritten, and the
identical sequence would repeat on every subsequent boot. AC-24a's retry contract is
precisely what would make that failure permanent instead of self-correcting.

**AC-34** *(amended by revision 1)* — WHEN `runMigrations()` is invoked twice in
succession against a database migrated by AC-33, THEN the second invocation SHALL
succeed, SHALL leave the schema and the row set unchanged, and SHALL NOT
duplicate, drop, or re-sentinel any row.

This SHALL hold on PostgreSQL as well as SQLite, per AC-19 and ADR 0027 — and on
**both** dialects the assertion SHALL be that AC-33's six-part end state is
present after each invocation, not merely that the second invocation changed
nothing since the first. *(revision 1)* An unchanged-only assertion is satisfied
vacuously by a migration that no-ops on PostgreSQL and never applied at all,
which is precisely the failure this criterion needs to catch.

---

## Failure modes

| Condition | Behaviour |
|---|---|
| Frame with no `tool_call_id` or no session id | No row; `skipped_no_identity`++; product path unaffected (AC-2). |
| Upsert returns an error | `WARN` + `failed`++; product path unaffected (AC-27). |
| Negative reported metric | That field NULL; `anomalous_value`++; other fields written (AC-9). |
| Frames arrive out of order | Terminality is monotonic; NULLs may still fill (AC-11, AC-12). |
| Duplicate frames | Upsert; exactly one row (AC-3). |
| Parent session deleted mid-turn | FK cascade removes rows; no orphan (AC-16). |
| Migration statement fails | Swallowed by the runner; surfaced as `WARN` + detectable via AC-28 (AC-20). Boot never aborts. |
| **Backfill statement fails** | No activation key written; `WARN`; retried next boot (AC-24a). Never publishes a boundary it did not reach. |
| **AC-33 rebuild fails or does not apply** | Pre-amendment table survives whole (transactional rollback); `WARN`; `subagent_context_execution_since` absent; retried next boot (AC-33a). |
| **AC-33 rebuild silently no-ops on PostgreSQL** | Prevented by AC-33's dialect scope and AC-34's end-state assertion; if it still occurs it presents exactly as the row above (AC-24b). |
| Live writes rejected because the rebuild left the two-column key | Every write `failed`++ and `WARN` per AC-27; product path unaffected; detectable three ways (AC-33a). |
| Backfill re-runs | `ON CONFLICT DO NOTHING`; no duplicates, no clobber (AC-22). |
| Agent reports an `agent_status` Kandev does not recognise | Stored verbatim; never affects `settled_at` (AC-10a). |
| Frame carries no task id | No row; `skipped_no_identity`++ (AC-2). |
| Same `tool_call_id` observed under a later turn | Existing row updated; original `turn_id` preserved (AC-2a). |
| A turn row is deleted | Subagent rows survive; `turn_id` has no FK, by design. |
| Same `tool_call_id` reused by a **later execution** | Two rows, one per execution; neither clobbers the other (AC-32). |
| Late frame from a completed execution after a newer execution reused its `tool_call_id` | Updates only its own execution's row; the newer row is untouched (AC-32). |
| Live frame carries an empty `ExecutionID` | Row written with `agent_execution_id = 'unknown'`; `unknown_execution`++ (AC-31). Never skipped — absence would be indistinguishable from no fan-out. |
| Backfill derives a row (source rows have no execution identity) | `agent_execution_id = 'unknown'` (AC-31); replay is a no-op via the shared sentinel (AC-22). |
| Backfill overlaps a subagent already captured live with a real execution id | Both rows exist, distinguishable by `source`; named and accepted (AC-22). |
| An execution row is deleted from `executors_running` | Subagent rows survive; `agent_execution_id` has no FK, by design. |
| Cross-execution collision *(revision 3)* | Does **not** affect the AC-28 comparison at all: both directions are `EXCEPT` set differences over `(session, tool_call_id)`, so one subagent contributes one key however many executions recorded it. Attributed by the separate collision query (AC-28, AC-32). |
| AC-2 skip deflates the AC-28 count | Expected; the expected-count query excludes the same rows, so the shortfall stays actionable (AC-28, revision 1). |
| **Two executions BOTH arrive with an empty `ExecutionID` and reuse one `tool_call_id`** | They share the `'unknown'` key and merge into one row (AC-3) — the pre-amendment clobber, still reachable when execution identity is absent from both frames. Accepted residual, not a defect to design around: the sentinel cannot manufacture a distinction the frames did not carry, and `unknown_execution` plus `subagent_context_execution_since` make the population countable and datable (AC-31). |
| Backfill row later merged onto by a live frame | Row keeps `source = 'backfill'` and its original `observed_at`; all other columns fill forward (AC-1, revision 1). |
| A message with an empty `task_id` is backfilled | Skipped, matching AC-2's live-path rule (AC-21a). |
| **Fresh database, or one predating the feature entirely** | No rebuild fires — the schema-init DDL already produced the amended shape — but the end state is observed and `subagent_context_execution_since` is written anyway (AC-33b). The installation is never mislabelled un-amended. |
| **Backfill retried after a partial failure** | `backfill_through` is recomputed from the AC-21 predicate, not from the rows this attempt inserted, so a no-op replay still publishes the true high-water mark rather than `''` (AC-24). |
| **`capture_since` absent** | The AC-28 comparison is not run; the missing key is itself the health signal, read per AC-24c (AC-28a). |
| **Live rows captured between a failed backfill and its successful retry** | Kept and counted. They precede `capture_since`, so completeness is disclaimed for that window, but validity is not (AC-25, revision 2). |
| **Two messages share one `(task_session_id, tool_call_id)`** | Greatest `created_at`, tiebroken by greatest `id`, supplies the row; exactly one row results (AC-21b). |
| **Writer stops after cross-execution collisions banked excess rows** | Divergence is immediate: the health comparison counts distinct `(session, tool_call_id)` keys, not rows, so excess cannot absorb a missed write (AC-28, AC-29). |
| **Two messages describe one subagent (AC-21b), inflating the expected side** | No false shortfall: both sides of the AC-28 comparison count distinct keys, so the duplicate collapses on the expected side exactly as it does on the observed side (AC-28). |
| **Backfill would otherwise run before the AC-33 shape change** | Prevented by AC-33c: the shape change goes first, and the backfill does not run against a stale shape. Without it, an install whose earlier backfill failed would fail identically on every boot forever. |
| **First frame resolves no turn, a later frame carries one** | `turn_id` is filled forward; "first observation wins" applies only between two non-empty turns (AC-2a, revision 2). |
| **Later frame reports `total_tokens` or `duration_ms` as `0` after a real value was learned** | The stored value is preserved — `0` means "not reported" for these two fields, so AC-4 applies (AC-7a, revision 2). |
| **Later frame reports a NEGATIVE metric after a real value was learned** *(revision 3)* | The stored value is preserved; `anomalous_value`++ still fires. A negative is not a measurement, so AC-4 applies exactly as it does to the reported zero (AC-9). |
| **An activation key is compared against a timestamp column as a raw string** *(revision 3)* | Forbidden. The boundary would snap to midnight of the key's date and misclassify up to a day of rows. The key is parsed and bound as a timestamp (AC-24e). |
| **`backfill_through` is the empty string** *(revision 3)* | The fresh-install default. A sentinel, not an instant: the backfill matched nothing, so no pre-capture history is claimed and none needs to be. Never parsed as a timestamp (AC-24f). |
| **Persistent observed-side EXCESS on a healthy writer** *(revision 3)* | Expected and reportable, never an alarm — reachable from a failed message write (AC-27 decouples the two) or an activation-boundary straddle (AC-1a). It cannot mask a shortfall because the two directions are separate anti-joins (AC-28). |
| **`MAX(created_at)` evaluated after the backfill INSERT** *(revision 3)* | Forbidden. A message committed between the two would publish coverage the backfill never reached, and the key is write-once. Evaluate it over the insert's snapshot or before it (AC-24d). |
| **Non-terminal frame arrives after `settled_at` is set** *(revision 3)* | `tool_status` and `settled_at` stay frozen; every other column follows AC-5 and AC-4 unchanged, so a late-reported `model` is still recorded (AC-12). |
| **Two frames for one key commit in reverse observation order** *(revision 3)* | `observed_at` is the creating (first-committed) frame's observation time. It is not recomputed to the earliest observation, because AC-1 makes it write-once (AC-1a). |
| **AC-33 rebuild commits but the `execution_since` key write fails** *(revision 3)* | Schema amended, key absent — an installation AC-24b would mislabel. The next boot observes the end state, fires no rebuild, and writes the key anyway (AC-33b). This is the state a `fired`-gated implementation never recovers from. |
| **`capture_since` sampled at transaction start** *(revision 4)* | Forbidden. It would over-claim completeness across the whole AC-23a scan — seconds or minutes on a large store. Sample inside the transaction, at or after the `INSERT` (AC-24 point 4). The commit instant itself is unobtainable there and is not what the key means. |
| **A row carrying a REAL UUID `agent_execution_id` observed BEFORE `execution_since`** *(revision 4)* | Its execution identity is real and is read as real. `execution_since` dates only the `'unknown'` population; it never bounds the validity of a value that is present (AC-25). Reachable from AC-33b's fourth state, so this is ordinary, not exotic. |
| **An activation key already present at second precision** *(revision 4)* | Accepted as-is and never rewritten — write-once outranks the precision mandate, and moving a published boundary is worse than the second of ambiguity it would fix. A disclosed limitation of already-activated installations (AC-24 point 5). |
| **Exactly one of the two backfill keys present** *(revision 4)* | Repaired, not tolerated. The activation guard is "both keys present", so the backfill re-runs once: the INSERT no-ops via `ON CONFLICT`, the missing key is computed from AC-21's predicate, and the present key is preserved. Reachable today because the pre-amendment build wrote three independent statements and guarded on `capture_since` alone (AC-24d). |
| **`julianday()` used to compare a key against a timestamp column** *(revision 4)* | Forbidden. Its ULP at this epoch is roughly 40 µs, so values tens of microseconds apart compare equal — coarser than the column's own microseconds. `timestampColumn` is the helper that does this and SHALL NOT be used here (AC-24e). |
| **An AC-28 anti-join run on PostgreSQL without a derived-table alias** *(revision 4)* | It does not parse; PostgreSQL requires the alias. `AS shortfall` / `AS excess` are accepted by both dialects (AC-28, AC-19). |
| **A message row whose `metadata` is `''` or `'null'`** *(revision 4)* | Absorbed by the anti-joins' `msg` CTE, exactly as AC-23b absorbs it in the backfill. Unwrapped, one such row raises and disables the health check on either dialect — the check failing silently in the same direction as the writer it watches (AC-28). |
| **Scoped `MAX(observed_at)` returns NULL** *(revision 4)* | Read as "the writer has produced nothing since activation" — the strongest divergence signal, never as healthy or unknown. Reachable when the very first post-activation subagent is the one missed (AC-29). |

## Persistence guarantees

- A row, once written, is durable until its parent session is deleted.
- `settled_at` is write-once (AC-11).
- `source` is write-once: a backfilled row is never relabelled `live`, and a
  live row is never overwritten by the backfill (AC-22). *(revision 1)* This
  holds on the merge path too — a live frame landing on an existing backfilled
  row leaves `source` alone (AC-1).
- *(added by revision 1)* `observed_at` is write-once. It records when Kandev
  first observed this subagent (AC-1a), so a later frame never moves it forward;
  otherwise the column would drift toward "last seen" and AC-29's
  `MAX(observed_at)` would stop locating when a writer failure began.
- *(added by Amendment 1)* `agent_execution_id` is write-once. It is part of the
  row's identity, so a later frame cannot change it — a frame with a different
  execution id addresses a different row (AC-30, AC-32). A row that was written
  with the `'unknown'` sentinel is never later "upgraded" to a real execution
  id, because that would silently move a row between identities and defeat the
  key.
- The **three** `kandev_meta` activation keys are write-once (AC-24), and none is
  written unless the work it attests to actually succeeded (AC-24a, AC-33a).
  *(revision 2)* For the two backfill keys that means the `INSERT … SELECT`
  committed, and both are written or neither is (AC-24d). For
  `subagent_context_execution_since` it means the AC-33 end state was **observed to
  hold** — not that a rebuild fired, which is a different and much narrower fact
  (AC-33b). All three carry nanosecond precision, and every criterion that classifies
  a row against one of them uses `observed_at >= key` for "at or after" (AC-24).
  *(revision 3)* That comparison is a **timestamp** comparison — the key is parsed and
  bound, never compared as a raw string against a column's storage form (AC-24e) — and
  `subagent_context_backfill_through` additionally carries a non-timestamp sentinel
  state, the empty string, which is exempt from both the precision requirement and the
  comparison (AC-24f). `subagent_context_execution_since` is the one key with no
  atomicity partner, which is why AC-33b requires the next boot to write it when the
  end state already holds.
  *(revision 4)* Write-once is the **stronger** invariant wherever it collides with
  anything else in this document: a key already present is never rewritten, not to raise
  its precision (AC-24 point 5) and not for any other repair. Completing a partial pair
  (AC-24d) does not breach this — it writes the key that is **missing** and leaves the one
  that is present untouched, which is what `ON CONFLICT DO NOTHING` already guarantees.
- No column in this table participates in any existing rollup
  (`task_sessions.cost_subcents`, `tokens_in`, `tokens_out` are untouched).

## Permissions

The table is session-scoped and inherits the session's scope. Any read surface
added later MUST authorize via the parent session's task
(`authorizeTaskID(session.TaskID)`), per the backend's session-keyed entry-point
rule. This spec adds no read surface, so it adds no new authorization path.

---

## Out of scope

Named exclusions. Each is a contract, not an omission.

- **No `parent_session_id` column on `task_sessions`.** Rejected above.
- **No change to `dim_session` or to "sessions per card".** That series keeps its
  current meaning; the new table is a separate, additive fact.
- **No UI surface, no DTO field, no WebSocket event.** The subagent card
  continues to render from message metadata. No E2E coverage is implied,
  because no user-visible surface changes.
- **No REST/MCP read API.** Consumers read the table directly via the extract.
  If a product surface is wanted later, it is a separate spec with its own
  authorization design.
- **No merging of OpenCode's child ACP session.** `child_session_id` is stored;
  resolving it into the parent transcript is a separate feature.
- **No reading of Claude's async `outputFile`.** The 190 `async_launched` rows
  stay usage-less by design; inventing their usage is out of scope, and
  approximating it is forbidden by AC-7.
- **No cost or dollar attribution.** Tokens are as-reported, unpriced,
  unaggregated into any existing cost column.
- **No `prompt` or `result_text` column.** Structure, not content.
- **No recursive subagent-tree API.** `parent_tool_call_id` is recorded (AC-6);
  walking it is a consumer concern.
- **No retention or pruning policy beyond the FK cascade.** The table grows with
  message history and is bounded by the same session lifetime.
- **No retroactive repair of rows the backfill could not see.** Sessions already
  deleted are gone; AC-25 requires disclosing that, not fixing it.

*(added by Amendment 1)*

- **No foreign key from `agent_execution_id` to any execution table.** Executions
  are removed from `executors_running` when they stop; an FK would delete the
  measurement when the measured thing ended. Same reasoning as `turn_id`.
- **No retroactive resolution of `'unknown'` execution ids.** A sentinel row is
  never later upgraded to a real execution id, not by a subsequent frame and not
  by a repair job. The value is part of the row's identity, and rewriting it
  would move a row between identities — which is the class of silent mutation
  this amendment removes.
- **No de-duplication of cross-execution `tool_call_id` collisions.** Two rows
  differing only by execution are the correct outcome (AC-32), not a defect to
  collapse. Any consumer wanting an execution-agnostic count derives it with
  `COUNT(DISTINCT task_session_id || tool_call_id)` rather than having it
  imposed on the store.
- **No `agent_execution_id` on `task_session_messages`.** The backfill's
  sentinel is accepted instead. Adding execution identity to the message table
  is a separate change with its own migration and its own backfill problem, and
  nothing in this feature needs it.

## Verification

- Repository tests alongside `task_external_id_migration_test.go`: fresh-DB
  create, pre-migration seeded row, `runMigrations` twice, backfill idempotence,
  and the NULL-not-zero assertions for an `async_launched`-shaped payload.
- The same fresh-plus-replay matrix under `KANDEV_TEST_POSTGRES_DSN` (AC-19).
- Orchestrator tests for the frame paths: first-frame insert, later-frame merge,
  out-of-order terminal, nested `parent_tool_call_id`, empty-identity skip
  (each of the three missing ids), same-`tool_call_id`-later-turn, negative
  metric, reported-zero tool count, empty-string-to-NULL, and concurrent
  same-key upsert.
- A query-level check of AC-28 against a seeded store.

*(added by Amendment 1)* Additionally:

- **The cross-execution regression test (AC-32), which is the point of the
  amendment.** Seed a row for `(session S, execution B, tool_call_id T)`
  carrying a terminal `tool_status`, settled timestamp, and metric values; then
  deliver a frame for `(session S, execution A, tool_call_id T)` where A is an
  earlier, already-completed execution. Assert two rows exist and that B's row
  is byte-identical to its seeded state. A test that only asserts "two rows
  exist" does not cover this — the clobber it replaces was an *update*, so the
  assertion must be on B's column values.
- The AC-33 rebuild test in the shape `apps/backend/AGENTS.md` requires for
  table-rebuild migrations: seed a pre-amendment row (including its timestamps
  and NULL metric columns), run `runMigrations` twice, and assert the row
  survives with values intact, `agent_execution_id = 'unknown'`, the
  three-column key present, and the two-column key gone.
- An AC-31 test that a live frame with an empty `ExecutionID` still writes a
  row, stores `'unknown'`, and increments `unknown_execution` — asserting the
  row is *written*, since the failure mode being guarded is a silent skip.
- The same fresh-plus-replay matrix for all of the above under
  `KANDEV_TEST_POSTGRES_DSN` (AC-19, AC-34).

*(added by revision 1)* And, because every finding below was a failure or
detection path that no existing test would have caught:

- **AC-24a — the failed-backfill test.** Force the backfill statement to fail,
  then assert that *neither* `subagent_context_capture_since` nor
  `subagent_context_backfill_through` exists in `kandev_meta`, and that a second
  `runMigrations()` re-attempts the backfill rather than skipping it. Asserting
  only "the boot survived" does not cover this — the defect is a key written
  after a failure, which is invisible unless the key is the assertion.
- **AC-33a — the failed-rebuild test.** Force the rebuild to fail and assert:
  boot did not abort; the pre-amendment table is intact (two-column key still
  present, no partial table, no lost rows); `subagent_context_execution_since` is
  absent; and the next `runMigrations()` retries.
- **AC-33 clause 4 — the index-set assertion**, comparing a migrated database's
  index set against a fresh one's rather than checking the indexes exist.
- **AC-34 on PostgreSQL — assert the end state, not just stability.** A test that
  only re-runs and diffs passes against a migration that never applied. This is
  the specific check that would have caught `recreateTable`'s PostgreSQL no-op.
- **AC-7a — a reported-zero test for `total_tokens` and `duration_ms`**, asserting
  both store NULL, alongside the existing `tool_use_count` reported-zero test
  asserting it stores 0. The three columns deliberately do not behave alike, and a
  test suite that only covers the pointer field documents the wrong rule.
- **AC-1 on the merge path** — seed a `source = 'backfill'` row, deliver a live
  frame on the same key, and assert `source` and `observed_at` are unchanged
  while other columns filled forward.
- **AC-11 — a test pinning the spec's six terminal values to
  `isTerminalToolStatus`**, so the enumeration in this document cannot drift from
  the function again.
- **AC-28 — the health queries as written**, including the `:since` predicate and
  the identity filter, executed on both dialects against a seeded store that
  contains an AC-2-skipped message, so the query is proven not to false-alarm.

*(added by revision 2)* And, because each finding this revision closes was a state no
existing test constructs:

- **AC-33b — the fresh-database activation test.** Create a database from scratch,
  run `runMigrations()`, and assert `subagent_context_execution_since` **is present**.
  Repeat on a database seeded to predate the feature entirely (no
  `task_session_subagents` table at all). Both are the classes revision 1 silently
  excluded, and a test that only exercises the pre-amendment-table upgrade path passes
  against the broken contract — which is exactly what happened.
- **AC-33b — the negative half.** Assert the key is written when the end state holds
  but no rebuild fired. A test that asserts "rebuild fired ⇒ key written" cannot
  distinguish the two and would have missed this.
- **AC-33c — the ordering test.** Seed a pre-amendment table with `capture_since`
  absent (an installation whose earlier backfill failed), run `runMigrations()`, and
  assert the backfill completed and both its keys exist. Under the wrong order this
  fails on every invocation, so the test must run `runMigrations()` at least twice and
  assert success both times.
- **AC-24 — `backfill_through` across a retry.** Run the backfill to success, clear
  only `capture_since`, run again, and assert `backfill_through` is unchanged and
  non-empty. The literal revision-1 reading publishes `''` here.
- **AC-24c — the four-state matrix**, one case each, asserting *(revision 4)* which keys
  are present and absent in each state — never a row count, and never the consumer reading
  laid on that state, which is contract rather than a test target (§ *Acceptance criteria*
  preamble).
- **AC-24d — atomicity.** Force the second key write to fail and assert neither key
  is present.
- **AC-28a — the absent-key branch.** With `capture_since` absent, assert the health
  comparison is not run and the absence is reported as the signal.
- **AC-28/AC-29 — the banked-excess test, which is the point of the comparison
  change.** Seed a store containing a cross-execution collision (two rows, one
  message), then add subagent messages with the writer disabled, and assert the
  comparison diverges on the **first** missed write. Against revision 1's `>=` rule
  it does not diverge at all until the excess is consumed, so a test that only checks
  "eventually diverges" passes the broken version.
- **AC-28 — the duplicate-message symmetry test.** Seed two qualifying messages
  describing one subagent (AC-21b) and assert the comparison shows **no** shortfall.
  Collapsing only the observed side reintroduces a permanent false alarm on a healthy
  writer, which is the failure the `>=` relaxation was originally reaching for.
- **AC-21 — the two named sources.** Assert a backfilled row's `turn_id` equals the
  source message's `turn_id` column (not NULL), and that its `tool_status` comes from
  top-level `metadata.status` while `agent_status` comes from
  `metadata.normalized.subagent_task.status` — with a case where the two differ, so
  conflating them fails.
- **AC-21b — the duplicate-source test.** Seed two qualifying messages sharing one
  `(task_session_id, tool_call_id)` with different payloads, and assert exactly one
  row results and that it came from the newer `created_at`.
- **AC-2a — `turn_id` fill-forward.** Deliver a first frame with an empty turn id and
  a later frame with a real one; assert the row ends attributed to that turn.
- **AC-7a — the later-zero test.** Learn a non-zero `total_tokens`, then deliver a
  frame reporting `0`; assert the stored value survives. Same for `duration_ms`.

*(added by revision 3)* And, because each of these is a state no listed test constructs
and three of them are already wrong in the shipped writer:

- **AC-24e — the cross-format comparison test, which is the one that would have caught
  the live defect.** Write an activation key, then insert a row whose timestamp column is
  **later in the same calendar day** than that key, and assert the row is classified *at
  or after* the key. A test using a key from a different day passes against the broken
  string comparison — the date part alone decides it — so the same-day construction is
  mandatory, on both dialects. Assert the same for the AC-28 `:since` window.
- **AC-24e — the precision test.** Assert each of the three keys round-trips at
  nanosecond precision, and specifically that the value written is not the second-precision
  form `time.RFC3339` and `rfc3339Timestamp` produce. *(revision 4)* This asserts keys
  **this build writes**; a key already present from the pre-amendment build is out of its
  scope and SHALL NOT be rewritten to satisfy it (AC-24 point 5).
- **AC-24f — the empty-sentinel test.** Run the backfill on a database with no qualifying
  message; assert `backfill_through` is `''` and that nothing parses it as an instant. This
  is the fresh-install path, so it is also the most-executed one. *(revision 4)* The
  consumer-side reading is contract, not a test target (§ *Acceptance criteria* preamble).
- **AC-24d — the snapshot-order test.** Assert `backfill_through` never names an instant
  later than the newest message the insert actually covered. Constructing a concurrent
  commit is not required: asserting the evaluation order (the `MAX` is taken over the
  insert's snapshot or before it) is sufficient and is what the criterion constrains.
- **AC-9 — the later-negative test.** Learn a non-zero `total_tokens`, then deliver a
  frame reporting `-1`; assert the stored value survives and `anomalous_value` still
  incremented. Pair it with the existing later-zero test so the two rules read alike.
- **AC-28 — the two-direction test.** Seed a store with a persistent excess (a subagent
  row whose message write failed) AND then stop the writer while messages continue.
  Assert the SHORTFALL query goes non-zero on the **first** missed subagent despite the
  standing excess. A test that asserts only "the counts differ" passes against a signed
  difference, which is the formulation this replaces.
- **AC-28 — the excess-does-not-alarm test.** With a standing excess and a healthy
  writer, assert the shortfall query is zero and the excess query is non-zero, so a
  monitor wired to the shortfall does not fire on correct behaviour.
- **AC-33b — the self-heal test, which is the discriminating one.** Seed a database whose
  table is **already amended** (clauses 1–3 hold) but whose
  `subagent_context_execution_since` is **absent** — the state a committed rebuild plus a
  failed key write leaves. Run `runMigrations()` and assert the key appears, with no
  rebuild and no table creation having fired. An implementation gated on "rebuild fired or
  the table was created this pass" passes every other AC-33b test and fails only this one,
  which is precisely why it is required.
- **AC-12 / AC-5 — the post-settlement replace test.** Settle a row, then deliver a
  non-terminal frame carrying a different non-empty `model`; assert `model` updated while
  `tool_status` and `settled_at` did not.

*(added by revision 4)* And, because three of these construct states that exist on the
INSTALLED BASE rather than on a fresh database, and two of them fail against queries
revision 3 declared normative:

- **AC-24e — the one-microsecond boundary test, which is the discriminating one.** Write an
  activation key, then insert two rows whose timestamp column is exactly **one microsecond
  before** and exactly **at** that key; assert the first classifies *before* and the second
  *at or after*. On both dialects. The same-day test above passes against a `julianday`
  implementation — its ULP is roughly 40 µs, so it cannot see a 1 µs difference — which is
  why the same-day test alone does not establish AC-24e and this one does.
- **AC-24 point 4 — the sampling-order test.** Assert `capture_since` is not earlier than
  the instant the AC-21 `INSERT … SELECT` completed. Constructing a slow scan is not
  required: asserting the sample is taken at or after the insert, and never at transaction
  start, is what the criterion constrains. A test that merely asserts the key exists passes
  against a transaction-start sample that over-claims by the whole scan.
- **AC-24 point 5 — the legacy-key test.** Seed `kandev_meta` with a second-precision
  `capture_since` and `backfill_through` (the exact values the pre-amendment writer
  produces), run `runMigrations()` twice, and assert **both keys are byte-identical
  afterwards**. This is the negative of the precision test and they must both hold: an
  implementation that "repairs" precision passes the precision test and fails this one.
- **AC-24d — the partial-pair repair test.** Seed a database with `capture_since` present
  and `backfill_through` **absent** — the state the pre-amendment build's three independent
  `Apply` calls leave — plus qualifying messages. Run `runMigrations()` and assert
  `backfill_through` appears, names the true high-water mark of the source predicate rather
  than `''`, and that `capture_since` is unchanged. Repeat with the mirror state
  (`backfill_through` present, `capture_since` absent). An implementation guarding on
  `capture_since` alone never runs at all in the first case, so the assertion must be on the
  missing key appearing, not on the boot succeeding.
- **AC-28 — the malformed-metadata test.** Seed a qualifying store that also contains a
  message row with `metadata = ''` and one with `metadata = 'null'`, then run both anti-joins
  on both dialects and assert they return counts rather than raising. Without the `msg` CTE
  this fails on SQLite (`malformed JSON`) and on PostgreSQL (`''::jsonb`), and it fails in
  the direction that silently disables the check.
- **AC-28 / AC-19 — the PostgreSQL parse test.** Execute both anti-joins verbatim against
  PostgreSQL. Revision 3's form had no derived-table alias and did not parse there at all,
  so a SQLite-only execution of these queries is not evidence that they run.
- **AC-29 — the attribution test.** With `capture_since` written and **no** subagent row
  observed after it, assert the scoped `MAX(observed_at)` returns NULL and that NULL is
  reported as "nothing produced since activation", not as healthy. Seed backfilled and
  pre-activation rows in the same store so an unscoped `MAX` would return a non-NULL
  instant — that is the implementation this test exists to fail.
- **AC-25 — the self-heal-window classification test.** Seed a row carrying a **real UUID**
  `agent_execution_id` whose `observed_at` precedes `subagent_context_execution_since` (the
  state AC-33b's fourth case produces), and assert it is classified as carrying a real
  execution identity — not folded into the `'unknown'` population by the date comparison.

### The one shape a builder must not get wrong

A regression test SHALL assert, on an `async_launched`-shaped payload, that
`total_tokens`, `tool_use_count` and `duration_ms` are all `IS NULL` and none is
`0`. This is 75% of real traffic; a `DEFAULT 0` on any of those three columns
would make every future token-per-subagent average wrong by roughly a factor of
four, silently and permanently, and would do so *after* the activation point,
where AC-25 offers consumers no protection.

## Implementation notes carried by Amendment 1 (not contract)

The same PR review raised a second, smaller point. It is recorded here so the
Build round that implements Amendment 1 picks it up in the same diff — it
touches the identical files — but it is **not** an acceptance criterion, because
it constrains no observable behaviour and the spec should not freeze an internal
Go interface shape.

`orchestrator.SubagentContextRecorder`'s repository-side dependency is declared
as `repository.SubagentContextRepository`
(`internal/task/repository/interface.go:488-495`), which requires three methods
while the service uses only `UpsertSubagentContext`;
`ListSubagentContextsBySession` and `ListSubagentContextsByTurn` have no
production caller (they exist for the read surface this spec places out of
scope). Narrowing the service's dependency to a write-only interface, and
dropping `subagentContextAdapter`
(`internal/backendapp/adapters.go:780-788`) — a pure pass-through, since
`taskservice.Service` already satisfies the recorder interface directly — is
in scope for that round at Build's discretion. Removing the two list methods
from the repository implementation is **not** implied: they are the read path a
future spec would use, and deleting them is a separate decision.

## Related

- ADR [0027 — replayable schema migrations](../../decisions/0027-replayable-schema-migrations.md)
- `apps/backend/AGENTS.md` § *Schema & migrations (SQLite repository)*
- `apps/backend/internal/agentctl/AGENTS.md` § *Subagent tool-call nesting: what each agent emits*
