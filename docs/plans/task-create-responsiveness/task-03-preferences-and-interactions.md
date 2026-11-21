---
id: "03-preferences-and-interactions"
title: "Avoid unchanged preferences and verify interactions"
status: in_progress
wave: 3
depends_on:
  - "02-prepare-session-once"
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-RESPONSIVENESS-001
acceptance_criteria:
  - AC-TASKS-CREATE-RESPONSIVENESS-001.1
  - AC-TASKS-CREATE-RESPONSIVENESS-001.2
  - AC-TASKS-CREATE-RESPONSIVENESS-001.5
  - AC-TASKS-CREATE-RESPONSIVENESS-001.6
system_design:
  - ../../specs/tasks/system-design/task-create-responsiveness.md
---

# Task 03: Avoid unchanged preferences and verify interactions

## Summary

Skip identical last-used updates without losing concurrent colors.
Prove the complete creation outcome on desktop and phone, then compare performance.

## In scope

- Add conditional targeted last-used SQL for SQLite and PostgreSQL.
- Return changed/no-change internally and publish only changed settings.
- Preserve missing-user errors, omitted fields, workspace-map entries, and CAS behavior.
- Add desktop and phone creation-plus-color regression tests.
- Compare original and candidate full-path benchmarks from Task 01.
- Record all measured acceptance bounds and test results in the plan.

## Out of scope

Asynchronous preference workers, color retry changes, new API fields, new UI
controls, layout changes, and changes to creation success or recovery boundaries.

## Acceptance

1. TDD proves identical choices preserve revision/timestamp and publish nothing.
   Changed choices publish once and preserve concurrent colors and other workspaces.
2. Desktop and phone create while another task is active, then save its color
   and retain it after reload. Existing uncertain-outcome recovery tests pass.
3. The complete benchmark meets every bound in acceptance criteria .1 and .2.
   A missed or unmeasured bound keeps this work order pending or in progress.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/user/store ./internal/user/service -run 'TestTaskCreateLastUsed|Test.*TaskCreateLastUsed|Test.*Sidebar.*Color' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run 'TestHTTPCreateTask|TestWSCreateTask|TestTaskCreateContentionFixture' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run '^$' -bench '^BenchmarkTaskCreateContention$' -benchtime=100x -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-create-contention.spec.ts tests/task/task-create-recovery.spec.ts tests/task/sidebar-task-color-sync.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-contention.spec.ts tests/task/mobile-task-create-recovery.spec.ts tests/task/mobile-sidebar-task-color-sync.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the first command with an isolated `KANDEV_TEST_POSTGRES_DSN` as well.
Record any skipped dialect test. Run E2E projects sequentially through the
managed runner so each uses rebuilt assets and its guarded worker budget.
For a fresh worktree, first run `pnpm install --frozen-lockfile` from `apps/`.
Compare the candidate benchmark against the retained original binary within
the same validation session. Record both complete result sets.

## Files likely touched

- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/store.go`
- `apps/backend/internal/user/service/service.go`
- Focused user-store and user-service tests for no-op and concurrent writes.
- `apps/web/e2e/tests/task/task-create-contention.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-create-contention.spec.ts` (new)
- The Task 01 benchmark fixtures and this package's Results sections.

No rendered component changes require a preview.
Use existing desktop sidebar and phone drawer interactions from the color tests.

## Dependencies

Tasks 01 and 02 provide the baseline and session optimization.

## Risks

No-op predicates must compare absent and null values consistently in both dialects.
An equal patch must not suppress a concurrent newer choice.
Browser timing is not a stable benchmark, so browser tests use causal completion.
Loaded-host measurements can miss the relative target despite fewer commits.

## Parallelism

`sequential`

## Inputs

The Last-used choices, Browser behavior, and Measurement boundary design sections.
Existing workflow-memory, sidebar-color, and creation-recovery tests.

## Results

Partially completed. The last-used and browser changes are implemented and
verified; the corrected full-path performance acceptance remains open.

`UpdateTaskCreateLastUsed` returns stored settings plus a changed result and
runs a null-safe conditional update in both SQLite and PostgreSQL. An equal
patch leaves settings, revision, and timestamp untouched; a changed patch
preserves omitted fields, other workspaces, and concurrent colors. The user
service publishes only committed changes.

Focused store tests cover no-op revision/timestamp preservation, changed
choice publication, missing users, omitted fields, workspace-map preservation,
and concurrent color survival. Matching Postgres tests skip without
`KANDEV_TEST_POSTGRES_DSN`. Existing handler and user-service suites pass with
`-race`.

The desktop and phone regressions now start a real streaming mock-agent session
on an existing task, create another task, change the streaming task's color
through the desktop menu or phone drawer context menu, verify the color
readback, and verify the streaming session remains RUNNING before and after
reload. Both passed through the managed Playwright runner.

The corrected benchmark in `plan.md` is the authoritative performance result.
It measures real orchestration, a real local repository, and production durable
delivery, and it correctly rejects backlog, failed colors, and inadequate
samples. On this shared host its eight-stream p95 comparison showed only a
10.7% completed-run median improvement, below the required 25%, and one run per
binary failed the 90% delivery bound. AC .1 and AC .2 remain unproven. This
work order therefore stays in progress until the same corrected workload on a
stable host satisfies every latency and delivery bound.