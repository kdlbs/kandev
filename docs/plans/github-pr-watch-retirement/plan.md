---
created: 2026-10-26
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001
system_design:
  - ../../specs/integrations/system-design/github-pr-watch-retirement.md
legacy_specs: []
---

# Implementation plan: Persistent GitHub PR watch retirement

## Overview

Resolve [issue #4666](https://github.com/kdlbs/kandev/issues/4666) through an explicit watch-only retirement operation.
First add persistence without changing public deletion. Then connect service admission, API resumption, result fencing, and documentation.
Both work orders run sequentially. The primary session owns implementation, integration judgment, and status updates.

## Scope

### In scope

- Persistent retirement through the existing watch DELETE operation.
- Authorized retired-watch inventory and explicit resumption.
- Preservation of task/session history, associations, CI state, and automation options.
- Restart, archive/unarchive, concurrency, and shared-target regression coverage.

### Out of scope

- Automatic retirement of all Done tasks, terminal PRs, or tasks without PRs.
- Workflow automation actions, rendered UI, physical request metrics, and issue #4452.
- Live-instance mutation, resource deletion, deployment, commits, and PR creation during planning.

## Evidence and root cause

Inspected source: `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa` on 2026-10-26.
The issue and both comments contain no image attachments.

`DeletePRWatch` removes the row and discovery consumer without retaining intent.
`ListSessionsWithBranches` retains unarchived tasks with active environment branches, regardless of Done state.
`buildTaskBranchList` emits their missing targets. `reconcileWatches` calls `EnsurePRWatchForWorkspace`, which creates a searching row.
`Poller.applyPRStatus` also resets terminal PR watches to searching after the terminal automation hold clears.
That reset serves follow-up PR discovery and is not itself proof of a defect.

A temporary `TestRepro4666_DeleteThenReconcile` used `setupPollerTest`, real SQLite storage, and the actual service deletion and poller reconciliation.
It seeded an unarchived task and a numbered watch on branch `finished`.
A branch-provider fixture supplied the retained target after deletion.
The assertion expected zero watches and failed with:

```text
deleted watch recreated: count=1 pr_number=0 branch=finished
```

Command: `(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestRepro4666_DeleteThenReconcile$' -count=1)`.
The temporary test was removed after execution. No production or permanent test files changed.
The fixture does not model a full Done transition. The real session query establishes why Done does not exclude the target.
No clean-install browser reproduction or request-volume attribution is claimed.

## Requirement reconciliation

The existing identity contract prevents duplicate watches, but does not preserve explicit deletion intent.
This package adds `REQ-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001`, ACs .1-.8.
The discovery requirements and design gain an explicit retirement exception to indefinite eligible discovery.
Completed discovery-health, watch-reconciliation, and watch-task-cleanup plans retain their historical results and scopes.
No old work order is reopened. This package owns the new retirement tests.

## Technical approach

Follow the [retirement design](../../specs/integrations/system-design/github-pr-watch-retirement.md).
Add nullable `retired_at` through `models.go`, the additive store migration, and all watch-table rebuilds.
Retain unique identities for retired rows. Separate public retirement from hard cleanup.
Add `store_pr_watch_retirement.go` and `service_pr_watch_retirement.go` to keep existing large files bounded.

Service create/ensure, poller, and sync admission must agree after task-owner redirection.
Audit `event_handlers_github.go` for the typed retirement skip and nil-watch handling.
Guard both batched and single-watch result application. Resume replaces the watch ID transactionally.
API controls reuse stored workspace authorization and require no rendering changes.

Compatibility coverage includes REST and GraphQL, PAT and App resolution, searching and numbered watches, and SQLite and PostgreSQL.
Unknown or incomplete retired identities remain blocked. Active unrelated targets retain current behavior.

## Tests

- ACs .1-.2: `TestPRWatchRetirement_DeleteThenReconcile` in `store_pr_watch_retirement_test.go` replaces the temporary reproduction.
- ACs .2, .8: `TestPRWatchRetirement_RestartAndArchive` in the same file proves persistence and lifecycle cleanup.
- ACs .3-.4: `TestPRWatchRetirement_PreservesHistoryAndMixedTargets` proves task state and unrelated watches remain intact.
- ACs .5-.6: `TestPRWatchRetirement_HTTPResumeAndScope` and `TestPRWatchRetirement_WSDelete` in `store_pr_watch_retirement_test.go`.
- AC .7: `TestPRWatchRetirement_InFlightResults` covers REST, GraphQL, delete, and retire/resume races.
- Persistence: `TestPRWatchRetirement_MigrationReplay` and `TestPRWatchRetirement_PostgresConcurrency` cover real store behavior.
- Effective ownership: `TestPRWatchRetirement_EffectiveOwner` in `service_pr_watch_workspace_group_redirect_test.go`.

## End-to-end evidence

A backend integration test, `TestPRWatchRetirement_TaskLifecycleIntegration`, exercises HTTP DELETE, three reconciliations, canned provider attempts, service reconstruction, inclusive GET, and HTTP resume.
It uses real task, session, repository, and environment fixtures for a retained Completed task and an active Review sibling. It asserts that the retired branch receives no provider request, the active sibling continues discovery across reconstruction, and explicit resumption creates a new lifetime and admits the Completed target.
`TestPRWatchRetirement_PreservesHistoryAndMixedTargets` separately verifies TaskPR history, CI state, automation options, active shared members, and unrelated targets. No browser test is needed because rendering does not change.

## Work orders

- [x] [Task 01: Persist canonical watch retirement](task-01-persist-retirement.md)
- [x] [Task 02: Enforce retirement and expose resumption](task-02-enforce-retirement.md)

## Verification results

Diagnosis: the temporary regression failed for the expected recreation, then was removed.
Design validation passed on 2026-10-26:

- `python3 scripts/list-docs.py validate`: 404 decisions and 1742 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs:validateCoverage`: both work orders passed the planned implementation artifact preflight.
- `git diff --check`: passed.
- `git status --short`: only the package, proposed decision, and owning specification files changed.

The coverage preflight supplied the planned service path to exercise traceability checks.
It does not claim that production implementation exists.
Task 01 implementation checks passed on 2026-10-26:

- `KANDEV_TEST_POSTGRES_DSN=<isolated local PostgreSQL DSN> go test -trimpath -tags fts5 ./internal/github -run 'TestPRWatchRetirement_|TestPRWatchMigration_|TestPostgresStoreSchemaReplay' -count=1`.
- `KANDEV_TEST_POSTGRES_DSN=<isolated local PostgreSQL DSN> go test -trimpath -race -tags fts5 ./internal/github -run '^TestPRWatchRetirement_PostgresConcurrency$' -count=1`.
- `go run ./cmd/sqlguard ./internal`.
- `KANDEV_TEST_POSTGRES_DSN=<isolated local PostgreSQL DSN> go test -trimpath -race ./internal/persistence/storeconformance -count=1`.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `git diff --check`, and `bash scripts/lint-go-changed apps/backend/internal/github/models.go apps/backend/internal/github/store.go apps/backend/internal/github/store_pr_watch_migration_test.go apps/backend/internal/github/store_watch_reset_test.go apps/backend/internal/github/store_pr_watch_retirement.go apps/backend/internal/github/store_pr_watch_retirement_postgres_test.go apps/backend/internal/github/store_pr_watch_retirement_test.go`.

Task 02 implementation checks passed on 2026-10-26. See [Task 02 results](task-02-enforce-retirement.md#results) for the exact commands and per-check outcomes.

- `make -C apps/backend build`.
- Full GitHub package tests, targeted orchestrator reconciliation tests, and race tests for all PR-watch-retirement cases passed.
- Public-doc tests and validation, specification validation and lint, changed-Go lint, and `git diff --check` passed.
- The final PR-documentation coverage preflight passed with status `covered` and no errors.

## Risks

- DELETE becomes persistent. Operators need the documented resume path.
- Active-only filtering must not hide retired identities from creation or authorization.
- Archive cleanup and legacy schema rebuilds can erase retirement unless explicitly covered.
- Preflight checks alone cannot fence a delayed result. Result application and retirement must share serialization.
- Explicit retirement stops watch-driven automation observations but preserves configuration and already queued actions.
- This targeted opt-out does not eliminate all idle GitHub polling or establish rate-limit recovery.
