---
requirements:
  - REQ-OFFICE-ROUTINE-ARMING-001
  - REQ-OFFICE-ROUTINE-ARMING-002
  - REQ-OFFICE-ROUTINE-ARMING-003
  - REQ-OFFICE-COORDINATOR-INSTALL-001
system_design:
  - ../../specs/office/system-design/routine-schedule-state.md
  - ../../specs/office/system-design/coordinator-install-idempotency.md
created: 2026-09-17
status: done
---

# Implementation Plan: Office Routine Arming Visibility and Coordinator Install Idempotency

## Overview

An Office routine has two independent switches — intent (`office_routines.status`)
and schedule state (whether its triggers can actually fire it) — and the product
today reports only intent. This plan makes schedule state visible everywhere
intent already is (routine reads, an unattended-install startup scan) and closes
the one identity defect that lets onboarding duplicate the pre-installed
coordinator routine. It repairs nothing: no criterion in any of the three
requirement documents enables, disables, creates, or deletes a trigger, and none
makes intent gate dispatch. That repair is deferred to task
`b0382916-da13-44a5-85be-064ae6a9533c` and must not be pulled forward here.

Two system-design documents back this plan:
[Routine Schedule State](../../specs/office/system-design/routine-schedule-state.md)
covers REQ-OFFICE-ROUTINE-ARMING-001/002/003 (classification, the read-path
report, and the startup scan — one vertical feature contract split across two
requirement documents by scope), and
[Coordinator Install Idempotency](../../specs/office/system-design/coordinator-install-idempotency.md)
covers REQ-OFFICE-COORDINATOR-INSTALL-001. Both were written after Build,
grounded directly in the merged implementation, rather than before it — the
three requirement documents already pinned every table, column, function, and
endpoint this plan touches at file-and-line precision, so drafting the designs
speculatively ahead of code would have either duplicated that text or drifted
from it. This plan itself still carries the remaining architecture decisions —
package placement, response shape, verification boundaries — that the
requirements deliberately left to Build.

Work lands in three waves: classification is the foundation everything else
reads (Wave 1); the read-path report, the startup scan, and the coordinator
install fix are three independent consumers of it and can proceed in parallel
(Wave 2); the frontend surface depends on the backend response shape Wave 2
defines (Wave 3).

## Code facts confirmed on this branch (2026-09-17)

All paths relative to `apps/backend`.

- **Schema.** `office_routines` and `office_routine_triggers` are defined in
  `internal/office/repository/sqlite/base.go`, `createRoutineTables`
  (L414-467) — one dialect-agnostic file for both SQLite and PostgreSQL via
  `internal/db/dialect`. There is no separate PostgreSQL schema file.
- **Trigger reads today.** `internal/office/repository/sqlite/routines.go`:
  `GetDueTriggers` (L65-78) filters `kind='cron' AND enabled=1 AND
  next_run_at IS NOT NULL AND next_run_at <= ?` with no `ORDER BY` and never
  reads `office_routines.status` — confirms the requirement's central claim
  that dispatch does not gate on intent. `ClaimTrigger` (L84-98) and
  `UpdateTriggerNextRun` (L99-105) are the only trigger-state writers.
- **Coordinator install today.** `internal/office/routines/service.go`:
  `CreateDefaultCoordinatorRoutine` (L145-196) calls `findCoordinatorRoutine`
  (L197-217), which matches workspace + assignee + canonical name +
  `hasCronTrigger` (L222-239, ignores `enabled`, matches on kind + exact
  expression only). Its two production callers —
  `internal/office/agents/service.go:572` and
  `internal/office/onboarding/service.go:767` — both discard the returned
  routine and only warn-log the error (`if _, err := ...`), which is why
  AC-OFFICE-COORDINATOR-INSTALL-001.10 forbids reporting through a return
  value.
- **Cron evaluation.** `internal/office/shared/cron.go`: `NextCronTime`
  (L13-53) validates the timezone independently of the expression and
  requires exactly 5 fields; `findNextMatch` (L221-235) returns no error, so
  an impossible-but-parseable expression yields a time rather than failing.
- **Startup sequencing.** `internal/office/infra/reconcile.go`:
  `Reconciler.ReconcileAll` (L34-54) is synchronous with a void return;
  `createTriggersForNewRoutines` (L113-132) gives every trigger-less routine
  a `manual` trigger. Called from `internal/backendapp/main.go:1378-1380`,
  immediately followed by `log.Info("Office reconciliation complete")` and,
  a few lines later (~L1393), `services.Office.RegisterEventSubscribers`.
  That gap is the wiring point for the startup-scan completion signal
  required by AC-OFFICE-ROUTINE-ARMING-003.1 — nothing today observes
  `ReconcileAll` returning.
- **The actual routine list/detail response.**
  `internal/office/routines/handler.go`'s `listRoutines` (L45-52) and
  `getRoutine` (L98-105) back `GET /workspaces/:wsId/routines` and
  `GET /routines/:id`, returning `RoutineListResponse`/`RoutineResponse`
  (`internal/office/routines/dto.go:34-41`) — today just `*Routine`
  (intent only). This, not the dashboard, is Task 02's target.
  Separately, `internal/office/dashboard/service_agents.go`'s
  `ListRoutines` block inside `runSoftQueries` (~L551-561) only counts
  `status == "active"` into the workspace KPI tile's `RoutineCount` and is
  not in scope.
- **Trigger HTTP routes**, `internal/office/routines/handler.go`,
  `RegisterRoutes` (L30-42): `GET`/`POST /routines/:id/triggers`,
  `DELETE /routine-triggers/:triggerId`,
  `POST /routine-triggers/:publicId/fire`. No route edits, enables, or
  disables a trigger — confirms `## Out of scope` → *Disabling a trigger from
  the UI* in the visibility document.
- **Metrics prior art.** `internal/orchestrator/office_stall_metrics.go`
  (`expvar.NewMap` + label builder, L20-46) and
  `internal/workflow/signalmetrics/` are the two labelled-counter
  conventions already in the codebase; this plan's new counters follow the
  same shape rather than inventing a third.
- **Persistence conformance.** `internal/persistence/requiredstores` already
  registers office's schema owner (`catalog.go:52`) and
  `internal/persistence/storeconformance` already has a fixed office
  adapter (`adapters.go:224-229`). No new table is required by any
  criterion in these three documents; Task 05's serialization mechanism must
  be chosen so that stays true (see its risks).

## Waves

### Wave 1 — classify

- [x] [Task 01: Classify a routine's schedule state](task-01-classify-schedule-state.md)

### Wave 2 — three independent consumers of classification

- [x] [Task 02: Report intent and schedule state on routine reads (API)](task-02-report-schedule-state-api.md)
- [x] [Task 04: Surface unarmed routines at startup](task-04-startup-scan.md)
- [x] [Task 05: Match the coordinator routine on identity, not mutable trigger state](task-05-coordinator-install-idempotency.md)

### Wave 3 — frontend

- [x] [Task 03: Render the intent/schedule-state distinction in the routines UI](task-03-report-schedule-state-ui.md)

Tasks 02, 04, and 05 touch disjoint files and have independent verification
boundaries; once Task 01 lands they may proceed in parallel. Task 03 depends
only on Task 02's response shape, not on Task 04 or Task 05.

## Required workflow verification

After all task-level checks pass:

1. `cd apps/backend && gofmt -l $(git diff --name-only main -- '*.go')` — must
   be empty.
2. `make -C apps/backend test` (full suite; the office and persistence
   packages are the ones this plan can regress).
3. `make -C apps/backend lint`.
4. If Task 05 adds a schema owner (see its risks):
   `go run ./cmd/sqlguard ./internal` and
   `go test -race ./internal/persistence/storeconformance -count=1` from
   `apps/backend`.
5. `cd apps/web && pnpm run typecheck && pnpm run lint`.
6. `cd apps/web && pnpm run i18n:check`.
7. Playwright coverage per Task 03 (`cd apps/web && pnpm e2e:run`, scoped to
   the affected spec — see that task).
8. Commit the explicit changed paths with a Conventional Commit message.

## Risks and non-goals

- **Do not pull the deferred repair forward.** Both the operator-initiated
  re-arm action and making intent gate dispatch are explicitly out of scope
  of every document in this plan and belong to task
  `b0382916-da13-44a5-85be-064ae6a9533c`. A work order that starts writing
  `enabled` or `next_run_at` outside Task 05's own coordinator-install
  repair path (AC-OFFICE-COORDINATOR-INSTALL-001.4/.12, which only ever
  *creates* the coordinator's own canonical trigger) has drifted from scope.
- **The runaway-fire defect is a known, accepted artifact, not a bug to
  fix.** When `computeRoutineMissed` errors, `processCronTrigger`
  (`routines/service.go:376-387`) re-arms to `now` and fires again every
  tick. Under REQ-OFFICE-ROUTINE-ARMING-001 such a trigger classifies
  `armed` (rule 1) forever, correctly per the requirement's own
  `## Out of scope` → *Cron evaluation defects*. Do not special-case it in
  Task 01's classifier, and write the fixture that way rather than assuming
  decay to `trigger_invalid`.
- **Dispatch grace is one constant, not two.** AC-OFFICE-ROUTINE-ARMING-001.10
  requires classification (Task 01) and the startup scan (Task 04) to share
  the exact same 60-second compile-time constant. Define it once in Task 01
  and import it in Task 04; do not redeclare it.
- **Task 05's serialization mechanism is Build's choice, but three
  candidates are pre-ruled-out in the requirement's own `## Out of scope`:**
  a bare default-isolation transaction (both callers can still both read
  "absent"), a PostgreSQL advisory lock (no SQLite equivalent), and an
  in-process-only guard (fails the documented cross-process domain). See
  Task 05 for the constraint set the chosen mechanism must satisfy.
- **No coverage percentage is a project gate.** New tests must assert the
  specific acceptance criteria named in each task, not a line-coverage
  target.
- **Translation completeness.** Task 03's copy must exist in all five
  shipped locales (`pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, plus source English)
  before `pnpm run i18n:check` passes; use `pnpm run i18n:zh-hant` for the
  Traditional Chinese pair rather than hand-translating.
