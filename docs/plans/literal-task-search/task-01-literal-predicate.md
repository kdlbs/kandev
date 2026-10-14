---
id: "01-literal-predicate"
title: "Correct literal workspace task matching"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-SEARCH-001
acceptance_criteria:
  - AC-TASKS-WORKSPACE-SEARCH-001.1
  - AC-TASKS-WORKSPACE-SEARCH-001.2
  - AC-TASKS-WORKSPACE-SEARCH-001.3
  - AC-TASKS-WORKSPACE-SEARCH-001.4
  - AC-TASKS-WORKSPACE-SEARCH-001.5
system_design:
  - ../../specs/tasks/system-design/workspace-search.md
---

# Task 01: Correct literal workspace task matching

## Summary

Make workspace task COUNT and SELECT treat percent signs, underscores, and
backslashes as literal substring text. Independently author real-repository
regressions and dialect coverage while preserving filters, ordering, and paging.

## In scope

- `searchTasks` pattern escaping and all four COUNT/SELECT comparisons.
- The three SQLite behavioral tests and environment-gated PostgreSQL matrix
  named in [the plan](plan.md#tests).
- Synchronize this order, the plan, and paired spec statuses/results after
  passing targeted checks and the owning-package aggregate.

## Out of scope

- ROOT's private candidate source; read only the authorized proof metadata/log.
- Other search paths, normalization, new syntax/settings, migrations,
  dependencies, frontend/E2E changes, layout, localization, and unrelated files.

## Acceptance

1. Independently authored `TestWorkspaceTaskSearchLiteralPagination` fails
   before the correction because distractors inflate the total and hide the
   literal row; after correction COUNT and SELECT agree on literal results.
2. Field/edge-character, deduplication, multi-page, ordinary/empty, and filter
   assertions pass on real SQLite. The PostgreSQL counterpart executes the
   same behavior on an isolated schema when the test DSN is supplied.
3. The owning-package aggregate, changed-Go package lint, and light artifact
   checks pass. Results distinguish actual PostgreSQL execution from a skip.

## Verification

Run from the repository root after the separate implementation request.
The targeted command runs RED before production changes and GREEN after the
correction; use only `TestWorkspaceTaskSearchLiteralPagination` for the first
causal RED if the complete new matrix has not yet been written.

```bash
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 360s go test -race -trimpath -tags=fts5 -p=1 -timeout=5m -count=1 -run '^Test(Postgres)?WorkspaceTaskSearchLiteral|^TestTaskSearchSelectQuery_' -v ./internal/task/repository/sqlite)
(cd apps/backend && env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 960s go test -race -trimpath -tags=fts5 -p=1 -timeout=15m -count=1 ./internal/task/repository/sqlite)
env PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS='-p=1 -tags=fts5 -trimpath' timeout --kill-after=10s 360s bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/task.go apps/backend/internal/task/repository/sqlite/task_search_literal_test.go apps/backend/internal/task/repository/sqlite/task_search_literal_postgres_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/backend/internal/task/repository/sqlite/task.go apps/backend/internal/task/repository/sqlite/task_search_literal_test.go apps/backend/internal/task/repository/sqlite/task_search_literal_postgres_test.go docs/specs/tasks/README.md docs/specs/tasks/requirements/workspace-search.md docs/specs/tasks/system-design/workspace-search.md docs/plans/literal-task-search
git status --short -- docs/plans/literal-task-search
```

The DSN remains inherited, never printed or embedded. With
`KANDEV_TEST_POSTGRES_DSN` configured, the targeted command must run the actual
PostgreSQL test; otherwise report the skip and inspect the actual PostgreSQL CI
job before delivery. Do not provision infrastructure during design.

Use `.github/scripts/pr-docs.cjs` `validateCoverage` on the actual owned changed
paths plus the complete linked artifact content, including untracked files.
The design checkpoint records this light preflight. Repeat with the final actual
changed paths after implementation; do not substitute catalog lint for coverage.
If Go ownership changes, pass every actual changed Go path to
`scripts/lint-go-changed`, which lints complete affected packages. CI owns the
full backend suite. Retain and join every original command handle.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/task_search_literal_test.go`
- `apps/backend/internal/task/repository/sqlite/task_search_literal_postgres_test.go`
- `docs/specs/tasks/README.md`
- `docs/specs/tasks/requirements/workspace-search.md`
- `docs/specs/tasks/system-design/workspace-search.md`
- `docs/plans/literal-task-search/plan.md`
- `docs/plans/literal-task-search/task-01-literal-predicate.md`

## Dependencies

None. Start only after the completed design checkpoint and a later explicit
implementation request in the same primary conversation.

## Risks

SQL escape consistency, count/page drift, and actual PostgreSQL dialect
behavior. Preserve existing repository-filter join semantics and case folding.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workspace-search.md), `.1` through `.5`.
- [System design](../../specs/tasks/system-design/workspace-search.md).
- `SearchMessages` in `message.go`; `task_order_test.go`; disposable SQLite
  setup in `sidebar_task_query_test.go`; isolated PostgreSQL setup in
  `message_prompt_index_postgres_test.go`; backend test guidance.
- Authorized `qualified-proof.json` and `proof.log` only.

## Results

Completed local implementation checks on 2026-10-14 after the separate grant:

- RED: the targeted race/trimpath/fts5/p1 command restricted to
  `^TestWorkspaceTaskSearchLiteralPagination$` failed with 22 rather than one
  for both punctuation queries; exit 1, package 0.389 seconds. Production was
  unchanged until this independently authored failure.
- Targeted GREEN (first verification command): exit 0, package 11.479 seconds.
  All SQLite cases passed, including fixed-date ordering, literal first-page
  recovery, all four arms, counts, pages, deduplication, and filter controls.
- Owning-package aggregate (second verification command): exit 0, package
  700.175 seconds under race detection. The original command was joined.
- Canonical `scripts/lint-go-changed` command: exit 0, zero issues across the
  complete owning package, with all three actual owned Go paths supplied.
- PostgreSQL counterpart: explicitly skipped locally without the test DSN.
  Actual Backend Postgres CI execution is required before PR readiness.
- Final catalog/spec/diff/status checks passed. Actual staged documentation
  coverage passed for all eight owned changed paths and the complete artifact
  chain, with no errors; results are synchronized in the plan.

This order's code and local behavior checks are complete. Hosted PostgreSQL
and PR gates remain delivery checks, and the local skip is not a pass. No
production code or permanent tests were changed during the prior design phase.
