---
status: draft
system: office
requirements:
  - REQ-OFFICE-BUDGET-001
  - REQ-OFFICE-BUDGET-002
  - REQ-OFFICE-BUDGET-003
  - REQ-OFFICE-BUDGET-004
  - REQ-OFFICE-BUDGET-005
  - REQ-OFFICE-BUDGET-006
  - REQ-OFFICE-BUDGET-007
created: 2026-09-07
owners:
  - kandev
---

# Office: Pre-Launch Budget Enforcement System Design

## Purpose and boundaries

This design replaces the three fail-open paths named in
[budget-enforcement.md](../requirements/budget-enforcement.md) with a five-gate
admission decision that runs inside `SchedulerIntegration.processRun`
(`internal/office/service/scheduler_integration.go`), immediately after task
checkout and before executor resolution. It owns: run-provenance
classification, the gate sequence and its dispositions, per-state activity
entries and counters, measurement-window and pricing-degradation correctness,
and a new durable per-workspace default-ceiling setting. It does not change the
post-event evaluation path (`costs.CostService.EvaluateBudget`,
`costs/budgets.go`), which continues to fire `budget.alert` / `budget.exceeded`
and pause agents on its own schedule, unconstrained by this design.

Adjacent contracts used and not owned:

- `internal/office/service/retry.go` — `MaxRetryCount`, the backoff schedule,
  `isRetryStale`/`retryMaxAge`. Reused, not reimplemented.
- `internal/office/service/scheduler_runs.go` — `FinishRun`, `FailRun`,
  `transitionRunTerminal`, `releaseTaskCheckoutForRun`.
- `internal/office/service/activity.go` — `LogActivityWithRun`.
- `internal/office/repository/sqlite/costs.go` — cost-event and
  budget-policy storage; extended, not replaced.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-BUDGET-007` | [Provenance classification](#provenance-classification) |
| `REQ-OFFICE-BUDGET-001` | [The five-gate admission decision](#the-five-gate-admission-decision) |
| `REQ-OFFICE-BUDGET-006` | [Two-phase policy evaluation](#two-phase-policy-evaluation), [Inertness](#inertness) |
| `REQ-OFFICE-BUDGET-002` | [Measurement integrity](#measurement-integrity) |
| `REQ-OFFICE-BUDGET-004` | [Pricing degradation](#pricing-degradation) |
| `REQ-OFFICE-BUDGET-003` | [Built-in default ceiling](#built-in-default-ceiling) |
| `REQ-OFFICE-BUDGET-005` | [Observability: activity entries and counters](#observability-activity-entries-and-counters) |

## Provenance classification

New file `internal/office/shared/runprovenance.go`, beside the existing
`IsPeriodicTasklessWake` this design deliberately does not touch (per
[budget-run-provenance.md](../requirements/budget-run-provenance.md)'s Out of
scope and the glossary's polarity warning):

```go
type RunProvenance int

const (
    RunProvenanceUnattended RunProvenance = iota
    RunProvenanceAttended
)

func ClassifyRunProvenance(reason string) RunProvenance
```

An unexported `map[string]struct{}` literal holds the 20-literal allowlist of
`AC-OFFICE-BUDGET-007.2` verbatim (16 current + 4 legacy). `processRun` calls
this exactly once, before gate 1, and threads the result through every gate
via a small `admissionCtx` struct (see below) rather than recomputing it —
satisfying `AC-OFFICE-BUDGET-001.1`'s "computed once, never revised".

### Build-time completeness test

`internal/office/shared/runprovenance_completeness_test.go`
(`package shared_test`) implements `AC-OFFICE-BUDGET-007.3` using plain
`go/parser`/`go/ast` per file, never a line-oriented regex — the X2 design
note. This is an implementation-time refinement from the originally-planned
`golang.org/x/tools/go/packages` + `go/types`: that combination is an
unnecessary new direct dependency (currently only transitive in `go.sum`)
and spawns `go list` subprocesses under the hood, which is a source of CI
fragility this test doesn't need to accept. The AC's actual requirements —
never regex/identifier-matching, fail hard on a missing block, an alias
contributes no new literal — hold under the simpler approach without
weakening what the test proves; see below for how alias handling differs.

For each of the seven blocks the test is hard-coded with **(file path
relative to the test file via `runtime.Caller(0)`, anchor constant name)**:
the anchor is one identifier already known to live in that block
(`RunReasonTaskAssigned` for (a) in `scheduler/run.go`,
`RunReasonTaskAssigned` for (b1) in `service/run.go`,
`legacyRunReasonBlockersResolved` for (b2) in the same file,
`RunReasonRoutineDispatch` for (c) in `shared/runreasons.go`,
`WakeReasonHeartbeat` for (d) in `routing/types.go`, `TaskCommentReason` for
(e) in `runs/commentkeys/commentkeys.go`, `runReasonTaskAssigned` for (f) in
`onboarding/service.go`, `RunReasonManualResumeAfterFailure` for (g) in
`service/failure.go`). For each anchor the test parses that one file with
`go/parser` (`parser.ParseFile`, `SkipObjectResolution`), finds the
top-level `*ast.GenDecl` with `Tok == token.CONST` containing a
`*ast.ValueSpec` naming the anchor, and fails the test (not skips) when no
such declaration exists — an enumerated block that was renamed, moved, or
deleted must fail the build rather than silently shrink the inventory.
Blocks (b1) and (b2) both resolve to `service/run.go`; naming the file per
block (rather than per package) is also what disambiguates block (g), which
shares a package with (b) but lives in a different file (`failure.go` vs
`run.go`).

Once a block's `GenDecl` is located, every `ValueSpec` in it whose RHS is a
string `*ast.BasicLit` contributes that literal (via `strconv.Unquote`); a
`ValueSpec` whose RHS is anything else — a `*ast.SelectorExpr` or
`*ast.Ident`, i.e. an alias like `RunReasonHeartbeat =
shared.RunReasonHeartbeat` — is skipped rather than resolved. This is
sufficient, not a weaker substitute for `go/types` resolution: an aliased
constant's underlying literal is itself already covered by scanning its own
enumerated block, so skipping the alias syntactically loses no literal from
the union. Names present in the fixed six-entry exclusion list of
`AC-OFFICE-BUDGET-007.3` are dropped before dedup/membership checks.

The test asserts, over the union of all seven blocks minus exclusions:
every resolved string is a member of exactly one of two Go string sets
declared directly in the test file — `attendedForTest` (the 20 literals of
`AC-OFFICE-BUDGET-007.2`, kept textually identical to
`shared.attendedRunReasons`, not derived from it, so the assertion is not
checking the production map against itself) and `unattendedForTest` (the 6
literals of `AC-OFFICE-BUDGET-007.4`, maintained as its own literal slice —
never `allReasons - attendedForTest`, which is what makes the test capable of
failing at all). A literal in both, or in neither, fails with the offending
block and literal named in the message.

## The five-gate admission decision

New file `internal/office/service/budget_admission.go`. `processRun` calls
one entry point immediately after `checkoutTask` succeeds, replacing today's
`checkBudget`:

```go
// admitRun runs AC-OFFICE-BUDGET-001.14's five gates in order and returns
// true only when the run should launch. False means admitRun has already
// driven the run to its terminal, retry, or cancelled disposition.
func (si *SchedulerIntegration) admitRun(ctx context.Context, run *models.Run, taskID string) bool
```

`admitRun` builds one `admissionCtx` carrying: the provenance
(`shared.ClassifyRunProvenance(run.Reason)`, computed once), the captured
evaluation instant (`time.Now().UTC()`, captured once — `AC-OFFICE-BUDGET-001.9`'s
"one captured evaluation instant"), and threads it through gates 1-5. Each
gate returns a small `(admit bool, terminal bool)` — `terminal=false` with
`admit=false` means "deferred; caller must return", so a single `if !gate() {
return false }` chain at the call site reproduces `AC-OFFICE-BUDGET-001.14`'s
"first gate that yields a non-launch decision, no later gate evaluated"
structurally, without a shared mutable "have we decided" flag that a future
edit could reorder around.

1. **Workspace resolution** (`gateWorkspace`). Reuses the *existing*
   `GetAgentFromConfig` call already made earlier in `processRun` (per
   `AC-OFFICE-BUDGET-001.14`'s "constrains the disposition of the workspace
   resolution wherever it already occurs rather than requiring a second
   one") — `admitRun` receives the already-resolved `*models.AgentInstance`
   from its caller instead of re-fetching it. Per the **W2 human
   disposition**, `GetAgentFromConfig`'s error contract is not reshaped: an
   error from this call (already handled by `processRun` before `admitRun`
   is even reached — see [Control flow](#control-flow-inside-processrun))
   defers under the same terms as gate 3. `admitRun` itself only sees the
   success case and applies `AC-OFFICE-BUDGET-001.13`'s empty-workspace
   branch: `agent.WorkspaceID == ""` cancels (not deferred), writing the
   `run_budget_workspace_unresolvable` activity entry.
2. **Evaluator presence** (`gateEvaluatorPresence`). `si.svc.budgetChecker ==
   nil`: unattended cancels
   (`run_budget_no_evaluator`), attended launches (falls through).
3. **Evaluator invocation** (`gateEvaluatorInvocation`). Calls the evaluator
   once via the new richer method (below). A non-nil error is an evaluator
   fault: `admitBudgetDeferral` (shared with gates 4/5's fault paths, see
   below) defers under `AC-OFFICE-BUDGET-001.3`/`.16`, or fails at
   `MaxRetryCount` under `AC-OFFICE-BUDGET-001.4`, tagged
   `budgetDeferralCauseEvaluatorFault`.
4. **Applicable policies** (`gateAppliedPolicies`). Resolves the run's
   project id per `AC-OFFICE-BUDGET-006.7` (see
   [Project resolution](#project-resolution)) then delegates the full
   evaluate-then-select to `costs.CostService.EvaluatePreLaunch` (see
   [Two-phase policy evaluation](#two-phase-policy-evaluation)).
5. **Built-in default** (`gateBuiltInDefault`). Only reached for an
   unattended run whose gate 4 result says no workspace-scoped
   blocking-capable `daily` policy was evaluated (superseding it per
   `AC-OFFICE-BUDGET-003.4`). Evaluated by the same
   `costs.CostService.EvaluateDefaultCeiling` call gate 4 already used to
   decide supersession — see [Built-in default ceiling](#built-in-default-ceiling).

Steps 3-5 share one disposition helper,
`(si *SchedulerIntegration) admitBudgetDeferral(ctx, run, cause
budgetDeferralCause) bool`, which increments `run.RetryCount` through the
*existing* `s.scheduleRetry`/`isRetryStale` machinery (so
`AC-OFFICE-BUDGET-001.16`'s "same `runs.retry_count`" and "no sooner than the
first backoff delay" hold for free) but, unlike `HandleRunFailure`, never
calls `escalateFailure`. See [MaxRetryCount without
escalation](#maxretrycount-without-escalation) for why.

### Control flow inside `processRun`

```text
checkoutTask succeeds
  -> admitRun(ctx, run, taskID, agent)   // replaces checkBudget
       gate1 workspace         (reuses already-fetched agent)
       gate2 evaluator presence
       gate3 evaluator invocation
       gate4 applicable policies (resolves project id first, AC-006.7)
       gate5 built-in default
  -> (unchanged) resolveExecutorForRun, prepareAndLaunch
```

`GetAgentFromConfig`'s pre-existing error branch in `processRun` (today:
`HandleRunFailure`) is left as-is for the **non-budget** failure it already
handles (agent truly not found); `AC-OFFICE-BUDGET-001.13`'s deferred-vs-cancel
split for a workspace-lookup **error** specifically is therefore also
satisfied by that existing branch, because `GetAgentFromConfig` returning an
error already routes through `HandleRunFailure` → retry → escalate, which
*is* the "same terms as an evaluator fault" `AC-OFFICE-BUDGET-001.13`
requires — except escalation: `HandleRunFailure`'s existing
`escalateFailure` call queues a CEO run, which
[MaxRetryCount without escalation](#maxretrycount-without-escalation)
requires this capability *not* do for a budget-caused permanent failure.
Because a workspace-lookup error is indistinguishable, at this call site,
from every other reason `GetAgentFromConfig` can fail (the W2 disposition:
"do not reshape `GetAgentFromConfig`'s error contract"), this design accepts
that a workspace lookup failing at `MaxRetryCount` escalates through the
pre-existing generic path rather than the budget-specific
non-escalating one — an intentional, narrow exception to
`AC-OFFICE-BUDGET-001.17`'s "no path" language, justified by W2's own text
("do not reshape... here") and recorded so a later reviewer does not
"fix" it by widening `GetAgentFromConfig`'s contract mid-capability.

### Project resolution

`AC-OFFICE-BUDGET-006.7` needs four outcomes `extractProjectID` cannot
produce (it collapses all four to `""`). New function in
`budget_admission.go`:

```go
type projectResolution int

const (
    projectResolutionNone projectResolution = iota // (a) no fault, no project
    projectResolutionFound
    projectResolutionLookupError   // (b) defer, AC-006.3
    projectResolutionUnparseable   // (c) cancel
    projectResolutionTaskNotFound  // (d) cancel
)

func (si *SchedulerIntegration) resolveRunProject(ctx context.Context, payload string) (projectID string, res projectResolution)
```

Parses `payload` with `json.Unmarshal` directly (not `ParseRunPayload`, which
swallows unmarshal errors — see budget-admission-integrity.md pinned
evidence) into a small local struct with a `TaskID` field. A parse error is
outcome (c). An empty/absent `task_id` is outcome (a). Otherwise calls
`si.svc.repo.GetTaskBasicInfo`: an error is (b); `info == nil` is (d);
`info.ProjectID` (possibly `""`, still (a)/(found)) otherwise.

## Two-phase policy evaluation

`costs.CostService` gains one new exported method,
`EvaluatePreLaunch(ctx, workspaceID, agentInstanceID, projectID string, hasProject
bool, at time.Time) (PreLaunchResult, error)`, in a new file
`internal/office/costs/prelaunch.go`. It does **not** call the existing
`CheckBudget`/`evaluatePolicy` (those remain the post-event path's, unchanged,
per `AC-OFFICE-BUDGET-006.2`). It:

1. Lists policies for the workspace, filters to applicable ones per
   `AC-OFFICE-BUDGET-001.15` (workspace-scoped always applies; agent-scoped
   only when `scope_id == agentInstanceID`; project-scoped only when
   `hasProject && scope_id == projectID` — the caller's `hasProject` flag,
   from `resolveRunProject`, is what keeps a genuinely-project-less run from
   ever matching a project policy, distinct from a resolution failure that
   never reaches this call at all).
2. For each applicable policy, validates it: period recognized (`daily`,
   `monthly`, `yearly`, `total`), `limit_subcents > 0`, `action_on_exceed`
   recognized, and (for `agent`/`project` scope) non-empty `scope_id`. A
   malformed policy is **skipped** — recorded via the caller's
   per-policy-per-day-deduped activity entry (see
   [Observability](#observability-activity-entries-and-counters)) — and
   excluded from both ordering and default-supersession. This is
   `AC-OFFICE-BUDGET-002.5`/`.11`/`.14`; it is explicitly not an "unevaluated
   policy".
3. For each surviving (non-skipped) policy, computes priced spend and the
   pricing-degradation determination over the *same* scope-selected event set
   (`AC-OFFICE-BUDGET-002.15` — see [Measurement
   integrity](#measurement-integrity)) for its own period window ending at
   `at`. A query error here makes the **whole call** return a non-nil error —
   this is what makes `AC-OFFICE-BUDGET-006.1` (an unevaluated policy faults
   the run, not a partial admission) true by construction rather than by a
   caller remembering to check a length mismatch: there is no code path that
   returns a partial `[]PreLaunchPolicyResult` and `err == nil`.
4. Orders the surviving policies by `(created_at ASC, id ASC)` and, in that
   order, finds the first whose limit test (`spend >= limit`, action ∈
   {`pause_agent`,`block_new_tasks`}) or degradation test (unattended,
   `2*spend >= limit`, degraded) fires — but **every** surviving policy's
   both tests are computed up front (step 3), so "first" is a selection over
   an already-fully-evaluated slice, never a short-circuiting loop.
5. Returns `PreLaunchResult{Decision, Policies []PreLaunchPolicyResult,
   WorkspaceDailyBlockingSuperseded bool}` — the last field tells
   `gateBuiltInDefault` whether a workspace-scoped, non-skipped, blocking-capable
   (`pause_agent`/`block_new_tasks`) `daily` policy existed among the
   surviving set, independent of whether it fired
   (`AC-OFFICE-BUDGET-003.4` supersedes by existing, not by blocking).

`PreLaunchResult` never triggers `budget.alert`/`budget.exceeded` or an agent
pause — it has no side effects at all, which is what makes
`AC-OFFICE-BUDGET-006.2` (inertness) hold structurally: the method's only
outputs are its two return values.

### Inertness

`EvaluatePreLaunch` and every function it calls are pure reads:
`ListBudgetPolicies`, the two new spend-selection queries below, and
(read-only) `GetWorkspaceBudgetDefault`. None of them writes
`office_activity_log`, flips `agent_profiles.status`, or calls
`UpdateAgentStatusFields`. `pauseAgentForBudget` and `logBudgetAlert`/
`logBudgetExceeded` (existing `costs/budgets.go`) are not imported by
`prelaunch.go`. Activity-entry writing for the *new* budget states happens
one layer up, in `budget_admission.go`, after `EvaluatePreLaunch` returns —
so a policy that decided the run's fate and a policy that merely degraded
both get their entries from the caller, never from the evaluator itself.

## Measurement integrity

### Period fix

`costs/budgets.go`'s `periodCutoff` is replaced by a function on the new
`prelaunch.go` (used by both the pre-launch path and, unchanged in
signature, still callable from the existing post-event path if a later
change wants it — not required here):

```go
func windowStart(period models.BudgetPeriod, at time.Time) (start time.Time, ok bool)
```

`daily` → most recent UTC midnight ≤ `at`; `monthly` → existing behavior;
`yearly` → 1 Jan UTC of `at`'s year; `total` → `ok=true`,
zero `time.Time` (unbounded below, `AC-OFFICE-BUDGET-002.8` — selected by
name, not by falling through). An unrecognized period returns `ok=false`,
which callers treat as "skip this policy" (`AC-OFFICE-BUDGET-002.5`), never
as lifetime.

`models.BudgetPeriod` gains `BudgetPeriodTotal = "total"` in
`internal/office/models/enums.go`, added to `Valid()`. `costs/handler.go`'s
two existing `period.Valid()` checks (create/update) need no change — they
already delegate to the enum.

### Scope-selection queries (`AC-OFFICE-BUDGET-002.15`)

Two new repository methods in `costs.go`, used by **both** the priced-spend
sum and the degradation check (same `WHERE`, different `SELECT`), and by
neither `SumCostsSince` nor `GetCostForProjectSince` — the **W4 human
disposition** explicitly forbids reuse:

```go
// SpendWindow reports priced spend and whether the window contains any
// unpriced event, for one scope, in one query round-trip.
type SpendWindow struct {
    PricedSubcents int64
    Degraded       bool
}

func (r *Repository) SpendWindowForWorkspace(ctx, workspaceID string, start time.Time, hasStart bool, before time.Time) (SpendWindow, error)
func (r *Repository) SpendWindowForAgent(ctx, agentInstanceID string, start time.Time, hasStart bool, before time.Time) (SpendWindow, error)
func (r *Repository) SpendWindowForProject(ctx, projectID string, start time.Time, hasStart bool, before time.Time) (SpendWindow, error)
```

Workspace scope joins `office_cost_events e JOIN agent_profiles a ON
a.id = e.agent_profile_id WHERE a.workspace_id = ?` — never `tasks`, closing
the 138-orphan / 973.76 USD gap `SumCostsSince`'s inner join produces (W4
pinned evidence). Agent scope filters `e.agent_profile_id = ?` directly
(already correct in `GetCostForAgentSince`, but not reused, per W4). Project
scope filters `e.project_id = ?` directly — the column already exists on
`office_cost_events` (`base.go:281`), so unlike the legacy
`GetCostForProject`'s live-task-project join, no query against `tasks` is
needed at all, and reparenting a task cannot move historical spend between
ceilings (`AC-OFFICE-BUDGET-002.15`'s "recorded on the cost event itself...
not... the current project of the event's task"). `hasStart=false` (the
`total` period) omits the lower bound. Every query also filters `e.occurred_at
< ?` (`before`, the captured evaluation instant) — the half-open
`[cutoff, now)` window of `AC-OFFICE-BUDGET-002.10`, closing the
clock-skew/back-dated-write gap. `Degraded` is `COUNT(*) FILTER (WHERE
e.cost_source = 'unpriced') > 0` (SQLite: `SUM(CASE WHEN cost_source =
'unpriced' THEN 1 ELSE 0 END) > 0`), never reading `estimated`.

## Pricing degradation

`gate4`/`gate5`'s per-policy test, applied identically to policies and the
default (`AC-OFFICE-BUDGET-003.11`):

```go
func degradationBlocks(degraded bool, pricedSubcents, limitSubcents int64, provenance shared.RunProvenance) bool {
    return degraded && provenance == shared.RunProvenanceUnattended && 2*pricedSubcents >= limitSubcents
}
```

The `2*x >= limit` form is `AC-OFFICE-BUDGET-004.3`'s exact-50%-without-
truncation requirement. `degradationBlocks` overrides `notify_only` (it is
called regardless of action, per `AC-OFFICE-BUDGET-004.3`'s "applies whatever
the policy's action"). When it fires and the plain limit test
(`AC-OFFICE-BUDGET-001.7`) did **not** also fire, the run's outcome is
`RunOutcomeBudgetUnmeasurable` (new constant, `"budget_unmeasurable"`,
`internal/office/service/run.go`) rather than `RunOutcomeBudgetBlocked` —
`AC-OFFICE-BUDGET-005.7`. When both fire, outcome stays `budget_blocked` and
the activity entry alone carries the degradation flag
(`AC-OFFICE-BUDGET-004.6`).

## Built-in default ceiling

New table, added to `createCostTables()` (`base.go`) beside
`office_budget_policies` — a genuinely new table, not a column on an existing
one, because `AC-OFFICE-BUDGET-003.7` requires the setting be addressed by a
stable identifier distinct from any `office_budget_policies` row and never
appear in that table's list results:

```sql
CREATE TABLE IF NOT EXISTS office_budget_default_settings (
    workspace_id    TEXT PRIMARY KEY,
    limit_subcents  INTEGER NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);
```

No migration needed — a brand-new table's `CREATE TABLE IF NOT EXISTS` is
idempotent on both fresh and existing databases (backend `CLAUDE.md`'s
migration rule applies to adding a *column* to an *existing* table, not to a
new table). `costs.go` gains `GetWorkspaceBudgetDefault(ctx, workspaceID)
(int64, error)` (returns `DefaultCeilingSubcents = 500_000` when no row
exists — `AC-OFFICE-BUDGET-003.9`, never an error for "absent") and
`SetWorkspaceBudgetDefault(ctx, workspaceID string, limitSubcents int64)
error` (`INSERT ... ON CONFLICT(workspace_id) DO UPDATE SET
limit_subcents = excluded.limit_subcents, updated_at = excluded.updated_at`
— single-statement last-write-wins, `AC-OFFICE-BUDGET-003.10`, no
read-modify-write race). `CostService` exposes both; `Handler` adds two
routes on the existing budget route group:

```text
GET  /workspaces/:wsId/budgets/default   -> {"limit_subcents": N}
PUT  /workspaces/:wsId/budgets/default   -> validates > 0 (else 400, AC-003.8), writes, echoes
```

`gateBuiltInDefault` calls `EvaluateDefaultCeiling(ctx, workspaceID, at
time.Time) (PreLaunchPolicyResult, error)` (same file, same shape as a
single-policy `EvaluatePreLaunch` result, `PolicyID=""`,
`IsDefault=true`), which reads the effective limit, then evaluates it exactly
like a `daily` + `block_new_tasks` policy using
`SpendWindowForWorkspace` — the same query gate 4's workspace-scoped
policies use, so a policy and the default cannot disagree about which events
count in the same window (`AC-OFFICE-BUDGET-001.9`'s "same captured
instant" applies transitively because both gates share `admissionCtx.at`).

## MaxRetryCount without escalation

`AC-OFFICE-BUDGET-001.17` forbids a budget-caused permanent failure from
queuing any new run, "in particular... an escalation run for another agent".
The existing `HandleRunFailure` → `escalateFailure` path queues exactly that
(`queueCEOAgentError`). `admitBudgetDeferral`'s terminal branch therefore
does **not** call `s.escalateFailure`; it calls a new
`(s *Service) failRunNoEscalation(ctx, run *models.Run, cause
budgetDeferralCause) error` (`retry.go` sibling in `budget_admission.go`)
that performs exactly `escalateFailure`'s non-escalating half: `s.FailRun`
plus one `LogActivityWithRun` naming the cause
(`AC-OFFICE-BUDGET-006.4`/`.5`) — no `GetAgentFromConfig` re-fetch for
escalation purposes, no `queueCEOAgentError` call. This is applied uniformly
to all four budget-deferral causes reaching `MaxRetryCount` — the evaluator
fault itself (`AC-OFFICE-BUDGET-001.4`) and the three `AC-OFFICE-BUDGET-006.4`
causes (workspace-lookup error, unevaluated policy, failed project lookup) —
because `AC-OFFICE-BUDGET-006.4` states they "defer... under
`AC-OFFICE-BUDGET-001.3`'s terms", and `AC-OFFICE-BUDGET-001.4` is the
`MaxRetryCount` disposition `AC-OFFICE-BUDGET-001.3` names; treating the
escalation exclusion as applying only to the literally-numbered `.4` while
letting the other three self-escalate a CEO agent would silently reopen the
launch-on-persistent-fault gap this capability exists to close. Documented
here as the resolving reading of an otherwise-textually-narrow cross-reference,
per Build's instruction to record a documented assumption rather than route
back to Spec for a wording gap.

## Observability: activity entries and counters

Every new state writes an `office_activity_log` row via
`LogActivityWithRun` with a distinct `Action` value — `action` is the
existing queryable, un-parsed column
`AC-OFFICE-BUDGET-005.3`/`AC-OFFICE-BUDGET-006.5` require:

| State | `action` | Names policy? | Counter |
| --- | --- | --- | --- |
| Evaluator fault, deferred | `run_budget_evaluator_fault_deferred` | no | `office_budget_deferred_evaluator_fault_total{provenance}` |
| Evaluator fault, failed at max retry | `run_budget_evaluator_fault_failed` | no | `office_budget_failed_evaluator_fault_total{provenance}` |
| No evaluator wired (unattended) | `run_budget_no_evaluator` | no | `office_budget_blocked_no_evaluator_total{provenance}` |
| Workspace lookup errored, deferred | `run_budget_workspace_lookup_deferred` | no | `office_budget_deferred_workspace_lookup_total{provenance}` |
| Workspace lookup failed at max retry | `run_budget_workspace_lookup_failed` | no | `office_budget_failed_workspace_lookup_total{provenance}` |
| Workspace unresolvable, cancelled | `run_budget_workspace_unresolvable` | no | `office_budget_cancelled_no_workspace_total{provenance}` |
| Stale deferral, cancelled | `run_budget_deferral_stale_cancelled` | no | `office_budget_cancelled_stale_total{provenance}` |
| Unevaluated policy, deferred | `run_budget_unevaluated_policy_deferred` | **yes** | `office_budget_deferred_unevaluated_policy_total{provenance}` |
| Unevaluated policy failed at max retry | `run_budget_unevaluated_policy_failed` | **yes** | `office_budget_failed_unevaluated_policy_total{provenance}` |
| Project lookup errored, deferred | `run_budget_project_lookup_deferred` | no | `office_budget_deferred_project_lookup_total{provenance}` |
| Project lookup failed at max retry | `run_budget_project_lookup_failed` | no | `office_budget_failed_project_lookup_total{provenance}` |
| Payload unparseable, cancelled | `run_budget_payload_unparseable` | no | `office_budget_cancelled_unparseable_payload_total{provenance}` |
| Task not found, cancelled | `run_budget_task_not_found` | no | `office_budget_cancelled_task_not_found_total{provenance}` |
| Limit reached, blocked | `run_budget_blocked` (existing) | yes/default | `office_budget_blocked_limit_total{provenance}` |
| Pricing-degraded, blocked | `run_budget_pricing_degraded_blocked` | yes/default | `office_budget_blocked_pricing_degraded_total{provenance}` |
| Pricing-degraded, admitted | `run_budget_pricing_degraded_admitted` | yes/default | `office_budget_admitted_pricing_degraded_total{provenance}`, incremented once per **run** admitted (not per entry), only when the run's final disposition is launch |
| Admitted against built-in default | (no entry — launch is silent by design) | — | `office_budget_admitted_default_total{provenance}` |

Counters live in a new `internal/office/service/budget_metrics_vars.go`,
following the `expvar.NewMap` + label convention of
`internal/office/scheduler/metrics_vars.go` cited in
`stall-visibility.md`. Every deferral counter increments once per deferral
**attempt** (`AC-OFFICE-BUDGET-005.4`/`.6`); every other counter increments
once per run. `AC-OFFICE-BUDGET-002.13`/`.4.8`'s per-policy-per-UTC-day
activity-entry dedup is an in-memory `map[string]civilDay` guarded by a
mutex on `CostService` (best-effort across restarts and concurrent
evaluations, per the criteria's own "best-effort under concurrency" clause —
no new table for a dedup key that is allowed to duplicate).

Every entry that fires before any ceiling was determined states so
explicitly in its JSON `Details` (`"ceiling_determined": false`) and never
sets a `policy_id` field, except the two unevaluated-policy states, which
name the policy id that could not be evaluated (`AC-OFFICE-BUDGET-005.5`).
Every deferral entry's `Details` also carries `"attempt": run.RetryCount + 1`
(`AC-OFFICE-BUDGET-005.6`).

## `runs.outcome` vocabulary change

`RunOutcomeBudgetUnmeasurable = "budget_unmeasurable"` added to
`internal/office/service/run.go`'s outcome block. Companion edit to
[task-delivery-ledger/spec.md](../../task-delivery-ledger/spec.md), same
change: the `runs.outcome` column doc's five-value list becomes six and its
"one of the five values... it never writes ''" sentence becomes six; the
`office/service/scheduler_integration.go:829` mapping-table row is narrowed
to "pre-execution budget block, limit reached" with a new row for the
pricing-degraded-unmeasurable block; the corresponding GIVEN/WHEN/THEN
scenario is narrowed and a new scenario added for the unmeasurable case —
per `AC-OFFICE-BUDGET-005.7`'s explicit requirement that both the
enumeration and every statement fixing its cardinality change together.

## Frontend

`apps/web/app/office/workspace/costs/create-budget-form.tsx`'s period
`Select` gains `daily` and `yearly` options (alongside the existing
`monthly`/`total`) so the offered set matches `BudgetPeriod.Valid()` exactly
— `AC-OFFICE-BUDGET-002.9`. A new Go test,
`internal/office/costs/period_parity_test.go`, implements
`AC-OFFICE-BUDGET-002.12`: it reads `enums.go`'s `BudgetPeriod` `Valid()`
switch cases via `go/ast` (same technique as the provenance completeness
test, smaller in scope — one file, one function) for the declared set, and
reads `create-budget-form.tsx`'s literal `SelectItem value="..."` list under
the period `FormField` via a regex scoped to that file for the offered set
(TypeScript, not Go — `go/ast` cannot parse it), and fails on any
asymmetric difference. This lives in the **Go** suite per
`AC-OFFICE-BUDGET-002.12`'s explicit requirement ("the check shall live in
the backend Go test suite").

`budgets-tab.tsx` gains a `DefaultCeilingCard` (new file,
`default-ceiling-card.tsx`) above the policy list: reads
`GET .../budgets/default`, renders the effective limit in dollars with an
inline edit control that `PUT`s the new value — `AC-OFFICE-BUDGET-003.5`'s
"visible... without inspecting the database". It is visually distinct from a
`BudgetPolicyCard` (no scope/action/period controls — the default is always
workspace-scoped, always `daily`, always blocking) and carries copy stating
it applies to unattended runs only, so an operator does not mistake it for a
policy that also gates attended work.

## Security

No new trust boundary. The two new routes reuse the existing
`officeWorkspaceScopeMiddleware` resolution already applied to
`/workspaces/:wsId/budgets*` (same route group, `registerBudgetRoutes`).

## Related decisions

None new. This design implements the human dispositions already recorded in
the task plan (W2, W3, W4, X2, Y1) rather than introducing its own
architectural choices beyond where noted above (the new table, and the
MaxRetryCount-without-escalation scope reading).
