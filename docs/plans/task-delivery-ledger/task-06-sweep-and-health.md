---
id: "06-sweep-and-health"
title: "Sweep, event triggers, writer health and wiring"
status: done
wave: 5
depends_on: ["02-ledger-store", "03-evaluator", "04-observations", "05-ancestry"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 06: Sweep, event triggers, writer health and wiring

Assemble the pieces into a running writer and make its failure visible.

## Service

`service.go` composes observe -> classify -> ancestry -> upsert for one pair, and
drives the whole candidate set for the sweep. Evaluation is idempotent, so a pair
evaluated twice with unchanged inputs is safe and a partially-completed sweep
needs no resume state beyond the deterministic ordering task 04 provides.

A pair whose task still has running sessions is evaluated normally. There is no
"wait until the task is finished" gate: the lattice makes early classification
safe, because an outcome can only be promoted.

## Sweep

`sweep.go` — 5-minute ticker, **no result cap**. It evaluates every candidate
pair whose `last_evaluated_at` is older than its most recent input observation. A
pair is never silently skipped; if the sweep is interrupted, the next one resumes
on the same ordering.

Follow the repository's goroutine ownership rules exactly
(`apps/backend/CLAUDE.md`): single owner, `Start(ctx)` / `Stop()`, registration
on a `sync.WaitGroup`, `Stop` cancels and waits for drain, idempotent on both
ends, restartable (reset stopped state and create a fresh cancellable context
under the same mutex `Stop` uses). Use `time.NewTicker` inside a `select` that
also watches shutdown — never `time.Sleep` in the loop. Add
`goleak.VerifyTestMain(m)` in a package `TestMain`.

## Event triggers

Subscribe to the three events that exist:

- `events.TaskSessionStateChanged` (`internal/events/types.go:62`) — evaluate the
  session's pair when the state is terminal.
- `events.GitHubTaskPRUpdated` (`:291`)
- `events.GitLabTaskMRUpdated` (`:306`)

The spec's fourth trigger, a git snapshot write, has no event type: 
`internal/task/repository/sqlite/git_snapshots.go:101` writes the row and
publishes nothing. Per plan decision D6 it is served by the sweep's freshness
predicate, bounding latency at one 5-minute interval rather than putting a
publish on the snapshot hot path. Azure has no PR-updated event and is covered
the same way. Do not add a new event type without resolving the plan's Open
Question 1 first.

## Writer health

Both signals are required, because they fail differently.

**In-process counters** — `metrics.go`, `expvar` maps published at package init
following `internal/common/subproc/metrics.go`, exposed through the existing
dev-mode `/debug/vars` handler. No new route:

- `delivery_ledger_evaluations_total` (keyed by outcome)
- `delivery_ledger_rows_written_total`
- `delivery_ledger_demotions_suppressed_total` (from task 02's `UpsertResult`)
- `delivery_ledger_ancestry_errors_total` (from task 05)
- `delivery_ledger_sessions_unattributed_total` (from task 04)

**A data-side stall signal** — `MaxLastEvaluatedAt` (task 02) compared against
the most recent `task_sessions` activity, with a 15-minute threshold (three
missed sweeps). This is the signal that survives a restart: counters reset to
zero, so a writer that stopped before the last restart reads perfectly healthy by
counters alone and stale by this one. Shipping only the counters would satisfy
the letter of "writer health" and miss the case the spec actually names.

Because the migration runner swallows errors
(`internal/db/migratelog.go:33`), a failed migration is invisible at boot. With
the table missing, writes fail and the stall detector fires — these health
signals are therefore also the migration-failure signal, and that is the only
detection this card has.

## Wiring

`provider.go` — `Provide(writer, reader *sqlx.DB, bus events.Bus, log *logger.Logger)
(*Service, func() error, error)` following the repository's provider pattern,
registered in `internal/backendapp` alongside the other providers, with cleanup
calling `Stop()`.

- **Acceptance:**
  1. An end-to-end evaluation over a seeded database persists the correct row for
     each of a `pr_merge`, `direct_commit`, `no_delivery_observed` and `unknown`
     pair, writes no row for a task with no repository, and counts a session with
     an empty `repository_id` as unattributed.
  2. The sweep starts, evaluates, and stops with no goroutine leak; `Stop` then
     `Start` restarts cleanly.
  3. All five counters appear in `/debug/vars` under dev mode and move as
     documented.
  4. After a simulated restart (fresh `Service` over the same database, counters
     at zero), the stall signal still reports the true last write instant.

- **Verification:**
  `cd apps/backend && go test -race ./internal/delivery/... ./internal/backendapp/... && make lint`

- **Files likely touched:**
  - `apps/backend/internal/delivery/service.go`
  - `apps/backend/internal/delivery/service_test.go`
  - `apps/backend/internal/delivery/sweep.go`
  - `apps/backend/internal/delivery/sweep_test.go`
  - `apps/backend/internal/delivery/metrics.go`
  - `apps/backend/internal/delivery/provider.go`
  - `apps/backend/internal/delivery/main_test.go` (`goleak.VerifyTestMain`)
  - `apps/backend/internal/backendapp/` (provider registration + cleanup)

- **Dependencies:** Tasks 02, 03, 04, 05.

- **Parallelism:** sequential. It integrates all four preceding ledger tasks and
  touches `internal/backendapp`.

- **Inputs:** Spec **Evaluation triggers and cadence**, **Writer health**,
  **Persistence guarantees**, **Failure modes**, and the **Writer health**
  scenarios; plan decision D6; goroutine ownership and leak-testing rules in
  `apps/backend/CLAUDE.md`; `internal/common/subproc/metrics.go` as the expvar
  precedent.

- **Output contract:** summary, files changed, tests run with counts, blockers,
  risks, and task/plan status update in the same conversation.

## Results

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries when
applicable, or explicitly state `None`.
