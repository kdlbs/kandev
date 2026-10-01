---
created: 2026-10-01
status: in_progress
requirements:
  - REQ-TASKS-WORKFLOW-STEP-ORDERING-001
  - REQ-TASKS-COMPLETION-001
system_design:
  - ../../specs/tasks/system-design/workflow-step-ordering.md
legacy_specs: []
---

# Implementation Plan: Workflow Step Reorder Atomicity

## Overview

Replace stale whole-row reorder writes with one positional transaction. One
sequential work order reproduces the defect, implements the repair, and checks
SQLite, PostgreSQL behavior, and existing guards before delivery.

## Confirmed root cause and evidence

Refreshed main and the branch both resolve to
`08e4ffdb99caf40b0df5baa67b29cf4313188f15`. `Service.ReorderSteps` reads each
step through `GetStep`, changes its position, and calls `UpdateStep`, which
overwrites content from that snapshot and commits each row separately.

Temporary tests recreated on 2026-10-01 failed with the expected assertions:
`TestInvestigationReorderConcurrentEdit` expected `edited concurrently` but
read `original`; `TestInvestigationReorderRollback` expected the first written
step to retain position 1 but read 0 after the second write failed. Both ran in
`go test ./internal/workflow/service -run '^TestInvestigationReorder' -count=1`
(0.056s package execution). The investigation file was removed; permanent
regressions are the implementation work order's first action.

## Scope

### In scope

- Ordinary workflow-step reorder persistence and exact membership validation.
- Saved prompt/profile/completion preservation and rollback of positions/timestamps.
- Permanent deterministic regressions and existing authorization/controller guards.
- Backend-only documentation and authorized PR delivery through normal merge.

### Out of scope

- Workflow content-save redesign, task ordering, schema changes, frontend changes.
- Other parallel repairs, broad suite runs, new tasks/sessions, or implementation workers.

## Technical approach

Add `Repository.ReorderSteps` in the workflow repository, using a transaction
for membership validation and position/timestamp-only writes. Replace the
ordinary service loop with authorized delegation and preserve typed visibility
errors. Keep controller guards and `ReorderStepsIfUnchanged` intact. Prefer a
small repository file over extending the large `sqlite.go` unnecessarily.

| Database | Persistence path | Evidence | Conditional coverage |
| --- | --- | --- | --- |
| SQLite | Writer transaction, transaction-scoped membership, positional UPDATE | File-backed two-handle edit barrier and second-write trigger | Always run |
| PostgreSQL | Rebound SQL and existing isolation/locking patterns | Real isolated-schema success, rejection, failure rollback, concurrency | `KANDEV_TEST_POSTGRES_DSN`; record actual execution or skip |

## Tests

- `service/reorder_test.go`: `TestReorderStepsPreservesConcurrentEdit` and
  `TestReorderStepsRollsBackSecondWriteFailure` cover ORDERING criteria 2/3 and
  `AC-TASKS-COMPLETION-001.4`.
- `repository/reorder_test.go`: `TestReorderStepsValidatesCompleteMembership`
  covers ORDERING criteria 1/4, including both legitimate and foreign rows.
- `repository/reorder_postgres_test.go`: real dialect success, rollback, and
  multi-connection concurrency variants of those tests.
- Existing `service/access_test.go`, controller session-target tests, and
  version-fencing repository tests cover ORDERING criterion 5 and compatibility.

Persistence is verified at the real service/repository boundary. The UI and
request shape do not change, so no artificial browser suite is added.

## Work orders

- [ ] [Task 01: Atomic positional reorder](task-01-atomic-positional-reorder.md)

## Documentation impact

The tasks system owns workflow definitions. Completion-setting preservation
already exists in `AC-TASKS-COMPLETION-001.4`; complete-order atomicity is now
recorded in the owning workflow ordering requirement and design. No new ADR
is required for applying the existing local transaction pattern.

`/docs-maintainer` checked `docs/public/workflow-tips.md`, task/workflow docs,
import/export docs, README, and screenshot catalog. Add one short explanation
of reorder preservation/rollback to the existing workflow how-to during
implementation, without implying that all settings Save operations are atomic.

## Verification results

Design validation passed: `python3 scripts/list-docs.py validate` (339 decisions,
1283 specifications), `python3 scripts/lint-spec-files.test.py` (36 tests),
`python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
The local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight, using the
planned service path and actual package content, returned `covered` with no
errors. Node is available through `mise exec -- node` in this environment.
The parent coordinator reviewed the package and requested implementation in a
later turn. Permanent RED regressions reproduced both original defects. The
minimal positional transaction is implemented; SQLite race tests, four real
PostgreSQL test families (including membership subcases and concurrency),
SQL guard, controller/HTTP guard tests, and public-document validation pass.
Persistence store-conformance passed with SQLite and disposable PostgreSQL.
Delivery remains pending;
see the work order for exact local results.

## Risks

- Query rows and single-connection pool ownership can deadlock if not closed.
- Membership errors must preserve foreign-step privacy.
- PostgreSQL behavior requires real conditional coverage, not schema-only checks.
- Concurrent main updates require rebasing and rerunning relevant checks before merge.
