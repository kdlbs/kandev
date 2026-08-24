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
# Subagent context persistence System Design Part 5

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
