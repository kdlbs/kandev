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
# Subagent context persistence System Design Part 7

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
