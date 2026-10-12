# Budget claim fencing: verification

Companion to [spec.md](spec.md) (`REQ-OFFICE-COSTS-003`), alongside
[prior-art.md](prior-art.md), [input-inventory.md](input-inventory.md) and
[amendments.md](amendments.md). Criterion ids below are spec.md's. Split out for the
specification linter's per-file size ceiling; the five files are one spec.

## Test strategy

Every criterion is observable at the repository or evaluation boundary. Assert submissions
on the activity **logger**, never with a `SELECT` against `office_activity_log`
(`AC-OFFICE-COSTS-002.2a`).

- **Race A regression (`.8`, `.9`, `.15`).** Read a policy snapshot at revision `r`; run
  `UpdateBudgetPolicy` to completion; attempt the snapshot's claim. Assert no activity row
  submitted, no error log, no counter increment. Then evaluate at revision `r+1` with spend
  reaching a level under the new values and assert exactly one row **is** submitted. That
  second half is the point — a test asserting only the first half also passes against an
  implementation that has merely broken emission.
- **Race B regression (`.10`, `.11`, `.12`).** With an exceeded-level claim already held
  for `(policy, period, revision)`, evaluate the same policy with spend in the alert band.
  Assert zero `budget.alert` submissions. Separately assert the invariant directly: after
  any successful exceeded claim, both rows exist at that revision.
- **Atomicity of the pair (`.10`).** Inject a failure between the two inserts using the
  same style of failpoint #3287 added for the discard (`failBudgetPolicyUpdateErr`,
  `budget_atomicity_internal_test.go`), and assert neither row exists and the evaluation
  fails open per `AC-OFFICE-COSTS-002.14`.
- **Revision lifecycle (`.4`, `.5`, `.6`).** Create → 1, asserted on what the create path
  hands back to its caller and not only on the stored row — the two disagree if the
  implementation leans on the column's `DEFAULT 1` — and asserted again after presenting a
  nonzero revision for creation, which must not survive. Update → 2, and the value returned
  to the updater is 2. Rolled-back update → unchanged. Two concurrent updates → `r+2`.
  Update naming a policy id that does not exist → the transaction rolls back and the caller
  gets an error, asserted as a behavior change from #3287, where that update committed and
  reported success.
- **Pair outcomes that are successes, not failures (`.10`, `.11`).** Two cases, both
  asserting a `budget.exceeded` row IS submitted. With an alert-level claim already held at
  `(policy, period, revision)`, run an over-limit evaluation: the companion insert conflicts,
  the pair still commits, `budget.exceeded` goes out, and both rows exist. Then, with an
  exceeded-level claim already held, run another over-limit evaluation: the exceeded insert
  writes nothing, no companion insert is attempted, nothing is emitted, and no claim-store
  failure is logged or counted. A test that only proves the happy pair misses both.
- **Invalid claim input is not a silent miss (`.13`, and `.8`'s guard ordering).** Call the
  claim path with revision `0`, then with an empty policy id, then with an empty period key,
  then with an empty level. Each shall report a claim-store error and fail open — the
  notification IS emitted, the
  counter increments, the error is logged — which is the opposite of `.9`'s silent refusal.
  This is the criterion that fails if the guard is implemented as a `WHERE` predicate
  instead of running before the fenced statement.
- **Which level the failure log names (`.14`).** With the claim store faulted, assert the
  structured log's `level` field is `exceeded` for the atomic pair of `.10` and `alert` for
  a standalone alert-band claim, and that a pair failure produces exactly one log line and
  one counter increment rather than two. The counter's label stays `op=claim` throughout.
- **Revision is server-owned on the wire (`.2`, `.7`).** Assert `revision` appears in the
  budget-policy JSON under that key and is not suppressed. Then PATCH a budget policy with
  a `revision` field in the request body and assert the stored revision is the server's
  own bump, unaffected by the value supplied.
- **Migration replay (`.1`, ADR 0027).** Fresh boot and boot replay on one connection,
  asserting the column exists once and replay is a no-op, with no new classifier.
- **The recreate is guarded (`.3a`).** Boot once against a database still carrying the
  pre-`.3` three-column claim table — so the cutover actually runs rather than being
  short-circuited by the probe — **then** write claims, **then** boot a second time and
  assert those claims are still there. The regression this guards is
  `AC-OFFICE-COSTS-002.11`, and an unguarded `DROP`/`CREATE` pair passes a single-boot test
  and fails this one. That ordering is load-bearing rather than incidental: claims written
  *before* the first boot are destroyed by the cutover itself, which
  `AC-OFFICE-COSTS-003.3` expressly permits, so a test that holds claims across the first
  boot asserts something a correct implementation does not promise. Assert the probe finds
  `revision` present on the second boot and does nothing. This bullet applies on every
  build: `AC-OFFICE-COSTS-003.3` runs the recreate whatever #3287's state, so there is no
  longer a merge precondition to record.
- **A failed recreate is visible, not swallowed (`.3a`).** Inject a failure into the
  recreate — the `failBudgetPolicyUpdateErr` failpoint style #3287 already uses in
  `budget_atomicity_internal_test.go`, or the shape of `TestCutover_RollbackAtEveryFailpoint`
  in `worktree_ownership_migration_test.go` — and assert the error reaches the caller that
  initializes the schema rather than being logged and dropped. This is the bullet that
  separates a correct implementation from the most likely wrong one, which is why it is
  worth its own case: the two calls sitting immediately beside the recreate's slot in
  `initSchema`, `r.runMigrations()` and `r.activateRunOutcome()`, both return nothing and
  are invoked bare, and the migration runner they use "swallows failures at WARN". A
  recreate wired to match its neighbours passes every other bullet here while violating
  `.3a`'s requirement that a failed recreate be visible rather than silently leaving the
  old three-column key in place.
- **PostgreSQL twin (`.3`, `.8`, Fork 3).** Extend #3287's `TestPostgresBudgetClaims`
  (`KANDEV_TEST_POSTGRES_DSN`) to cover the fenced insert and the four-column key. Fork 3's
  argument is dialect-specific, so a SQLite-only proof does not establish it. If the DSN is
  unavailable on the build runner, record that in the task plan with the skip output as the
  receipt — do not silently omit it.
- **Suppression boundary (`AC-OFFICE-COSTS-002.12`).** A refused claim on a superseded
  revision must still pause an unpaused in-scope agent and still deny a pre-execution
  check.
