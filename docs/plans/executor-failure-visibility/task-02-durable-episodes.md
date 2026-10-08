---
id: "02-durable-episodes"
title: "Persist and reconcile executor failure episodes"
status: completed
wave: 2
depends_on:
  - "01-observations"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-FAILURE-VISIBILITY-001
acceptance_criteria:
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.2
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.3
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.4
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.5
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.6
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.7
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.8
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.11
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.12
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.13
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.14
system_design:
  - ../../specs/executors/system-design/executor-failure-visibility.md
---

# Task 02: Persist and reconcile executor failure episodes

## Summary

Connect normalized observations to durable scoped episodes, readback, guarded
session settlement and authorized read-only recheck. Prove crash/restart and duplicate
behavior using the real repository and lifecycle/service boundaries.

## In scope

Schema/migrations, bounded inventory inspection, stream grace and startup coordination,
episode/session linkage, additive status summary/DTO events, guarded recheck and
existing Resume admission. Persist history and resolution without erasing independent errors.

Preserve primary failure and cleanup timeout/401 as distinct bounded causes without
changing sentinel matching or bypassing cleanup. Repeated crash-loop observations
must update one episode. Carry provider load-versus-fresh-create evidence into a
durable correlated recovery result, including successful Resume with empty error;
never infer restored conversation from session state. Add the regression names in
the plan and include stale attempt, migration/legacy unknown and reload cases.

## Out of scope

No new automatic retries, resource mutations, capacity settings, worker upgrade logic,
UI rendering or production validation.

## Acceptance

- An idle/shared environment and restart inspection produce one durable episode; every actually affected current session settles idempotently with preserved history and no completion, workflow retry or queue consumption.
- Transport uncertainty preserves possibly live capacity, recovered transport creates no persistent failure, and stale/foreign observation or action cannot affect a successor.
- Boot/read/WS/rebuild carry matching episode revisions, independent errors coexist, successful recovery retires only matching controls, and recheck is demonstrably read-only.

## Verification

Commands run from repository root; new test files named here are implementation
outputs, not existing checks. Demonstrate each regression failing before its fix.
Run the affected baseline tests as well if implementation touches additional suites.

```bash
(cd apps/backend && go test ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service ./internal/task/statussummary ./internal/task/handlers ./internal/orchestrator ./internal/agent/runtime/... -run 'Test(ExecutorFailure|PostgresExecutorFailure|TaskShared|TaskError|PollOneRemoteStatus|StreamDisconnect|ReconcileActive|Interrupted)' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/task/service ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'TestExecutorFailure' -count=1)
```

PostgreSQL evidence is mandatory for episode uniqueness and concurrent settlement.
Use an isolated disposable database with `KANDEV_TEST_POSTGRES_DSN` already set by
the test environment (never a production DSN), then run:

```bash
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run '^TestPostgresExecutorFailure' -count=1 -v)
```

Require named tests to run with zero skips. An unavailable isolated database is a
reported verification blocker, not a passing skipped run.

## Files likely touched

- `apps/backend/internal/task/models/executor_failure.go (new)`
- `apps/backend/internal/task/repository/sqlite/executor_failure_episodes.go (new)`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/service/executor_failure_reconciliation.go (new)`
- `apps/backend/internal/task/statussummary/{model,projector,rebuild}.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/environment_handlers.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_remote_status.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` (propagate existing fallback outcome)
- `apps/backend/internal/orchestrator/session_recovery_feedback.go`
- `apps/backend/internal/orchestrator/task_operations.go` (cleanup cause propagation)
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/executor_failure.go (new)`
- `apps/backend/internal/backendapp/ (lifecycle wiring)`
- `Adjacent *_test.go files named by plan.md`

## Dependencies

01-observations. Refresh open admission/compatibility changes before coding.

## Risks

Identity and prompt-generation races must fail closed. Inspect/recheck cannot mutate
resources or cause prompt replay. Tests must use owned disposable data and cluster;
if container prerequisites are absent, record the exact blocker and leave completion
pending rather than substituting production. Reuse existing database test harnesses
for supported dialects and ensure a regex command actually discovers the new tests.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirements](../../specs/executors/requirements/executor-failure-visibility.md)
- [System design](../../specs/executors/system-design/executor-failure-visibility.md)
- [Plan, fixture matrix, source baseline and coordination](plan.md)
- Scoped AGENTS.md, /tdd, and /mobile-parity plus /e2e for rendered changes.

## Results

Implemented durable fenced episodes, affected-session references and deterministic
history, idle/shared/startup reconciliation, exact interruption settlement without
completion, additive status-summary hydration, read-only recheck, and actual
provider conversation recovery outcomes with recorded workspace-check timestamps.

Validation passed after the final source change:

- The task-defined affected backend regression command, expanded to include
  backendapp, polling and Kubernetes checks, with `GOMAXPROCS=2`, `-p 1`, and an
  owned disposable `KANDEV_TEST_POSTGRES_DSN`.
- Race regressions across repository, service, statussummary, orchestrator and
  lifecycle, including PostgreSQL episode admission and settlement.
- Named `TestPostgresExecutorFailureConcurrentAdmission` and
  `TestPostgresExecutorFailureConcurrentSettlement`: both pass, zero skips.
- Full service, statussummary, handlers, orchestrator and backendapp suites in
  the broader affected-package run.
- Go lint: zero issues; public/spec validation and whitespace checks pass.

RED/GREEN regressions cover durable reload, semantic summary CAS after private
fences are omitted from JSON, stale ownership, history publication/deduplication,
retained workspace versus fresh conversation, bounded secondary evidence and
resource disappearance preserving the earlier actionable cause. The initial
exhaustive repository run reached its 10-minute timeout; the isolated 20-minute
rerun passes: 1,775 cases, no failed assertions (1,168.022s). This supersedes the
earlier timeout. `TestConversationLargeLegacyUpgrade` remains skipped by its
existing optional-test gate; the two mandatory PostgreSQL tests ran with zero skips.

### Worker connectivity regression results

The responsive-API worker regression verifies durable uncertainty, deduplication,
secondary cleanup evidence, reopened-database persistence and healthy resolution.
Generic unknown inspection still creates no new failure episode. Reconciliation
coverage verifies no session settlement, execution retirement or completion for
explicit worker uncertainty. SQLite and service focused regressions pass with the
race detector; affected backend suites and Go static analysis pass.
