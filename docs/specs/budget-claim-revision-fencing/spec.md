# Budget claim fencing: a claim must belong to the policy revision that earned it

**Status:** spec
**Area:** backend — `internal/office/costs` (evaluation), `internal/office/repository/sqlite`
(claim store, policy store), `internal/office/models` (policy struct)
**Slug:** `budget-claim-revision-fencing`
**Requirement id:** `REQ-OFFICE-COSTS-003`
**Depends on:** PR #3287 (`REQ-OFFICE-COSTS-002`) — **open, unmerged**; see
[Dependency and sequencing](#dependency-and-sequencing)
**Amends:** `AC-OFFICE-COSTS-002.5`, `.8`, `.10`, `.13`, `.14a` as introduced by PR #3287 —
see [Amendments](#amendments-to-req-office-costs-002)

---

## Why

`REQ-OFFICE-COSTS-002` (PR #3287) makes `budget.alert` / `budget.exceeded` idempotent with
a durable claim table keyed `(policy_id, period_key, level)`. Two reviewers found the same
structural gap from opposite ends: **the claim key carries no evidence of which policy
revision earned it, nor of what else has already been claimed for that period.** #3287
deliberately accepted both (`AC-OFFICE-COSTS-002.8`'s stale-evaluation carve-out). This
spec closes them.

**Race A — stale-snapshot claim poisoning**
([coderabbitai](https://github.com/kdlbs/kandev/pull/3287#discussion_r3914827075)).
`CheckBudget` calls `ListBudgetPolicies` once, then hands each already-read
`*BudgetPolicy` to `evaluatePolicy`, so an evaluation works from a snapshot. If
`UpdateBudgetPolicy` discards that policy's claims and commits new values mid-evaluation,
the evaluation's `Claim` insert lands **after** the discard and writes the old threshold's
outcome into the new revision's claim slot. Two user-visible harms:

- **(A1) obsolete emission** — a `budget.alert` row whose `limit_subcents` describes the
  policy the user just replaced;
- **(A2) suppressed legitimate crossing** — the next evaluation that genuinely reaches a
  level under the *new* policy finds the slot taken and stays silent until the next update
  or period rollover.

**Race B — `budget.alert` submitted after `budget.exceeded`**
([cubic-dev-ai](https://github.com/kdlbs/kandev/pull/3287#discussion_r3914856011)).
`claimAndEmit`'s `afterClaim` seam runs `claimCompanionAlert` *after* the exceeded claim,
not atomically with it. An alert-band evaluation racing an over-limit one can win the alert
claim in that gap and emit `budget.alert` describing a below-limit spend after
`budget.exceeded` has gone out. Both rows land once each; the defect is that a period
already over its limit still produces an alert-band notification at all.

## Dependency and sequencing

**This work cannot be built on `origin/main`.** Verified 2026-09-07 at `origin/main` =
`cd7823631`: `office_budget_claims`, `Repository.Claim`, `periodKeyFor` and
`BudgetCheckResult.AlertSubmitted` do not exist there, and `gh pr view 3287` reports
`"state":"OPEN"`, `"mergedAt":null`. Every symbol this spec modifies is introduced by
#3287 (head `feature/budget-alerts-should-c2t`).

Build shall branch from #3287's head, not `main`, and shall record which commit in the task
plan. If #3287 has merged by then, branch from `main` as normal. This is a sequencing fact,
not an open question: no contract below turns on it except `AC-OFFICE-COSTS-003.3`,
where it selects only which file the claim-table DDL is edited in — never what schema the
running database ends up with, which `AC-OFFICE-COSTS-003.3a` settles on every boot.

## Prior art

Three legs, each opening with a receipt, in [prior-art.md](prior-art.md): the wiki leg and
the saas-kb leg both came back **unavailable on this runner** and are recorded there with
their receipts and intended queries, so neither fork below is established as unopposed by
prior positions. The in-repo leg **found four precedents and decided both mechanisms** —
three agreeing fencing implementations (`ownership_generation`, `gitlab_configs.revision`,
`users.settings_revision`) that fix the column shape, the in-SQL `+ 1` bump, the
predicate-inside-the-write fence and the `RETURNING` read-back, and
`run_outcome_activation.go`'s `columnExists` probe, which fixes the migration shape behind
`AC-OFFICE-COSTS-003.3a`. Where this spec departs from a precedent is stated there too.

## Input inventory

Sampled by reading the code on #3287's head, not inferred, and recorded in
[input-inventory.md](input-inventory.md): the two tables and their exact columns, the
`SELECT *` / `StructScan` coupling that makes `AC-OFFICE-COSTS-003.2` mandatory rather than
cosmetic, `UpdateBudgetPolicy`'s transaction shape and its test-only failpoint,
`evaluatePolicy`'s snapshot-then-claim ordering and the `afterClaim` seam Race B exploits,
`office_activity_log`'s missing tiebreak column, the two budget surfaces on `main`
(including the PR #3276 change that makes #3287's inbox claim stale), and the `updateBudget`
handler's unfenced read-modify-write.

## Design decisions

**Fork 1 — fence, or serialize?** coderabbitai offered both. **Fence.** #3287's
`AC-OFFICE-COSTS-002.8` already rejected serialization with a reason this spec accepts
verbatim: a lock spanning the evaluation and the update *"would put a user-facing write
behind a background evaluation."* A revision fence costs one column and one predicate and
holds no lock across the evaluation.

**Fork 2 — reuse `updated_at`, or add a column?** **Add `revision`.** `updated_at` is
already bumped on every update, so it looks free, but two updates inside one clock tick
produce the same value, timestamp round-tripping through the driver is not guaranteed to
survive exact-equality comparison, and a backwards clock step breaks monotonicity. An
`INTEGER` counter has none of those failure modes, and all three in-repo precedents chose
one.

**Fork 3 — is `revision` a fence only, or also part of the claim's key? Both, and both are
load-bearing.** The fence alone is sufficient on SQLite, whose single writer connection
(`internal/db/pool.go`) serializes the claim insert against the update transaction. It is
**not** sufficient on PostgreSQL under `READ COMMITTED`: a claim insert whose `WHERE
EXISTS` reads the pre-update revision before the update commits will insert a row that the
update's already-executed `DELETE` cannot remove, leaving a stale claim behind. Putting
`revision` in the claim's primary key neutralises that survivor — it is keyed to a
superseded revision and can never match a later evaluation. So: **the fence prevents the
obsolete emission (A1); the revision in the key prevents the suppression (A2).** Removing
either reopens one half on one dialect.

This does not contradict #3287's exclusion of `workspace_id` from the claim row. That
exclusion was about *duplicating a fact the policy already owns*, which could drift.
`revision` on a claim is not a copy of the policy's current revision — it is the identity
of *which* revision earned this claim, which is a different fact and cannot drift.

**Fork 4 — how to close Race B.** cubic proposed making the exceeded and companion claims
atomic, *or* making alert claims reject an already-claimed exceeded level. **Atomicity is
necessary; the rejection rule is not, once atomicity holds.** Atomicity is also
*insufficient* as a fix for the ordering symptom — worth stating, because that is the
obvious reading of the comment: if the alert-band evaluation wins the alert claim *first*,
atomicity of the exceeded pair changes nothing and both rows still go out unordered. What
atomicity buys is the invariant **"an exceeded claim implies an alert claim at the same
revision"**; given it, an alert-band evaluation arriving after an exceeded claim finds the
alert row present and suppresses via the ordinary primary-key conflict. The rejection rule
would be a second, divergent answer to a question the key already answers — the pattern
#3287 explicitly refuses.

**Fork 5 — the residual write-order inversion is excluded, deliberately.** Rationale and
bound in [Out of scope](#out-of-scope).

## Contract

### REQ-OFFICE-COSTS-003: Budget claims are fenced to the policy revision that earned them

**Intent:** A budget claim shall be valid only for the policy revision the evaluation
actually read, and an evaluation whose snapshot has been superseded shall neither emit nor
suppress.

**User story:** As a workspace owner who has just raised a budget limit, I want the next
real crossing of the *new* limit to notify me, and I do not want a notification quoting
the limit I replaced.

#### Acceptance criteria

Terminology (`evaluation period`, `alert level`, `limit level`, `reaches a level`, `claim`,
`emit`, `submit`) is inherited unchanged from `REQ-OFFICE-COSTS-002`'s `## Terminology`.
**Policy revision** is added: a per-policy integer that identifies one immutable set of
policy field values, assigned by the server, starting at 1 and increasing by exactly 1 on
each successful update.

- **AC-OFFICE-COSTS-003.1:** `office_budget_policies` shall carry a `revision` column,
  `INTEGER NOT NULL DEFAULT 1`, added by a replayable `ALTER TABLE ... ADD COLUMN`
  migration registered through the existing `r.migrate.Apply(name, sql)` mechanism, so
  that existing rows are backfilled to 1 and a boot replay is a no-op. No new local
  error-string classifier shall be introduced; `db.IsDuplicateColumnError` is the only
  one (ADR 0027).
- **AC-OFFICE-COSTS-003.2:** `models.BudgetPolicy` shall carry the matching field, so
  that the existing `SELECT *` reads in `GetBudgetPolicy` and `ListBudgetPolicies` keep
  scanning. The field shall be serialized in the policy's JSON representation as a
  read-only diagnostic under the JSON key `revision`; the frontend `BudgetPolicy` type
  need not change, and the field shall not be suppressed from JSON.
- **AC-OFFICE-COSTS-003.3:** `office_budget_claims` shall carry a `revision INTEGER NOT
  NULL` column, and its primary key shall be `(policy_id, period_key, level, revision)`.
  `createCostTables` shall declare that shape, **and** the recreate of
  `AC-OFFICE-COSTS-003.3a` — dropping `office_budget_claims` and recreating it with that
  same four-column primary key — shall run on every boot, whatever #3287's state. Declaring the
  final shape is not sufficient on its own, and an implementation that relies on it alone
  does not satisfy this criterion: `createCostTables` issues `CREATE TABLE IF NOT EXISTS`,
  which is a no-op against a database that has already executed #3287's three-column
  `CREATE TABLE`. Such a database would keep the three-column table and its three-column
  primary key, so every fenced insert would fail on the missing column and every
  evaluation would take the `AC-OFFICE-COSTS-002.14` fail-open path from then on —
  emitting a duplicate notification, an error log and a counter increment on each pass,
  violating `AC-OFFICE-COSTS-002.11`, and leaving this requirement's fence permanently
  inert — while a suite that only ever builds a fresh database stays green.
  `AC-OFFICE-COSTS-003.3a`'s probe is what closes that, and it costs nothing where the old
  shape never existed: it finds `revision` present and does nothing, which is exactly its
  second-boot behavior. Whether #3287 has merged — `gh pr view 3287 --json state`, the
  only check Build shall consult, deployment state being neither observable from the
  repository nor to be consulted — therefore selects only **where the final-shape DDL is
  edited**: #3287's own `CREATE TABLE` statement while it is open, the same statement on
  `main` once it has merged. It does not decide whether the recreate runs. Deciding that
  from a probe of the live schema rather than from the fork is the point: the fork is
  answered when Build authors the change, whereas the shape of the database this code
  meets is settled later, when it deploys. No `r.migrate.Apply` migration shall be added
  for this table; `AC-OFFICE-COSTS-003.3a` is expressly not one. Claims are pure
  suppression state, and losing them costs at most one additional notification per policy
  per period, which
  `AC-OFFICE-COSTS-002.17` already describes as the accepted first-run behavior.
- **AC-OFFICE-COSTS-003.3a:** The recreate path shall take effect at most once per
  database. It shall first probe whether `office_budget_claims` already declares
  `revision` — the office repository's existing `columnExists` helper answers this on both
  dialects — and shall do nothing when it does. It shall **not** be expressed as an
  `r.migrate.Apply` statement: that runner re-executes its SQL on every boot and is
  idempotent only because an already-exists error is swallowed, so an unguarded
  `DROP TABLE` / `CREATE TABLE` pair succeeds on every boot and destroys claim state each
  time, violating `AC-OFFICE-COSTS-002.11`. It shall return its error to its caller rather
  than swallowing it, so a failed recreate is visible rather than silently leaving the old
  three-column key in place. It shall run after `office_budget_claims` exists, so it probes
  the real table rather than racing its creation. A claim attempt arriving from an
  already-running backend while the recreate is in flight is **not** required to succeed:
  it is an ordinary claim-store error and takes the `AC-OFFICE-COSTS-002.14` fail-open path,
  costing one further notification per evaluation that attempts a claim inside the recreate
  window, beyond the one `AC-OFFICE-COSTS-003.3` already accepts for discarding the claims
  themselves: that path emits without recording a claim, so it does not suppress the next
  healthy evaluation. This criterion therefore does not require the recreate to lock
  concurrent claim traffic out, and an implementation that does lock it out satisfies it
  equally.
- **AC-OFFICE-COSTS-003.4:** When a budget policy is created, the system shall assign it
  revision 1, regardless of any revision value presented for creation — zero, or any other.
  The system shall make that assigned value available to the caller on the created policy,
  for the same reason `AC-OFFICE-COSTS-003.5` requires the post-update value to be read back:
  the create response carries the policy, and `AC-OFFICE-COSTS-003.2` makes `revision` a
  field of it, so a create that stores 1 while handing its caller a zero would report a
  revision the database does not hold. Satisfying this criterion by the column's `DEFAULT 1`
  alone therefore does not satisfy it.
- **AC-OFFICE-COSTS-003.5:** When a budget policy is updated, the system shall increase
  that policy's revision by exactly 1, in the same transaction as the row update and the
  claim discard that `AC-OFFICE-COSTS-002.8` requires. The increase shall be expressed as
  an in-SQL `revision = revision + 1`, never as a value computed in application code from
  a prior read, so that two concurrent updates cannot both write the same revision. The
  system shall read the post-update revision back within that transaction and shall make
  it available to the caller, so that the value returned to whoever performed the update
  is the value now stored. If the update matches no policy row — the policy was deleted
  between the caller's read and this write — the read-back returns nothing, and the system
  shall roll the transaction back and return an error to the caller rather than committing
  an empty update. This tightens the pre-`REQ-OFFICE-COSTS-003` behavior, where the update
  ignored its affected-row count and reported success. The obligation is at the repository
  boundary only: the `updateBudget` handler already maps every update error to HTTP 500,
  and giving this particular case a 404 instead is a separate user-facing change, excluded
  under [Out of scope](#out-of-scope).
- **AC-OFFICE-COSTS-003.6:** If an update transaction is rolled back for any reason, then
  the policy's revision shall be unchanged. A rolled-back update shall not consume a
  revision number.
- **AC-OFFICE-COSTS-003.7:** The system shall not accept a revision from an API client.
  The budget-policy update request body shall have no revision field, and supplying one
  shall have no effect on the stored revision.
- **AC-OFFICE-COSTS-003.8:** When an evaluation attempts a claim **whose outcome decides
  an emission**, it shall present the revision from the same policy snapshot it computed
  spend and levels from, and the claim shall be recorded only while that revision is still
  the policy's stored revision. The revision check and the insert shall be a single
  statement, so no evaluation can observe a matching revision and then insert against a
  superseded one. That single-statement rule governs the fence check only. It does not
  forbid the input guard `AC-OFFICE-COSTS-003.13` requires, which shall run **before** the
  statement is issued: once the statement has run, a zero-row result carries no distinction
  between a refused fence, a policy that no longer exists and an already-held claim, all
  three being the silent miss of `AC-OFFICE-COSTS-003.9`, so any outcome that must be told
  apart from them has to be settled before the statement rather than read out of it. A
  claim that decides no emission — the companion of `AC-OFFICE-COSTS-003.10` — is exempt;
  see there.
- **AC-OFFICE-COSTS-003.9:** When a claim is refused because the presented revision is no
  longer the policy's stored revision, the system shall treat it exactly as
  `AC-OFFICE-COSTS-002.3` treats an already-held claim: no activity row, no error log, no
  counter increment, and the evaluation shall report `submitted` false for that level and
  shall still return its result to its caller without error. Enforcement
  (`AC-OFFICE-COSTS-002.12`) is unaffected: `pause_agent` and the pre-execution denial
  still run off the level check.
- **AC-OFFICE-COSTS-003.10:** When an evaluation reaches the limit level, the system shall
  record the exceeded-level claim and the companion alert-level claim of
  `AC-OFFICE-COSTS-002.5` **atomically**: once the transaction commits either both rows
  exist for that policy, period and revision, or this evaluation wrote neither. The
  `budget.exceeded` emission shall be decided by
  whether the exceeded-level row was inserted by this evaluation; the companion row's
  outcome shall not change it. The fence shall be evaluated **once for the pair**, on the
  exceeded-level insert; the companion insert shall carry no fence of its own, because its
  key already names the revision, so a companion row written against a revision that has
  since moved is inert rather than wrong. Re-checking the fence on the companion insert is
  prohibited: it could refuse there after succeeding on the exceeded insert, breaking
  `AC-OFFICE-COSTS-003.11`. Two outcomes of the pair are named here so that neither is left
  to inference. **A companion insert that conflicts with an alert-level claim already held
  for that policy, period and revision is a success for the pair,** not a failure: that row
  is the ordinary trace of an earlier alert-band evaluation, `AC-OFFICE-COSTS-003.11`'s
  invariant is satisfied by its presence however it arrived, and the transaction shall
  commit and the `budget.exceeded` row shall go out. Reading the companion's zero affected
  rows as a pair failure is prohibited — it would withhold a `budget.exceeded` that the
  exceeded insert had already earned, which `AC-OFFICE-COSTS-002.5` forbids. **When the
  exceeded insert itself writes no row** — refused by the fence, or conflicting with an
  exceeded claim already held — the companion insert shall not be attempted and the pair
  shall commit having written nothing: there is no emission to protect, and
  `AC-OFFICE-COSTS-003.11` is not engaged.
- **AC-OFFICE-COSTS-003.11:** The system shall maintain the invariant: **if an
  exceeded-level claim exists for a given policy, period and revision, an alert-level
  claim exists for that same policy, period and revision.** No operation shall create an
  exceeded-level claim without it.
- **AC-OFFICE-COSTS-003.12:** While an exceeded-level claim is held for a policy, period
  and revision, an evaluation of that policy whose spend reaches the alert level shall not
  emit `budget.alert` for that period and revision. This shall follow from
  `AC-OFFICE-COSTS-003.11` and the claim's primary key, and shall not be implemented as a
  separate read of the exceeded claim before emitting.
- **AC-OFFICE-COSTS-003.13:** If a claim attempt presents a revision less than or equal to
  zero, an empty policy id, an empty period key, or an empty level, then the system shall
  report a claim-store error, which `AC-OFFICE-COSTS-002.14` then handles by emitting,
  logging and incrementing the counter. Those four are exactly the components of the claim's
  primary key, and the guard covers all four rather than a subset: such input is a
  programming error rather than a superseded snapshot, and shall degrade toward an extra
  notification, never toward silence. An empty level leaves
  `AC-OFFICE-COSTS-003.14`'s log unambiguous — that field names the level whose emission the
  failed claim governs, which is fixed by the calling branch rather than by the rejected
  value.
- **AC-OFFICE-COSTS-003.14:** The claim-store failure counter shall keep its single label
  value `op=claim`. The accompanying structured log's `level` field shall carry the level
  whose *emission* the failed claim governs — `exceeded` for the atomic pair of
  `AC-OFFICE-COSTS-003.10`, `alert` for an alert-band claim — so the field never carries a
  sentinel or two meanings.
- **AC-OFFICE-COSTS-003.15:** A claim row whose revision is no longer the policy's stored
  revision shall never suppress an emission and shall not be read for any decision. The
  claim discard of `AC-OFFICE-COSTS-002.8` shall remove **all** of a policy's claims
  regardless of revision, and shall not be narrowed to the current revision, so such rows
  are reclaimed by the next update or by the policy's deletion cascade. No retention job
  shall be added.

## Forced-to-invent pass

**Ordering.** Policy evaluation order is unchanged: `created_at ASC, id ASC`
(`AC-OFFICE-COSTS-002.16`). Within the update transaction: discard claims, update the row
(including the revision bump), read the revision back, commit — discard first, as #3287
specifies. Within `AC-OFFICE-COSTS-003.10`'s atomic pair the exceeded row is inserted
first, because its insertion decides the emission; the companion alert row follows in the
same transaction. The relative order of the two resulting `office_activity_log` rows is
**not** specified — see [Out of scope](#out-of-scope).

**Idempotency and retry.** Nothing here is retried. A refused claim (already held,
superseded revision, deleted policy) is terminal for that evaluation; the next trigger
re-evaluates from a fresh snapshot. Re-running an identical evaluation after a successful
claim produces no second row. An update with unchanged field values still bumps the
revision and still discards claims — the revision counts *update operations*, not *value
changes*, matching `AC-OFFICE-COSTS-002.8`'s refusal to compare old and new values.

**Concurrency — two callers, same row.** Enumerated exhaustively:

1. *Two evaluations, same policy, period, level, same revision, healthy store.* The
   primary key admits one. Exactly one submission (`AC-OFFICE-COSTS-002.10` preserved).
2. *Evaluation vs. `UpdateBudgetPolicy`, evaluation claims after the update commits.* The
   fence refuses. No obsolete row; the claim slot for the new revision stays open, so the
   next evaluation that reaches a level emits. **Race A closed.**
3. *Evaluation vs. `UpdateBudgetPolicy`, evaluation claims before the update commits.* The
   claim is accepted and the row emitted — correctly, since the policy still held those
   values at that instant. The update's discard then removes the claim (SQLite), or it
   survives keyed to the superseded revision and is inert (PostgreSQL `READ COMMITTED`).
   Either way no later evaluation is suppressed.
4. *Alert-band evaluation vs. over-limit evaluation, exceeded claims first.* The
   alert-band evaluation finds the companion alert row present and suppresses. Only
   `budget.exceeded` is emitted. **Race B's semantic half closed.**
5. *Alert-band evaluation vs. over-limit evaluation, alert claims first.* Both emit, once
   each. Their activity-log order is unconstrained — the named residual.
6. *Two concurrent `UpdateBudgetPolicy` calls.* Serialized by the single writer (SQLite)
   or the row lock (PostgreSQL). Each bumps by 1; the final revision is `r+2`; no
   increment is lost. Which caller's *field values* win is the pre-existing unfenced
   read-modify-write in `updateBudget`, excluded below.
7. *Evaluation vs. `DeleteBudgetPolicy`.* The fence's existence check fails, so the claim
   is refused with no row, no log and no counter — `AC-OFFICE-COSTS-002.14a` preserved,
   now reached through the fence rather than through the foreign key. The foreign-key
   classifier is retained because the constraint still carries `AC-OFFICE-COSTS-002.9`'s
   cascade, and dropping the classifier would let a genuine foreign-key error read as a
   store fault.

**Nil / empty / error.** Revision `<= 0`, empty policy id, empty period key, empty level:
claim-store error, fail open (`AC-OFFICE-COSTS-003.13`). Claim-store error during the
atomic pair: the transaction rolls back, no claim is held, the evaluation emits
`budget.exceeded` and records the failure — `AC-OFFICE-COSTS-002.14` continues to take precedence over
`AC-OFFICE-COSTS-002.5` and `.10`. Activity write failure after a successful claim: one
notification lost, not retried, not reported (`AC-OFFICE-COSTS-002.2a`, unchanged).
Discard failure during an update: whole update rolls back, revision unchanged, error
returned to the caller (`AC-OFFICE-COSTS-002.8`, extended by
`AC-OFFICE-COSTS-003.6`).

**Defaults and boundaries.** Revision starts at 1, is `int64`, never resets and is never
reused for a policy id; a period rollover, a backend restart and a claim discard all leave
it alone. Existing rows migrate to 1. A `period` change still side-steps this whole class
of races by moving `period_key`. Row growth stays bounded by `policies × periods × 2` in
steady state, plus case 3's inert survivors, which the next update or deletion reclaims —
no retention job (`AC-OFFICE-COSTS-003.15`).

## Amendments to REQ-OFFICE-COSTS-002

Which `REQ-OFFICE-COSTS-002` criteria this spec supersedes, widens, or leaves untouched —
all 21 of them, named individually — is in [amendments.md](amendments.md). No acceptance
criterion of `REQ-OFFICE-COSTS-003` lives there.

## Out of scope

- **Serializing `AC-OFFICE-COSTS-003.3a`'s recreate across two initializers.** `.3a` requires
  the recreate to take effect at most once per database, guarded by its `columnExists` probe;
  it does **not** require two backends initializing the same database to be prevented from
  both performing it. **Window:** two backends cold-starting simultaneously against one
  PostgreSQL database still carrying the pre-`.3` three-column `office_budget_claims` — not
  reachable on SQLite, whose writer pool is `MaxOpenConns(1)` (`internal/db/pool.go`), and not
  reachable at all once any boot has completed the recreate. **Residual:** both probe, both
  find `revision` absent, both drop and recreate; the table still ends in the four-column
  shape `AC-OFFICE-COSTS-003.3` mandates. Worst case one initializer's `CREATE` races the
  other's `DROP` and errors, which `.3a` already requires be returned to its caller, so that
  backend fails to boot, restarts, probes, finds `revision` present and does nothing. Claim
  loss across the window is already permitted by `AC-OFFICE-COSTS-003.3` via
  `AC-OFFICE-COSTS-002.17`. **Mechanism for a follow-up:** a shared advisory lock, as in
  `task/repository/sqlite/worktree_ownership_migration.go:216`, which takes
  `pg_advisory_xact_lock` on a fixed bigint so every instance derives the same value and a
  second boot waits for the first; a follow-up must choose a **new** lock id, not reuse that
  file's `taskWorktreeCutoverLockID`, which would cross-block an unrelated migration.
  **Why not here:** no in-repo precedent for this mechanism carries a concurrency test, so a
  follow-up has to decide whether to set that bar rather than assume it exists.
- **The residual `budget.alert`-after-`budget.exceeded` write-order inversion.** When an
  alert-band evaluation wins the alert claim strictly before a concurrent over-limit
  evaluation claims `exceeded` (concurrency case 5), both rows are submitted and the order
  in which they reach `office_activity_log` is unconstrained, so the activity feed can
  render them exceeded-then-alert. **Bound:** at most one such pair per policy, period and
  revision, under genuine concurrency only. **Harm:** display only — both rows are
  individually truthful about the spend they observed, neither is duplicated, neither is
  lost, and the Office inbox is unaffected because `listBudgetInboxEntries` concatenates
  the two actions as separate already-sorted lists rather than interleaving them. **Why not
  fixed:** closing it needs either a lock spanning claim and emit — the mechanism
  `AC-OFFICE-COSTS-002.8` rejected and Fork 1 upholds — or a monotonic ordering key on
  `office_activity_log`, a table shared by every Office surface whose only ordering today
  is `created_at DESC` with no tiebreak, so two rows written in one clock tick already have
  undefined relative order. Promising an order for this one pair would be a new table-wide
  guarantee, not a fix; a follow-up wanting it should specify the guarantee for the whole
  activity log first.
- **Optimistic concurrency for the budget-policy HTTP API.** `updateBudget` does an
  unfenced `GetBudgetPolicy` → patch → `UpdateBudgetPolicy`, so two concurrent PATCHes can
  lose one caller's field values. The revision column added here would make that fixable —
  `AC-OFFICE-COSTS-003.7` deliberately declines — but an expected-revision request field, a
  conflict status code, and the frontend threading are a separate user-facing change. The
  same exclusion covers the status code for `AC-OFFICE-COSTS-003.5`'s deleted-policy case:
  the handler's existing "any update error is a 500" mapping stands, and turning that one
  case into a 404 belongs with the rest of this endpoint's error contract, not here.
- **Everything `REQ-OFFICE-COSTS-002` already excluded**, inherited unchanged rather than
  restated: unifying the three disagreeing spend-window implementations
  (`costs.periodCutoff`, `backendapp.budgetEvaluator.spendForPolicy`,
  `cron.periodStartFor`, and `BudgetPeriod.Valid()` accepting a wider `period` set than
  `periodCutoff` honours), changing when a policy reaches a level, the cron budget-alert
  trigger, resolving or expiring inbox items, `budget.exceeded` visibility, and a
  user-facing notification-frequency setting. Nothing here depends on any of them.
- **Correcting #3287's stale claim that only `budget.alert` feeds the Office inbox.** PR
  #3276 changed that on `main` after #3287 branched; it is recorded in
  [Input inventory](#input-inventory) so Build does not rely on it. Editing #3287's own
  design documents is not this card's work.

## Verification

Test strategy, per criterion, is in [verification.md](verification.md); research
provenance is in [prior-art.md](prior-art.md); sampled code shapes are in
[input-inventory.md](input-inventory.md); the `REQ-OFFICE-COSTS-002` deltas are in
[amendments.md](amendments.md). They are separate files because this document sits against
the specification linter's 32,768-byte ceiling for its kind, the same reason
`costs-03.md` / `costs-04.md` are split. The five files are one spec.

## User-visible surfaces (E2E decision input)

Backend-only: no new HTTP route, no request-body change, no required frontend change, no
new user-facing string. The two surfaces that read these rows — the Office inbox
(`/office/inbox`) and the workspace activity feed (`/office/workspace/.../activity`) — are
unchanged in code and see strictly fewer and more accurate rows. `revision` appears as an
additional read-only field in the budget-policy JSON, which the hand-written frontend type
ignores.

**Recommendation: no new E2E coverage.** The behavior is a concurrency property between two
backend goroutines and a database transaction, observable at the repository and evaluation
boundaries and not reproducible through the UI. Go integration tests plus the PostgreSQL
twin are the right level.
