---
id: "01-backfill-tree-lifecycle-vacancies"
title: "Backfill feeder vacancies after task-tree lifecycle mutations"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.2
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 01: Backfill Feeder Vacancies After Task-Tree Lifecycle Mutations

## Summary

Make `ArchiveTaskTree` and `DeleteTaskTree` wake task-service WIP
reconciliation once per distinct workflow step they vacate. Add a
SQLite-backed regression test for archive and delete cascades. The test proves
that eligible feeder candidates replace destination occupants. It also proves
`wip_pull` transition attribution.

## In scope

- Add the failing archive/delete cascade regression test before production
  changes.
- Add and wire a narrow `VacatedStepReconciler` dependency.
- Capture pre-mutation workflow-step IDs and batch distinct reconciliation
  calls after each mutation loop.
- Preserve the cascade's post-commit, log-and-continue behavior.
- Run targeted RED/GREEN checks and the requested backend test and lint gates.

## Out of scope

- Changing queue ordering, feeder eligibility, WIP counting, or startup
  recovery.
- UI, API, persistence-schema, or documentation behavior changes.
- General cleanup or refactoring outside the touched cascade and wiring.

## Acceptance

- Archiving or deleting a two-task tree from a WIP-limited destination through
  `HandoffService` promotes enough admitted, untagged feeder candidates to fill
  the newly available slots.
- A cascade that vacates multiple tasks in the same workflow step invokes the
  vacancy reconciler once for that step, after the mutation loop, and skipped
  archive rows do not contribute reconciliation work.
- Promoted candidates persist in the destination as admitted work and record
  task-step transitions with trigger `wip_pull`. A reconciliation error does not
  fail an already-committed archive or delete.

## Verification

```bash
cd apps/backend
go test -run '^TestHandoffTaskTreeLifecycleBackfillsFeederVacancies$' ./internal/task/service -count=1

cd ../..
make -C apps/backend test
make -C apps/backend lint
bash -c 'files=$(git diff --cached --name-only --diff-filter=ACMR | grep "\.go$"); [ -z "$files" ] || [ -z "$(gofmt -l $files)" ]'
git diff --check
```

During RED, the focused test must fail on feeder placement or reconciliation
count. It must not fail on compilation or fixture setup. Run the focused command
again after the last production edit. Record the RED and GREEN outcomes in this
file.

## Files likely touched

- `apps/backend/internal/task/service/handoff_cascade_feeder_pull_test.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/backendapp/helpers.go`
- `docs/plans/tree-archive-feeder-pulls/plan.md`
- `docs/plans/tree-archive-feeder-pulls/task-01-backfill-tree-lifecycle-vacancies.md`

## Dependencies

None.

## Risks

- The existing public feeder-source reconciler has opposite step semantics.
  Reusing it for a vacated destination silently reconciles the wrong edges.
- The archive cascade can skip rows that another caller already archived. The
  step set must receive data only after a successful CAS.
- Tree deletion can include already archived descendants. Only active rows
  consume WIP. An extra destination reconciliation is idempotent and safe.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/wip-limit-pull-system.md`
- `docs/specs/tasks/system-design/wip-limit-pull-system.md`
- `docs/decisions/2026-07-28-visible-wip-overflow-queues.md`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/backendapp/helpers.go`

## Results

- Added an explicit `VacatedStepReconciler` dependency from backend
  composition to `HandoffService`.
- Archive and delete cascades capture pre-mutation workflow steps, retain only
  successful mutations, and reconcile each distinct step after the loop with
  a detached context.
- The SQLite-backed regression test failed before production changes because
  both cascade paths stranded feeder work. It now passes for archive and
  delete, including distinct-step batching and `wip_pull` ledger attribution.
- Review remediation moved vacancy capture into the archive/delete transaction
  and added a deferred batch finalizer for committed partial cascades.
- `go test ./internal/task/service -count=1` passed 1,567 tests.
- `make -C apps/backend test` passed with the session's pinned internal config
  path removed so config-discovery tests could use their temporary fixtures.
- `make -C apps/backend lint` passed with zero issues.
