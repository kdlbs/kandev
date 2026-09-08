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
# Subagent context persistence System Design Part 8

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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

- ADR [0027 — replayable schema migrations](../../../decisions/0027-replayable-schema-migrations.md)
- `apps/backend/AGENTS.md` § *Schema & migrations (SQLite repository)*
- `apps/backend/internal/agentctl/AGENTS.md` § *Subagent tool-call nesting: what each agent emits*
