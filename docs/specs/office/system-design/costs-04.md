---
status: draft
system: office
requirements:
  - REQ-OFFICE-COSTS-002
created: 2026-09-02
updated: 2026-09-02
owners:
  - cfl
---
# Office: Budget Notification Idempotency System Design (Part 4)

The operational half of [part 3](costs-03.md), split out when that file reached the
specification linter's 32,768-byte ceiling for a system-design file. Part 3 defines
the claim store and the control flow that uses it; part 4 defines what the claim
must **not** gate, how the design behaves when a component fails, where the state
lives, and what it exposes. The two parts are one design against
`REQ-OFFICE-COSTS-002` and share part 3's
[requirement mapping](costs-03.md#requirement-mapping).

## Suppression boundary

The most dangerous way to implement this requirement is to gate too much on the
claim. Enforcement is a level check and must stay one (AC-002.12):

- **`pause_agent`** runs on every evaluation where spend is at or above the limit,
  claim or no claim: a user who un-pauses an agent that is still over budget must
  see it re-paused next evaluation. `pauseAgentForBudget` already no-ops on an
  already-paused agent, so repeating it is cheap and idempotent.
- **`CheckPreExecutionBudget`** keeps denying on `LimitExceed` for `pause_agent`
  and `block_new_tasks` policies, reading the level and never the claim. A policy
  that already emitted must still block every subsequent run.
- **`BudgetCheckResult.AlertFired` / `LimitExceed`** keep meaning "reaches this
  level". Only the two new fields report submission.

One rule: the claim gates writes to `office_activity_log` and nothing else.

## Failure and recovery

- **Claim store read/write error** (AC-002.14): treat the claim as not held, emit,
  log at error level, increment the counter, and report `submitted` true because a
  row went out (AC-002.13). Rationale: this requirement fixes a flood produced by
  *normal* operation, whereas a broken store is exceptional, and degrading it to
  "duplicates plus an error signal" beats "silently stop warning the user about
  money". A store broken for a whole period reproduces today's behavior exactly,
  and the counter makes that visible. The error must not fail the evaluation:
  `evaluatePolicy` still returns its result and the cost event is not rolled back.
- **Activity write fails after a successful claim**: one notification is lost for
  that (policy, period, level). Not detectable from the caller; see
  [Claim-then-emit](costs-03.md#claim-then-emit) for why this is the accepted direction.
- **Concurrent evaluation of the same policy**: resolved by the primary key.
  Exactly one caller inserts; the other observes `claimed=false` and suppresses.
  AC-002.10's "exactly one" is a count of **submissions**, not of durable rows:
  assert it on the activity logger, never with a `SELECT` against
  `office_activity_log`. AC-002.2a makes the durability of any one row unobservable,
  so a durable-row assertion is testing something the contract does not promise.
  No transaction spanning claim and emit is required, and none should be added —
  the activity write is not transactional with anything.
- **Concurrent evaluation where the claim store also faults**: the two rules above
  compose into a duplicate, and AC-002.10 says so explicitly. The healthy caller
  wins the insert and emits; the faulted caller fails open per AC-002.14 and also
  emits. Two rows for one crossing. **AC-002.14 takes precedence over AC-002.10** —
  a documented tie-break, not an oversight: a duplicate under a broken store is
  today's behavior, whereas suppressing on claim-store error converts a storage
  fault into silent under-notification about money. Do not "fix" it with a retry
  loop inside the claim; a retry outliving the evaluation moves the fault into the
  cost-event path, which AC-002.14 forbids. Unlikely on SQLite, where
  `internal/db/pool.go` uses a single writer connection to serialize writes; a real
  path on Postgres. Test consequence, which is why this is written down: the
  AC-002.10 concurrency test must run against a **healthy** store, and the
  AC-002.14 fault-injection test must not assert single emission. Written the other
  way round they contradict each other and one is flaky rather than wrong.
- **Claim discard fails during a policy update** (AC-002.8): the whole update
  fails and rolls back. See [Repository surface](costs-03.md#repository-surface). The
  caller gets an error, the policy keeps its old values, and the claims survive
  intact — consistent with the un-updated policy they belong to.
- **An in-flight evaluation claims just after an update's discard commits**: it
  read its spend and levels before the update, so it may insert a claim keyed to
  the pre-update period and level, suppressing one notification under the new
  policy. Real rather than theoretical: `CheckBudget` calls `ListBudgetPolicies`
  once, then passes each already-read `*BudgetPolicy` into `evaluatePolicy`, so a
  policy updated mid-loop is evaluated from a stale read. **AC-002.8 carries the
  matching carve-out**, scoping the discard's guarantee to evaluations beginning
  after the update commits. Bounded by one evaluation's duration, and accepted
  rather than locked against: the alternative is a lock spanning the evaluation and
  the update, putting a user-facing write behind a background evaluation. A
  `period` change moves `period_key` and side-steps it entirely; a limit or
  threshold change costs at most one suppressed notification, self-correcting at
  the next period.
- **Backend restart mid-period**: claims are on disk, so a restart changes nothing
  (AC-002.11) — the promise `costs-01.md` already makes and the cron handler's
  in-memory map cannot keep.
- **Policy re-created with the same scope**: a new policy row means a new
  `policy_id` means no claims, so it emits once. Correct — it is a new policy.

## Persistence

- **Table**: one new Office-owned table created in the Office SQLite schema
  initializer, using `CREATE TABLE IF NOT EXISTS` so startup replay is safe. It
  must be created **after** `office_budget_policies` in the initializer, because
  its foreign key references that table: SQLite tolerates a forward reference at
  `CREATE TABLE` and only fails later at DML time, but PostgreSQL rejects it
  outright, so the ordering is a correctness requirement on one dialect and a
  latent trap on the other. Per ADR 0027, the change needs fresh-plus-replay
  coverage on the same connection at the schema owner's boundary, and no new
  local `strings.Contains(err.Error(), ...)` replay classifier —
  `db.IsDuplicateColumnError` / `db.IsAlreadyExistsError` are the only
  classifiers.
- **Deployment / first run** (AC-002.17): the table starts empty, so the first
  evaluation after deployment emits once per policy per reached level, then goes
  quiet for the rest of the period. No backfill runs, and none should be written:
  pre-claiming from existing activity rows would suppress the one notification
  telling the user the new behavior is live, and the activity log carries the
  policy id only inside a JSON `details` string with no index to match on.
- **Policy update**: discard all of the policy's claims on *any* update
  (AC-002.8), unconditionally, without comparing old and new field values.
  `limit_subcents` and `alert_threshold_pct` matter most since they redefine what
  a level means, but `UpdateBudgetPolicy` receives only the new state, so a
  narrower rule would need a read-compare-write against the stored row — both a
  race and a second place for "changed" to drift. The cost is that editing an
  unrelated field (say `action_on_exceed`) re-arms the notification once, which
  is acceptable: a human just edited the policy. A `period` change moves
  `period_key` anyway, so it would re-arm regardless.
- **Policy deletion, all three paths** (AC-002.9): handled by the foreign key's
  `ON DELETE CASCADE`, so **no code change is required at any of them**. All three
  are enumerated because only one goes through `DeleteBudgetPolicy`:
  1. `Repository.DeleteBudgetPolicy` — the single-policy delete behind the HTTP
     route.
  2. The workspace-deletion sweep in `repository/sqlite/workspace_deletion.go`,
     whose existing `DELETE FROM office_budget_policies WHERE workspace_id = ?`
     now cascades. It needs **no** new statement, and must not gain one ordered
     before the policies delete — the cascade already covers it.
  3. `Repository.DeleteBudgetPoliciesForRemovedScopes`, called from
     `infra.Reconciler.reconcileBudgetPolicies` at startup to drop policies
     whose agent or project scope no longer exists. This path bypasses
     `DeleteBudgetPolicy` entirely and would have orphaned claims permanently,
     since [Retention](#persistence) is deliberately none.

  A fourth deletion path added later inherits the cascade for free — the whole
  argument for putting this in the schema rather than a service method, and why
  AC-002.9 is phrased as a property of the row's removal.
- **Retention**: none. Growth is bounded by `policies × periods × 2` (two rows
  per policy per period) and needs no pruning job. Row count is not surfaced.
- **Backups**: covered by the standard Office SQLite snapshot.

## Security

No new trust boundary. Claims are reachable only through a policy, and policies are
already workspace-scoped by `officeWorkspaceScopeMiddleware`. No new HTTP route is
added, so `officeParamScopeResolvers`, `officeScopedSubResourceParams` and
`officeWorkspacelessRoutes` need no entry and `TestOfficeRouteScopeCompleteness`
stays green unmodified. Claims carry no user content, spend figures or PII: a
policy id, a period key, a level, a timestamp.

## Observability

- **Counter** for claim-store failures (AC-002.14), exported through the
  existing `expvar` surface at `/debug/vars` in dev mode. Concretely:

  | | |
  | --- | --- |
  | Name | `budget_claim_failures_total` |
  | Shape | `expvar.NewMap`, matching its siblings — **not** a plain `Int` |
  | Label key | `op`, with exactly one value: `claim` |
  | Home | a new file in **`internal/office/costs`**, which owns `evaluatePolicy` and every increment site |

  The home needs stating because the obvious neighbours sit in a *different*
  package: `cost_events_written_total` / `cost_events_dropped_total` live in
  `internal/office/service/cost_metrics.go`, and their increment helpers are
  unexported methods on `*service.Service`, unreachable from `costs`. So this
  counter sits beside the code that increments it, while following their *shape*
  (label-keyed `expvar.Map`, not Int) because `/debug/vars` is a contract surface
  and a lone Int among Maps would be the odd one out.

  `claim` covers both the level claim and the AC-002.5 companion claim, and the
  label stays at that one value: the log below carries the policy id and level, so
  the counter does not need that cardinality and must not grow it.

  **The in-transaction discard is deliberately NOT counted, and a later reader must
  not "fix" the omission.** Three reasons, the first decisive. (1) A discard failure
  is not silent: AC-002.8 rolls the update back and returns the error, so the person
  who edited the policy sees it fail. This counter is for the failure that *is*
  silent — an evaluation that fails open, continues, and tells nobody (AC-002.14 is
  now scoped to evaluation-time claim reads and writes for exactly this reason). A
  second, weaker signal for a failure that already surfaces would imply the discard
  can fail quietly, which AC-002.8 forbids. (2) A discard spans every period and
  level of one policy, so it has no `level` for the log line below; a sentinel would
  make that field mean two things. (3) It is not reachable from this package: the
  discard runs inside `Repository.UpdateBudgetPolicy` in `office/repository/sqlite`,
  which has no production import of `internal/office/costs`; **both** facades reach
  it and `office/service.Service.UpdateBudgetPolicy` calls `s.repo` directly without
  entering `costs`; and the repository returns one `error` for both halves of the
  transaction, so even from inside `costs` a discard failure is indistinguishable
  from a row-update failure without a typed error this design does not define.
  Counting it would mean an unnamed `sqlite`→`costs` import edge or over-counting
  every failed update — the same two-facade trap
  [Components](costs-03.md#components-and-responsibilities) puts the discard in the repository
  to avoid.

  A non-zero value means notifications are being duplicated and the claim store
  needs attention — it is the only signal that distinguishes "quiet because
  nothing crossed" from "loud because the store is broken".
- **Structured log** at error level on a claim-store failure during an
  evaluation, carrying the policy id and the level being claimed. Both fields are
  always available here, because every increment site is a claim attempt for a
  known policy at a known level.
- **No log line on ordinary suppression.** Suppression is the steady state; a
  log per suppressed evaluation would move the flood from the inbox to the log
  file. The activity log itself is the record of what fired.

## Related decisions

- [ADR 0027: Replayable schema migrations across SQLite and Postgres](../../../decisions/0027-replayable-schema-migrations.md)
