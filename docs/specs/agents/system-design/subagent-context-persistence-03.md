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
# Subagent context persistence System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
