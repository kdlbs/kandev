---
created: 2026-11-10
status: done
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001
system_design:
  - ../../specs/integrations/system-design/jira-watch-deduplication.md
legacy_specs: []
---

# Implementation plan: Jira watch recovery after task deletion

## Overview

[Issue #5206](https://github.com/kdlbs/kandev/issues/5206) reports matching issues stranded after native task deletion.
The issue is assigned to `carlosflorencio`.

One sequential work order corrects both deduplication gates and covers the native deletion-to-watch flow.
Requirements and design are active. Implementation and permanent regressions are complete; the PR remains pending.

## Scope

### In scope

- Make Jira poll deduplication distinguish missing tasks from existing tasks and unfinished reservations.
- Reclaim only an exact stale watch-and-issue mapping inside an atomic reservation transaction.
- Recover existing stale mappings on eligible polls.
- Preserve archive protection, watch identity, routing, capacity, and reset behavior.
- Cover concurrent claims, mixed watch states, SQL errors, and native deletion.
- Document the resulting Jira behavior with the implementation.

### Out of scope

- Production deployment, live data edits, operator purges, watch resets, custom deployment, or provider writes.
- Global deduplication, other integrations, unfinished-reservation aging, or failed attachment repair.
- Query pagination, new settings, migrations, rendered UI, or localization changes.

## Confirmed root cause

Investigation used `361d0dbd641c6df5d44221973c4ac3bc85adcb20`, equal to remote `main` at the time of inspection.
The canonical issue was open and had no comments or image attachments.

The native DELETE handler delegates to `service.DeleteTaskWithOptions`, then native task repository deletion.
`jira_issue_watch_tasks` references its watch through a foreign key, but its `task_id` has no task foreign key.
Task deletion and the task-deleted orchestrator handler do not remove Jira mappings.

`Store.ListSeenIssueKeys` selects every issue key for a watch. `CheckIssueWatch` therefore filters an issue whose task no longer exists.
`Store.ReserveIssueWatchTask` uses `ON CONFLICT ... DO NOTHING`, so bypassing only the polling filter still cannot acquire that issue.
Both gates require correction.

### Reproduction evidence

A temporary `TestRepro5206NativeTaskDeletion` used the real task repository and Jira service on one disposable database.
It created a task, reserved and assigned a Jira mapping, and verified suppression while the task existed.
It then called the native repository's `DeleteTask`, followed by `CheckIssueWatch` and reservation acquisition.

Command, from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/jira -run '^TestRepro5206NativeTaskDeletion$' -count=1 -v)
```

Observed result: exit 1, with the intended two failing assertions:

```text
after native repository deletion: task_count=0 mapping_count=1
matching issue stranded after native deletion: got 0 eligible issues, want 1
dispatch cannot reclaim deleted task mapping
```

The temporary test was removed after diagnosis. No production or permanent test changes remain.
This proves the repository-to-service failure; HTTP preflight and asynchronous poll publication remain planned regression coverage.

## Requirement and design reconciliation

No existing Jira acceptance criterion defines deduplication after task deletion.
The prompt requirement owns issue content; the watch-cleanup requirement covers different providers and preserves their deduplication behavior.

The new capability pair defines the missing Jira contract under Integrations.
Task existence remains owned by Tasks. No existing requirement or task lifecycle design needs amendment.

- [Requirement: `REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001`](../../specs/integrations/requirements/jira-watch-deduplication.md)
- [System design](../../specs/integrations/system-design/jira-watch-deduplication.md)

## Technical approach

### Jira store

Move `ListSeenIssueKeys` and `ReserveIssueWatchTask` from `apps/backend/internal/jira/store.go` into `store_issue_watch_dedup.go`.
Keep their public signatures. Keep assignment, release, reset, and source-link lookup unchanged.

The seen-set query includes unfinished reservations and mappings with an existing task. It excludes only assigned mappings whose tasks are absent.
Do not filter on `archived_at`, completion, or activity.

The reservation transaction serializes the exact identity, removes only its assigned stale row, and performs the existing unique insert.
SQLite uses the production writer factory; PostgreSQL uses the existing transaction-scoped advisory-lock pattern.
Any failure rolls back the stale-row removal. Success requires a committed new reservation.

This approach recovers old mappings without coupling task deletion to integration stores.
Deletion-only cleanup cannot recover rows left by previous releases. A poll-only filter still loses at the reservation gate.

### Test fixtures and native lifecycle

Add a minimal `tasks` table to standalone Jira fixtures. Seed actual task rows wherever a test claims that an assigned mapping is live.
Update `store_postgres_test.go`, which currently assigns `task-1` without creating it.
Use a full native task schema for deletion integration tests; do not upgrade a partial task table into a native repository fixture.

Use bounded event channels for poller publication. Make the watch due explicitly before the next tick.
The native HTTP regression uses the real task service and repository, deletion preflight, and the Jira immediate-trigger route.
Drive the existing reservation, task-creation, and attachment boundaries explicitly; do not add a production test hook or coordinator constructor.

### Compatibility matrix

| Boundary | Identity and transport | Result | Evidence |
| --- | --- | --- | --- |
| Jira clients | Cloud, Server/Data Center, MCP, mock through `CheckIssueWatch` | Same local existence predicate; provider calls unchanged | Real store/service tests; existing client tests |
| Polling | Enabled, due watch; current result page | Missing assigned tasks can dispatch | Real poller and manual trigger regressions |
| Reservation | Exact watch ID and issue key | One committed claimant | SQLite and PostgreSQL connection tests |
| Existing task | Active, completed, or archived | Remains suppressed | Mixed-state store and native-route controls |
| Multiple watches/workspaces | Same issue key, separate watch identities | Recover only eligible mappings | Mixed-state and shared-missing-task regressions |
| Unfinished reservation | Empty `task_id` | Remains protected | Store and concurrent-claim regressions |
| Missing schema or SQL error | No reliable task-existence result | No delivery or successful claim | Error and rollback regressions |

### Public documentation

Update `docs/public/integrations.md`, under **Jira issue watches**, during implementation.
Its primary content type is reference. Explain archived-task protection, recreation after permanent deletion, per-watch identity, and existing page/capacity limits.
Keep the existing watch deletion and destructive reset guidance.
The root README and screenshot catalog need no change because this repair adds no entry point or visual state.

## Tests

All test names below are permanent regressions.

| Acceptance criteria | File and test |
| --- | --- |
| `.1`, `.6` | `internal/jira/service_issue_watch_dedup_test.go`: `TestService_CheckIssueWatch_DeletedTaskEligible` |
| `.1`, `.6` | `internal/jira/poller_issue_watch_dedup_test.go`: `TestPoller_CheckIssueWatches_RecoversDeletedTask` |
| `.2`, `.3` | `internal/jira/store_issue_watch_dedup_test.go`: `TestStore_IssueWatchDedup_MappingStates` |
| `.4` | `internal/jira/store_issue_watch_dedup_test.go`: `TestStore_IssueWatchDedup_ConcurrentReclaim`; one-winner burst with independent production SQLite writers |
| `.4` | `internal/jira/store_issue_watch_dedup_wait_test.go`: `TestStore_IssueWatchDedup_WaitingClaimantPreservesReplacement`; holder writer transaction installs the replacement before the blocked claimant resumes |
| `.4` | Same file: `TestPostgresIssueWatchDedup_WaitingClaimantPreservesReplacement`; holder advisory-lock transaction installs the replacement before the blocked claimant resumes |
| `.5` | `internal/jira/store_issue_watch_dedup_test.go`: `TestStore_IssueWatchDedup_WatchIsolation`; include missing, active, archived, and unfinished mappings for the same issue |
| `.5`, `.6` | `internal/jira/store_issue_watch_dedup_test.go`: `TestStore_IssueWatchDedup_SharedMissingTask`; several watches reference one missing task |
| `.7` | `internal/jira/store_issue_watch_dedup_test.go`: `TestStore_IssueWatchDedup_ReclaimRollback`; compare every original mapping field after injected insert failure |
| `.7` | `internal/jira/service_issue_watch_dedup_test.go`: `TestService_CheckIssueWatch_TaskLookupFailure`; retain the existing dedup-query error test |
| `.1` through `.7` | `internal/jira/store_issue_watch_dedup_postgres_test.go`: `TestPostgresIssueWatchDedup`; exercise both store methods with real connection contention |

Each decimal reference above belongs to `AC-INTEGRATIONS-JIRA-WATCH-DEDUP-001`.
Cover successful insertion, stale replacement, existing-task refusal, unfinished-claim refusal, rollback, and cancellation in the store tests.
Do not weaken existing assertions to accommodate incomplete fixtures.

## E2E tests

Add `internal/task/handlers/task_delete_jira_watch_test.go` with `TestHTTPDeleteTask_JiraWatchRecovery`.
Use the same real temporary database for Jira and task state, plus a deterministic Jira mock search.
No browser project is required: this defect is below rendering and has native HTTP evidence.

1. Seed a task, watch, and assigned mapping. Verify that an immediate trigger reports no new issue.
2. Obtain native preflight consent, then DELETE the task with its confirmation header.
3. Verify task absence, then invoke `POST /api/v1/jira/watches/issue/:id/trigger` in the correct workspace.
4. Await the published event, claim it through the real store/source, create a real replacement task, and attach its identity.
5. Verify exactly one replacement and no new issue on the following trigger.
6. Repeat the control without deletion and with archival; both retain the original task and mapping.

This flow covers `.1`, `.2`, `.4`, and `.6`. A separate real poller test covers ordinary tick publication.
Channels provide completion evidence; no wall-clock sleep or live Jira account is required.

## Work orders

- [x] [Task 01: Recover Jira mappings after native task deletion](task-01-recover-deleted-task-mappings.md) (`done`, sequential, no dependencies)

## Verification strategy

The work order owns the complete runnable commands, including changed-file lint, SQL guard, store conformance, and documentation gates.
Primary regression commands are:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/jira -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/handlers -run '^TestHTTPDeleteTask_JiraWatchRecovery$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'TestCoordinator_Dispatch_|TestCreateJiraIssueTask_|TestInterpolateJiraPrompt' -count=1)
```

PostgreSQL behavior requires a real `KANDEV_TEST_POSTGRES_DSN` and the work order's guarded command.
Implementation ran the PostgreSQL tests against a disposable PostgreSQL 16 instance; the DSN value is not recorded.

## Verification results

Diagnosis: the temporary native repository/service reproduction failed at both intended gates, then was removed.
Assignment: GitHub confirmed `carlosflorencio` as the issue assignee.
Implementation and permanent regressions verified on 2026-11-10:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/jira -count=1)`: passed with a disposable PostgreSQL 16 `KANDEV_TEST_POSTGRES_DSN`, including `TestPostgresIssueWatchDedup` and `TestPostgresIssueWatchDedup_WaitingClaimantPreservesReplacement`.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/handlers -run '^TestHTTPDeleteTask_JiraWatchRecovery$' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'TestCoordinator_Dispatch_|TestCreateJiraIssueTask_|TestInterpolateJiraPrompt' -count=1)`: passed.
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)`: passed with no findings.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)`: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, and `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`: passed.
- The work order's Node `validateCoverage` command reported `covered` for the runtime path and all four package files.
- `git diff --check`: passed.

Implementation and permanent regressions are complete. No PR is included in this turn.

## Risks

- Ignoring empty reservations can duplicate creation or a task whose attachment failed. Keep that sentinel protected.
- A reclaim without writer serialization can remove a concurrent winner's claim. Exercise independent connections on both dialects.
- A missing-task row in another watch must not invalidate a valid mapping. Keep exact watch predicates on every read and write.
- Broad JQL still returns only its first page. The repair cannot recover an issue that never enters the current result window.
- PostgreSQL coverage requires a configured test database. A skip cannot count as successful concurrency evidence.
