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
# Office: Budget Notification Idempotency System Design (Part 3)

Part 3 holds the contract: what the claim store is, and the control flow that uses
it. Its operational half — suppression boundary, failure and recovery, persistence,
security and observability — is [part 4](costs-04.md).

## Purpose and boundaries

Office owns budget policies, their evaluation, and the `budget.alert` /
`budget.exceeded` rows they produce. This part designs the durable claim state
that turns those two rows from a level check into a crossing notification, per
`REQ-OFFICE-COSTS-002`.

Adjacent contracts this design uses but does not own:

- **Office activity log** (`office_activity_log`, `shared.ActivityLogger`) is the
  emission sink. It is fire-and-forget: `LogActivity` returns nothing, so a
  failed write is not observable to the caller. That constraint drives the
  claim-then-emit ordering below.
- **Office inbox** (`dashboard.DashboardService.inboxBudgetAlertItems`,
  `GetInboxCount`) is the user-visible surface that reads `budget.alert` rows.
  It is unchanged; it simply stops receiving duplicates.
- **Spend rollups** (`Repository.GetCostForAgentSince` /
  `GetCostForProjectSince` / `SumCostsSince`) define the spend window. This
  design reads the window boundary they are given; it does not redefine it.
- **Agent pause** (`shared.AgentWriter.UpdateAgentStatusFields`) and the
  pre-execution gate (`shared.BudgetChecker`) are enforcement, not notification,
  and are deliberately outside the suppression boundary.

## Prior art

Three legs. Two of them are recorded as unavailable rather than skipped.

### Our own prior reasoning (wiki) — searched, unavailable

Receipt: resolved `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`,
`QMD_WIKI_COLLECTION=wiki` from `~/.obsidian-wiki/config` (`@henry`, pinned).
Intended query: idempotent alerting, crossing-versus-level notification semantics.
Neither leg could run — `obsidian-wiki`/`qmd` are not on `PATH` here and the grep
fallback is sandbox-blocked (`ls: /Users/henry/Documents/: Operation not
permitted`). Nothing retrieved, nothing claimed: re-run with vault access before
treating the forks below as unopposed by prior positions.

### What other products shipped (saas-kb) — unavailable

Receipt: the `saas-kb` MCP server and its `search_fsm_docs` tool are absent from
this session's tool list, with no tool discovery exposed. Intended query:
`category: "ai_sdlc"`, budget and spend-limit alerting in agent platforms (Devin,
OpenHands, Warp, Factory.ai, Augment). Nothing retrieved; not substituted.

### In-repo prior art — found, and it decides the key shape

`internal/scheduler/cron/budget.go` already solved this exact problem for a
different notification channel. `BudgetHandler.tryFire` dedupes on

```text
workspace_id | scope_type | scope_id | threshold | period_start
```

and `periodStartFor` anchors that key to the policy's period start so it resets at
a boundary. Its own comment names the gap this design closes: *"Phase 5 keeps
state in-memory for simplicity; a process restart re-fires once per active
crossing [...] A future iteration may persist this."*

We take the key shape and reject two details. The scope tuple becomes
`policy_id`, because two policies can cover one scope (the schema and
`costs-01.md` both allow "monthly + total") and a scope-keyed claim would let one
policy's emission silence the other's. And the state is persisted rather than
in-process, because `costs-01.md` already promises "Monthly reset is idempotent:
backend restart mid-month does not refire", which an in-memory map cannot
deliver (AC-002.11).

We depart once more: the cron handler dedupes on a fixed ladder (50/80/90/100)
ignoring the policy's `alert_threshold_pct`, while this design keys on the two
levels the policy actually defines. The mechanisms stay independent; see the
requirement's `## Out of scope`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COSTS-002` (all ACs) | [Control flow](#control-flow) |
| AC-002.1–.3, .2a, .5, .10 | [Claim-then-emit](#claim-then-emit) |
| AC-002.4, .4a | [Call sites](#call-sites) |
| AC-002.6, .6a, .7, .15 | [Period identity](#period-identity) |
| AC-002.8, .9, .11, .17 | [Persistence](costs-04.md#persistence) (part 4) |
| AC-002.8, .9, .14a | [Repository surface](#repository-surface) |
| AC-002.14 (counter, log), .8 (why the discard is excluded) | [Observability](costs-04.md#observability) (part 4) |
| AC-002.13 | [Evaluation result](#evaluation-result) |
| AC-002.12 | [Suppression boundary](costs-04.md#suppression-boundary) (part 4) |
| AC-002.10, .14, .14a | [Failure and recovery](costs-04.md#failure-and-recovery) (part 4) |
| AC-002.16 | [Evaluation order](#evaluation-order) |

## Components and responsibilities

- **`costs.CostService.evaluatePolicy`** — where all three call sites converge.
  It computes spend and levels exactly as today and gains one responsibility: ask
  the claim store whether this (policy, period, level) may emit, and emit only if
  so.
- **Claim store** — a new Office-owned table plus repository methods on
  `costs.Repository`: atomically claim a (policy, period, level), and discard a
  policy's claims as part of that policy's update. No business logic, no clock,
  and no deletion method — deletion is a schema property, see below.
- **`costs.BudgetCheckResult`** — extended so a caller can distinguish "at or
  above this level" from "this evaluation emitted" (AC-002.13). The existing
  `AlertFired` / `LimitExceed` fields keep their present meaning (level reached),
  because `CheckPreExecutionBudget` gates on `LimitExceed` and must not start
  allowing runs the moment a claim exists.
- **`office/repository/sqlite`** — owns the table DDL in the Office schema
  initializer, the foreign key that deletes claims, and the transaction that
  discards them on update. **The claim lifecycle lives at the persistence layer,
  not at a service method.** This is the load-bearing decision of this design.
  Anchoring it to a service method would be wrong here, because two service
  facades already expose those exact method names —
  `costs.CostService` (`internal/office/costs/budgets.go`) and
  `office/service.Service` (`internal/office/service/service.go`) — and the second
  calls `s.repo` directly, inheriting nothing. Three deletion paths already
  exist, and only one of them goes through `DeleteBudgetPolicy` at all (see
  [Persistence](costs-04.md#persistence)). Anchoring the rule to the row rather than to a
  method is what makes AC-002.8 and AC-002.9 hold for every path, including the
  ones nobody remembers to update.

Nothing changes in the frontend: the inbox, badge count and costs page read the
same rows, just fewer of them.

## Data and contracts

### Claim record

One row per (policy, evaluation period, level).

| Field | Type | Notes |
| --- | --- | --- |
| `policy_id` | string | The budget policy this claim belongs to. Not the scope tuple — see [Prior art](#prior-art). |
| `period_key` | string | Canonical identity of the evaluation period. See [Period identity](#period-identity). |
| `level` | string | Exactly `alert` or `exceeded`. Any other value is a bug, not an extension point. |
| `claimed_at` | timestamp | When recorded. Diagnostic only; never read for a decision. |

`(policy_id, period_key, level)` is the primary key. That uniqueness constraint
is the concurrency control (AC-002.10) — not an advisory lock, not a
read-then-write guard.

`policy_id` carries
`FOREIGN KEY (policy_id) REFERENCES office_budget_policies(id) ON DELETE CASCADE`.
That constraint — not a method — is how AC-002.9 is satisfied, for every deletion
path at once. The pattern is already established in this schema: nine `ON DELETE CASCADE`
foreign keys across seven tables, `office_routine_triggers` and `office_routine_runs` hang off
`office_routines` exactly this way, and the SQLite DSN sets `_foreign_keys=on` for
both the read-write and read-only pools, so the cascade is live rather than
declarative. `labels.go` already relies on the same mechanism in production.

`workspace_id` is intentionally absent: a claim is reachable only through its
policy, which already carries the workspace, and duplicating it would create a
second answer that could drift. The foreign key makes that structural.

### Repository surface

**One** new operation on `costs.Repository`, plus a change to an existing one.
Both are implemented in `office/repository/sqlite`:

- **Claim** `(policy_id, period_key, level) -> (claimed bool, err error)`.
  Inserts the row and reports whether *this* call inserted it. Must be a single
  statement so two concurrent callers cannot both observe "not claimed". Returns
  `claimed=false` with no error when the row already existed.
  **The conflict-resolution form is contract, not local style: use
  `INSERT ... ON CONFLICT(policy_id, period_key, level) DO NOTHING`, and
  specifically NOT `INSERT OR IGNORE`.** The targeted form resolves only the named
  primary-key conflict and lets every other constraint violation surface as an
  ordinary Go `error`, which is what makes the foreign-key rule below implementable
  at all. `claimed` is the statement's **affected-row count**: 1 inserted, 0 already
  existed. That reading holds on both dialects (SQLite's `sqlite3_changes()`
  excludes a row skipped by conflict resolution; PostgreSQL's command tag counts
  only inserted rows), so no `RETURNING` and no follow-up `SELECT` is needed — and a
  `SELECT` would reintroduce the read-then-write race the primary key prevents.
  `INSERT OR IGNORE` instead applies IGNORE resolution to *every* constraint
  violation, foreign keys included: an insert failing only its FK check returns zero
  rows and **no error**, indistinguishable from "already claimed", leaving nothing
  for `db.IsForeignKeyViolation` and making the FK rule below dead code. Worth
  naming because `INSERT OR IGNORE` is the established idiom *in this package* for a
  table of this exact shape (`labels.go` writes `office_task_labels`, which carries
  two `ON DELETE CASCADE` foreign keys, that way), and AC-002.14a's black-box
  outcome converges under both forms, so no test catches the substitution.
  **Foreign-key violation is not a store failure.** An evaluation that listed a
  policy, then raced its deletion, fails the insert on the foreign key because the
  parent row is gone. Treat that specific error as `claimed=false, err=nil` —
  suppress the emission, do not log it, do not increment
  `budget_claim_failures_total`. It is not a degraded store but a policy that no
  longer exists, and none warrants a notification; routing it through AC-002.14's
  fail-open path would emit a `budget.alert` naming a deleted policy and report a
  healthy store as broken. Classify with the shared `internal/db` helpers, never a
  local `strings.Contains` on the message.
- **`Repository.UpdateBudgetPolicy`** gains the discard, *inside* the same
  transaction as the row update, and not as a separate exported method — that is
  the point: both service facades already call this one repository method, so
  putting the discard here is what makes them agree (AC-002.8, closing the second
  half of the two-facade problem in
  [Components](#components-and-responsibilities)). Order within the transaction:
  delete the policy's claims, then update the row. On any error roll back and return
  it; the caller sees a failed update with policy and claims both unchanged.
  `UpdateBudgetPolicy` is today a single untransacted `ExecContext`, so this
  introduces a `BeginTxx` / `Commit` / `Rollback` wrapper matching
  `workspace_deletion.go`'s. The discard is idempotent: deleting zero claim rows is
  success, not an error — a policy that never reached a level has no claims, and
  that is the common case, not the edge case.

There is **no** "discard by policy" and **no** "discard by workspace" method: the
foreign key does both deletions, so an explicit method would be a second, divergent
answer to the same question — the one call sites forget. There is no read-only "is
claimed?" method either; it would invite a check-then-emit call site that
reintroduces the race the primary key exists to prevent.

### Evaluation result

`BudgetCheckResult` gains one boolean per level recording whether this
evaluation **submitted** that level's notification. Existing fields are
unchanged:

- `AlertFired` / `LimitExceed` — the policy reaches that level *now*. Level
  check. Used for gating.
- the two new fields — this evaluation handed that level's row to the activity
  logger. Crossing. Used for assertions and for any future caller that wants to
  react only to new crossings.

A field reports **submission only**; holding the claim is the mechanism that
decides whether to submit and is not part of what the field says (AC-002.13
enumerates all four reachable cases). Two of them are counter-intuitive and are
why this is spelled out: the AC-002.5 companion claim is *held* while the
alert-level field reads **false** (no `budget.alert` row went out), and a
fail-open emission under AC-002.14 holds *no* claim while the field reads **true**
(a row did). Reading the field as "held the claim and submitted" gets that second
case backwards and hides every fail-open emission from the callers the field
exists to serve.

The word is **submitted**, not *emitted* (AC-002.13, and the `Emit` / `Submit`
split in the requirement's Terminology). `LogActivity` returns nothing, so the
evaluation cannot observe whether its row became durable. A field documented as
"emitted" would promise a durability guarantee AC-002.2a says is unavailable, and
a caller would be entitled to believe it.

## Control flow

### Period identity

`period_key` is derived from the same window boundary the policy's spend is
computed against, not from an independently-chosen calendar rule. The evaluation
reads the clock **once** and derives that boundary **once**: `evaluatePolicy`
calls `periodCutoff` with a single evaluation timestamp, hands the resulting
instant to the spend rollup, and formats that same instant as `period_key`
(AC-002.6a).

**This requires a contract change at an internal seam.** Today
`getSpendForPolicy` calls `periodCutoff(string(policy.Period), time.Now())`
itself and returns only `(int64, error)`, so the instant is discarded before
`evaluatePolicy` regains control and is unreachable from the claim site. The
boundary must move up to the caller — either `evaluatePolicy` computes it and
passes it in, or `getSpendForPolicy` returns it alongside the spend. Either
satisfies AC-002.6a; leaving the seam alone does not.

Recomputing the boundary at claim time is **prohibited**, and is named because it
is the path of least resistance: a second `periodCutoff(period, time.Now())`
compiles, reads naturally, and agrees with the first except exactly at a boundary
— spend summed against September, claim written against October — re-arming the
policy once per boundary forever, invisibly in steady state. That is why AC-002.6
is phrased as an equality rather than as a calendar:

- A non-zero window start renders as that instant formatted RFC3339 in UTC
  (`2026-09-01T00:00:00Z`). The layout is fixed and is part of the contract: it
  is a stored key, so changing the layout later orphans every existing claim and
  re-fires every policy once.
- A zero window start — `periodCutoff`'s "no filter / lifetime" answer — renders
  as the literal `lifetime`. It is not the RFC3339 rendering of the zero time,
  because that reads as a real instant and would be silently reset by any future
  change that starts passing a real epoch-anchored floor.

The consequence is deliberate. A `daily` or `yearly` policy today gets a lifetime
spend window from `periodCutoff`, so it also gets a non-resetting claim: it
notifies once, ever, consistent with a limit that never resets. A calendar-derived
key would instead reset the claim daily against a spend total that never resets,
re-firing forever on the same accumulated spend — this requirement's own defect,
reintroduced through the back door. The divergence between the three period
functions is real and is named in the requirement's `## Out of scope`; this design
refuses to depend on resolving it.

Because the key is the window start, a new window is automatically unclaimed
(AC-002.7), and no scheduled job, cleanup pass, or reset trigger exists or is
needed.

### Claim-then-emit

Within `evaluatePolicy`, for whichever level the spend reaches:

1. Compute spend and levels exactly as today.
2. If the spend reaches the **limit** level: claim `exceeded`; then also claim
   `alert` (AC-002.5). Run the `pause_agent` side effect regardless — see
   [Suppression boundary](costs-04.md#suppression-boundary). `Claim` returns
   `(claimed bool, err error)`, so the `exceeded` claim has **three** outcomes and
   each decides the emission differently. Do not collapse them to two:
   - `claimed=true, err=nil` — this evaluation won the claim: **emit**.
   - `claimed=false, err=nil` — an earlier evaluation already holds it, or the
     policy was deleted mid-evaluation (AC-002.14a): **do not emit**.
   - `err != nil` — the claim store is degraded: **emit anyway**, and log and count
     it (AC-002.14, [Observability](costs-04.md#observability)). Suppressing here is
     the one outcome AC-002.14 exists to prevent, and it is what any two-state
     summary of this step ("emit only if the claim succeeded") silently produces,
     which is why all three outcomes are written out instead.
   The companion `alert` claim's **outcome** is discarded — claimed and
   already-claimed are equally fine, and neither changes whether
   `budget.exceeded` is emitted. Its **error** is not discarded: a claim-store
   error here is logged and counted exactly like any other (see
   [Observability](costs-04.md#observability)), because it is the same degraded
   store AC-002.14 exists to make visible. It still does not change the
   `budget.exceeded` emission, and it is not retried. Naming this matters
   because the consequence is specific and otherwise silent: if the companion
   claim errors, the de-escalation hole below reopens **for that period only** —
   a later evaluation landing in the alert band emits one `budget.alert` — and
   the counter is the only signal that this is what happened. **AC-002.5 carries
   the matching carve-out**: its "shall also record" is conditional on the store
   accepting the companion write, with AC-002.14 taking precedence when it does
   not. So a test of AC-002.5 asserts the companion claim on a **healthy** store,
   and a fault-injection test must not assert it was recorded — the same split
   AC-002.10 and AC-002.14 already require.
3. Else if the spend reaches the **alert** level: claim `alert`, and decide the
   `budget.alert` emission from the same three outcomes as step 2.
4. Else: no claim, no emission. Note that a spend that has fallen back below a
   claimed level does **not** release the claim (AC-002.15); nothing in this
   flow ever deletes a claim.

The order is claim first, emit second, and that ordering is forced rather than
preferred. `ActivityLogger.LogActivity` returns nothing, so emit-then-claim
cannot know whether it has anything to claim, and a claim written after a failed
emit would silence a notification that never happened. Claim-then-emit has the
opposite failure: a successful claim whose emission then fails loses exactly one
notification for that period. That is the accepted trade — one lost notification
under an already-degraded activity log, versus a flood under normal operation,
which is the defect being fixed.

Step 2's unconditional alert-claim closes the de-escalation hole: without it, a
policy whose spend jumps straight past the limit never claims `alert`, so a
later reassignment that drags spend back into the band between the alert level
and the limit would emit a `budget.alert` — a *new* notification describing a
*reduction* in spend.

### Call sites

All three converge on `evaluatePolicy`, so all three inherit the behavior with
no per-caller logic (AC-002.4):

| Caller | Trigger | Path |
| --- | --- | --- |
| `Service.CheckBudget` (post-cost-event subscriber, `service/event_subscribers.go`) | every recorded cost event | `EvaluateBudget` → `CheckBudget` → `evaluatePolicy` |
| `SchedulerIntegration.checkBudget` (pre-execution gate, `service/scheduler_integration.go`) | every run dispatch | `CheckPreExecutionBudget` → `CheckBudget` → `evaluatePolicy` |
| `DashboardService` task reassignment (`dashboard/service_tasks.go`, added by PR #3276) | every reassignment into a project | `EvaluateProjectBudget` → `evaluatePolicy` |

`EvaluateProjectBudget` does not exist at this branch's merge base (`c51ec0a21`);
PR #3276 is open against `main`. Apply the behavior at `evaluatePolicy`, which is
present today, and add the reassignment coverage AC-002.4 names once that call
site exists — the reassignment row above is the only deferred part of this design,
and because the behavior lives at `evaluatePolicy` rather than per caller, that
call site inherits it the moment it lands. The deferred item is the **test
coverage**, not the behavior.

The deferral has a named, auditable home (AC-002.4a): a heading in the **task
plan** reading exactly

```text
## DEFERRED: AC-OFFICE-COSTS-002.4 reassignment coverage
```

naming the absent symbol (`EvaluateProjectBudget`) and the PR it waits on (#3276).
The task plan is this board's running record — it is the Kandev artifact behind
`get_task_plan_kandev` / `update_task_plan_kandev`, not a file in the repository,
it survives context resets, and every later step reads it. A code comment is
explicitly **not** sufficient: nothing on this board reads code comments, which is
how a deferral becomes a silent drop. While that heading is present, AC-002.4 is
not fully closed and no step may report it as such.

### Evaluation order

`Repository.ListBudgetPolicies` currently orders by `created_at` alone, which is
not unique: two policies created in the same second have no defined relative
order, so which emits first can differ between runs. The ordering becomes
`created_at ASC, id ASC` (AC-002.16); `id` is the primary key, so the ordering is
total. This matters because each policy's emission is now one-shot per period, so
a non-deterministic order makes the activity-log sequence non-reproducible and
any test asserting on it flaky rather than wrong.

---

Enforcement boundaries, failure handling, persistence, security and observability
are designed in [part 4](costs-04.md). They are split out because this design
reached the specification linter's 32,768-byte ceiling for a system-design file;
the two parts are one design and part 4 has no requirement mapping of its own.
