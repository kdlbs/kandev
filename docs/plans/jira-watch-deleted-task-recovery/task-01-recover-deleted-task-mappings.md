---
id: "01-recover-deleted-task-mappings"
title: "Recover Jira mappings after native task deletion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.1
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.2
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.3
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.4
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.5
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.6
  - AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001.7
system_design:
  - ../../specs/integrations/system-design/jira-watch-deduplication.md
---

# Task 01: Recover Jira mappings after native task deletion

## Summary

Correct the Jira seen-set query and reservation transaction so a missing mapped task does not strand a matching issue.
Prove recovery through native deletion and polling while existing tasks and unfinished claims remain protected.

## In scope

- Move the two deduplication methods into `store_issue_watch_dedup.go` without changing their public signatures.
- Add the task-existence predicate to the bulk seen-set query.
- Reclaim an exact stale mapping inside the reservation transaction, with SQLite writer and PostgreSQL advisory-lock serialization.
- Supply task tables and live rows in standalone Jira fixtures.
- Add the named store, service, poller, PostgreSQL, and native HTTP regressions from the plan.
- Update the Jira watch reference in `docs/public/integrations.md` with the implemented behavior.
- Synchronize this work order and the plan with actual command results after implementation.

## Out of scope

Do not modify live databases, reset watches, deploy a runtime, change other providers, or infer stale unfinished claims from their age.
Keep task deletion production code, provider search, routing, capacity, and automatic start unchanged.

## Acceptance

1. The deleted-task regression fails at the old polling and reservation gates, then passes after both corrections.
2. Active, completed, archived, unfinished, and mixed-watch controls remain protected; concurrent recovery has one winner and errors preserve retryable state.
3. Native preflight/DELETE followed by watch triggering produces one replacement task; a subsequent trigger produces none. All listed verification commands pass with PostgreSQL evidence.

## Implementation sequence

1. Read the current plan, requirement, design, scoped guidance, and referenced test patterns.
2. Re-read any changed upstream methods since the recorded investigation commit.
3. Mark this work order `in_progress` and update the plan.
4. Add the meaningful permanent regressions before production changes.
5. Run the deleted-task service and native lifecycle tests and record the expected failures.
6. Correct both store methods and supply valid task fixtures.
7. Run the complete verification block from the repository root.
8. Record actual results, including PostgreSQL execution rather than skips, before marking this order `done`.

## Test details

Use `TestStore_IssueWatchDedup_MappingStates` for no mapping, empty claim, active task, archived task, completed task, and missing task.
Use real task rows for all states that claim task existence.

`TestStore_IssueWatchDedup_WatchIsolation` includes one missing mapping and another valid mapping for the same issue.
Include archived and unfinished controls, plus another workspace with that issue key.
`TestStore_IssueWatchDedup_SharedMissingTask` makes several watches reference one missing task and verifies independent claims.

`TestStore_IssueWatchDedup_ConcurrentReclaim` uses independent production SQLite connections and barriers to prove one winner in a simultaneous burst.
`TestPostgresIssueWatchDedup` uses a real PostgreSQL pool and covers both modified store methods.
`TestStore_IssueWatchDedup_WaitingClaimantPreservesReplacement` and `TestPostgresIssueWatchDedup_WaitingClaimantPreservesReplacement` hold the relevant writer or advisory lock in one transaction, start the real reservation on another connection, observe the claimant's database or driver busy/lock barrier, install a real replacement task and mapping in the holder transaction, then commit. The waiting claim must return false and preserve the replacement reservation ID and task ID.
Include a waiting claimant after the winner creates and attaches a real task.

Inject insertion failure after stale-row removal in `TestStore_IssueWatchDedup_ReclaimRollback`.
Verify the original ID, watch ID, issue key, URL, task ID, and timestamp after rollback; then remove the failure and retry.
Missing-table/query failures must return errors without task delivery or a successful claim.

`TestPoller_CheckIssueWatches_RecoversDeletedTask` invokes the real native repository deletion and an explicitly due poller check.
Await event delivery through a bounded channel; preserve last-polled and interval assertions.

`TestHTTPDeleteTask_JiraWatchRecovery` uses a full native task schema, real task service, real Jira store/service, and deterministic provider data.
Obtain consent from `/api/v1/tasks/delete-preflight`, then send the confirmation header with DELETE.
Use the existing immediate watch trigger to exercise the same search, filtering, and publication path without timer sleeps.
Await the event before driving real reservation, creation, and attachment. Repeat the trigger after replacement and include existing/archived controls.
The handler test can use `authzDeleteCleanup` and `withTaskDeleteTestIdentity`; do not use the mock deletion repository as task-existence evidence.

## Verification

Run this complete block from the repository root. Provide the PostgreSQL test DSN through the existing environment; never record its value.
The guard fails if PostgreSQL coverage is unavailable.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/jira -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/handlers -run '^TestHTTPDeleteTask_JiraWatchRecovery$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'TestCoordinator_Dispatch_|TestCreateJiraIssueTask_|TestInterpolateJiraPrompt' -count=1)
(test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && cd apps/backend && go test -trimpath -tags fts5 -race ./internal/jira -run '^TestPostgres' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/jira/store.go apps/backend/internal/jira/store_issue_watch_dedup.go apps/backend/internal/jira/store_test.go apps/backend/internal/jira/store_issue_watch_dedup_test.go apps/backend/internal/jira/store_issue_watch_dedup_wait_test.go apps/backend/internal/jira/store_postgres_test.go apps/backend/internal/jira/store_issue_watch_dedup_postgres_test.go apps/backend/internal/jira/service_issue_watch_dedup_test.go apps/backend/internal/jira/poller_issue_watch_dedup_test.go apps/backend/internal/task/handlers/task_delete_jira_watch_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
node <<'NODE'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/jira-watch-deleted-task-recovery/plan.md',
  'docs/plans/jira-watch-deleted-task-recovery/task-01-recover-deleted-task-mappings.md',
  'docs/specs/integrations/requirements/jira-watch-deduplication.md',
  'docs/specs/integrations/system-design/jira-watch-deduplication.md',
];
const result = validateCoverage({
  changedFiles: ['apps/backend/internal/jira/store.go', ...paths].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
assert.equal(result.ok, true, result.errors.join('\n'));
assert.equal(result.status, 'covered');
console.log('Prospective runtime documentation coverage: covered');
NODE
git diff --check
git status --short -- docs/plans/jira-watch-deleted-task-recovery
```

The coverage command intentionally includes the proposed runtime path, so a documentation-only exemption cannot conceal a broken artifact reference.
If implementation changes another Go fixture, include its path in changed-file lint and its package in verification.

## Files likely touched

- `apps/backend/internal/jira/store.go`
- `apps/backend/internal/jira/store_issue_watch_dedup.go` (new)
- `apps/backend/internal/jira/store_test.go`
- `apps/backend/internal/jira/store_issue_watch_dedup_test.go` (new)
- `apps/backend/internal/jira/store_issue_watch_dedup_wait_test.go` (new)
- `apps/backend/internal/jira/store_postgres_test.go`
- `apps/backend/internal/jira/store_issue_watch_dedup_postgres_test.go` (new)
- `apps/backend/internal/jira/service_issue_watch_dedup_test.go` (new)
- `apps/backend/internal/jira/poller_issue_watch_dedup_test.go` (new)
- `apps/backend/internal/task/handlers/task_delete_jira_watch_test.go` (new)
- `docs/public/integrations.md`
- This work order and `plan.md` for execution results.

## Dependencies

None. Implement this single work order sequentially after an explicit user request.

## Risks

- Deleting an empty reservation can duplicate a task that already exists but lacks an attached ID.
- An issue-only cleanup predicate can erase another watch's valid mapping.
- A separate stale delete and insert without one transaction can lose claims during failures or concurrent dispatch.
- PostgreSQL needs a real multi-connection regression; SQLite-only evidence is insufficient.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/jira-watch-deduplication.md): all criteria under `REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001`.
- [Design](../../specs/integrations/system-design/jira-watch-deduplication.md): mapping states and atomic claim recovery.
- [Plan](plan.md): root cause, reproduction evidence, compatibility, and test matrix.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- `internal/jira/store_issue_watch_test.go`, `service_issue_watch_test.go`, `poller_issue_watch_test.go`, and `store_postgres_test.go`.
- `internal/github/store_pr_watch_retirement.go`: existing PostgreSQL advisory-lock shape.
- `internal/task/handlers/task_delete_preflight_test.go`, `task_delete_authz_test.go`, and `repository_branch_policy_handlers_test.go`: consent, identity, and real database patterns.
- `internal/orchestrator/source_jira.go` and `watcher_dispatch.go`: unchanged reservation ownership and task-creation boundaries.

## Results

Implemented on 2026-11-10:

- `ListSeenIssueKeys` and `ReserveIssueWatchTask` moved to `store_issue_watch_dedup.go`. The seen query now admits only empty reservations and mappings whose task still exists. The reservation transaction reclaims an exact stale watch-and-issue row before the existing unique insert.
- SQLite serialization uses the production `db.OpenSQLite` writer transaction. PostgreSQL uses a transaction-scoped advisory lock on the watch-and-issue identity.
- Standalone Jira fixtures now provide a minimal `tasks` table; the PostgreSQL schema replay test creates its mapped task instead of referencing a missing one.
- Added permanent store, service, poller, PostgreSQL, and native HTTP regressions, including mixed watch states, shared missing tasks, concurrent recovery, rollback, lookup failure, archive control, and native deletion followed by replacement.
- Added deterministic waiting-claimant regressions for SQLite and PostgreSQL. Each holds the writer or advisory lock in one transaction, observes the claimant's SQLITE_BUSY or exact PostgreSQL advisory-lock wait in the current database, installs a replacement task and mapping in the holder transaction, commits, and verifies the waiting claim loses while preserving the replacement reservation ID and task ID.
- Added a cancellation regression proving a claim cancelled immediately after `BEGIN` leaves the original mapping unchanged.
- Updated `docs/public/integrations.md` with archived-task protection, permanent-deletion recovery, per-watch identity, and unfinished-reservation behavior.

Verification results:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/jira -count=1)`: passed, including `TestPostgresIssueWatchDedup`, `TestPostgresIssueWatchDedup_WaitingClaimantPreservesReplacement`, `TestStore_IssueWatchDedup_CancelledClaimAfterBegin`, and `TestPostgresStoreSchemaReplay` against a disposable PostgreSQL 16 instance.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/handlers -run '^TestHTTPDeleteTask_JiraWatchRecovery$' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'TestCoordinator_Dispatch_|TestCreateJiraIssueTask_|TestInterpolateJiraPrompt' -count=1)`: passed.
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)`: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, `node --test scripts/validate-public-docs.test.mjs`, and `node scripts/validate-public-docs.mjs`: passed.
- The work order's Node `validateCoverage` command reported `covered`.
- `git diff --check`: passed.
