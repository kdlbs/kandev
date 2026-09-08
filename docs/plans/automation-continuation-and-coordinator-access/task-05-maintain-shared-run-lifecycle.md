---
id: "05-maintain-shared-run-lifecycle"
title: "Maintain shared run lifecycle"
status: completed
wave: 3
depends_on: ["02-dispatch-reusable-turns"]
plan: "plan.md"
spec: "../../specs/office/automation-runs.md"
---

# Task 05: Maintain Shared Run Lifecycle

## Acceptance

- Per-automation/workspace queries project each run's exact-turn summary, title, thread outcome, and
  open state; legacy unbound rows remain readable.
- Retention counts distinct tasks and protects the continuation; run/delete-all cleanup is
  reference-aware and never rewrites a reused checkout.
- Automation deletion prevents admission, captures all referenced tasks, stops live sessions, and
  inserts a durable cleanup job for every now-unreferenced hidden task/worktree before deleting its
  references. Jobs survive restart and are removed only after cleanup succeeds.

## TDD scenarios

1. RED: Seed two scheduled turns plus a human turn in one session; assert exact summaries, titles,
   statuses, and legacy fallback.
2. RED: Cover distinct-task retention, continuation protection, and no reset/rebase call on resume.
3. RED: Delete one shared run, delete all runs, and delete an automation with current/superseded/live
   tasks; assert reference ownership, session stop, and worktree cleanup.
4. RED: Fail post-commit task cleanup, restart the service, and prove the persisted cleanup job is
   retried and removed only after task/worktree cleanup succeeds.
5. GREEN: Update projections, retention ownership, automation lifecycle deletion, and retry state.
6. REFACTOR: Centralize hidden-task ownership and cleanup across run, automation, and workspace
   deletion.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/automation ./internal/orchestrator ./internal/task/service`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/store_runs_test.go`
- `apps/backend/internal/automation/store_summaries_test.go`
- `apps/backend/internal/automation/store_workspace_runs_test.go`
- `apps/backend/internal/automation/run_retention.go`
- `apps/backend/internal/automation/run_retention_test.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/service_test.go`
- `apps/backend/internal/automation/cleanup_jobs.go`
- `apps/backend/internal/automation/cleanup_jobs_test.go`
- `apps/backend/internal/automation/handlers.go`
- `apps/backend/internal/automation/handlers_test.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_automation_test.go`
- `apps/backend/internal/task/service/service.go`

## Dependencies

Task 02 supplies exact run bindings, terminal ownership, stop behavior, and continuation updates.

## Inputs

- Persistence, deletion, and retention guarantees in both automation specs.
- Existing task deletion/worktree cleanup and automation retention paths.

## Parallelism

Sequential after Task 02. It overlaps automation store/service files with Task 02 and supplies the
projection consumed by Task 07.

## Output contract

Report projection fixtures, retention IDs, cleanup ownership/order, orphan retry evidence, files
changed, and exact tests.

## Risks

- Deleting references before capturing task IDs can leave hidden worktrees permanently unreachable.

## Results

Implemented exact-turn liveness/reconciliation, exact-run stopping, distinct-task retention, reference-aware deletion, durable cleanup jobs, startup retry, and worktree-preserving reuse. Automation and orchestrator verification passed with 392 and 2,060 tests respectively.
