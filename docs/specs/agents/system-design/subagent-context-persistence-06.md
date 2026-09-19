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
# Subagent context persistence System Design Part 6

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
