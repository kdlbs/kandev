---
id: "01-persist-continuation-policy"
title: "Persist continuation and run contracts"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/automation-continuity.md"
---

# Task 01: Persist Continuation and Run Contracts

## Acceptance

- Automation create, update, get, and list carry validated `continuation_policy`; existing rows
  default to `new_task`, and `reuse_thread` requires concurrency 1.
- Runs persist exact session/turn, thread outcome, and `display_title`; the existing `triggered`
  status represents admitted-but-unbound work and counts as open with `task_created`.
- YAML exports `continuation_policy` but excludes continuation pointers, run bindings, run titles,
  and the fixed MCP profile.

## TDD scenarios

1. RED: Add SQLite/Postgres migration and round-trip tests for policy, pointer, bindings, thread
   fields, and title snapshots.
2. RED: Add service/store tests for invalid policy/concurrency and the shared open predicate over
   `triggered` plus `task_created`.
3. RED: Extend export disposition, deterministic order, runtime exclusion, and literal fixtures.
4. GREEN: Add typed fields, migrations, request validation, run scans, and export DTO changes.
5. REFACTOR: Keep policy parsing and open-status ownership in one automation package location.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/automation`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/automation/models.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/store_test.go`
- `apps/backend/internal/automation/store_runs_test.go`
- `apps/backend/internal/automation/store_summaries_test.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/service_test.go`
- `apps/backend/internal/automation/export.go`
- `apps/backend/internal/automation/export_service.go`
- `apps/backend/internal/automation/export_ac22_test.go`
- `apps/backend/internal/automation/export_document_test.go`

## Dependencies

None.

## Inputs

- Data model, API, and state-machine sections in the automation settings spec.
- Export requirements in `docs/specs/automations-yaml-export/spec.md`.

## Parallelism

Parallel-safe with Task 03 only: this task owns automation persistence/export files; Task 03 owns
agent lifecycle history files.

## Output contract

Report migrations, invalid cases, open-status results, export bytes, files changed, and exact tests.

## Risks

- Treating only `task_created` as open can admit a second run while the first is still unbound.

## Results

Implemented continuation policy defaults/validation, admitted `triggered` runs, exact task/session/turn bindings, title snapshots, thread actions/reasons, reference-aware deletion, and export exclusions. Verified with the automation package suite: 389 tests passed.
