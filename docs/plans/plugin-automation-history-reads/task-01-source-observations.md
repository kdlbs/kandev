---
id: "01-source-observations"
title: "Add bounded automation source observations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-AUTOMATION-READS-001
acceptance_criteria:
  - AC-PLUGINS-AUTOMATION-READS-001.1
  - AC-PLUGINS-AUTOMATION-READS-001.2
  - AC-PLUGINS-AUTOMATION-READS-001.3
  - AC-PLUGINS-AUTOMATION-READS-001.4
  - AC-PLUGINS-AUTOMATION-READS-001.5
  - AC-PLUGINS-AUTOMATION-READS-001.7
  - AC-PLUGINS-AUTOMATION-READS-001.10
  - AC-PLUGINS-AUTOMATION-READS-001.12
system_design:
  - ../../specs/plugins/system-design/automation-history-reads.md
---

# Task 01: Add Bounded Automation Source Observations

## Summary

Add workspace-scoped source readers for definitions and run observations.
Give run lists and exact refresh the same status and summary projection.
Keep execution readers and domain state unchanged.

## In scope

- New `ListHistoryAutomations`, `ListHistoryRuns`, and `GetHistoryRun` service
  methods with internal query and observation types.
- Bounded workspace/automation/time-range queries with deterministic identity
  tie-breakers and a row-limit sentinel.
- Stored status alongside the existing observed status, exact-turn summary
  hydration, and preserved retry/delivery links.
- Authorization, retention, taskless/skipped records, and read-only source tests.
- PostgreSQL behavior coverage for the new queries.

## Out of scope

- Host RPCs, SDK types, public documentation, and plugin authorization.
- Changing raw `Store.GetRun`, execution status writers, or retention policy.
- Schema migrations, source repairs, and new scheduler behavior.

## Acceptance

1. Scoped source queries return the complete bounded matching set, including
   records beyond the existing recent-feed limit, or identify overflow.
2. Lists and exact observations agree on unchanged source data while preserving
   raw status, absent links, and unknown values.
3. Missing/foreign identities and source errors stay distinct from successful
   empty queries. Reads leave all domain rows unchanged.

## Verification

Run from the repository root after the TDD implementation:

```bash
go -C apps/backend test -trimpath -race ./internal/automation -run 'TestAutomationHistory|TestListRunPage|TestListRuns|TestListWorkspaceRuns' -count=1
go -C apps/backend run ./cmd/sqlguard ./internal
go -C apps/backend test -trimpath -race ./internal/persistence/storeconformance -count=1
git diff --check
```

The first command includes `TestAutomationHistoryPostgres` when
`KANDEV_TEST_POSTGRES_DSN` is configured. Record a skip explicitly otherwise.
New test names and files are deliverables, not existing passing evidence.

Required source evidence includes equal timestamps, range edges, more than 200
runs, snapshot-limit overflow, and foreign filters.
It also includes taskless and skipped runs, cancelled retries, and later
completion without an event.
Compare persisted rows before and after reads. Preserve the existing archived
and cancelled-task status precedence.

## Files likely touched

- `apps/backend/internal/automation/history_reads.go` (new service/types).
- `apps/backend/internal/automation/store_history_reads.go` (new queries).
- `apps/backend/internal/automation/history_reads_test.go` (new).
- `apps/backend/internal/automation/history_reads_postgres_test.go` (new).
- Existing `store.go` projection helpers only where shared extraction is
  necessary.

## Dependencies

None.

## Risks

- Reusing a capped list can hide records before the Host applies its bounds.
- Raw status is used by execution code. Do not replace it with a display
  projection for existing callers.
- Source timestamps need portable SQL comparisons across SQLite and PostgreSQL.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/automation-history-reads.md).
- [Source observations design](../../specs/plugins/system-design/automation-history-reads.md#source-observations).
- Existing `automation/store_run_pages_test.go`, `store_runs_projection_test.go`,
  `store_workspace_runs_test.go`, and `store_postgres_test.go` patterns.
- `apps/backend/AGENTS.md` and `/tdd`.

## Results

Status: done.

Implemented `Service.ListHistoryAutomations`, `Service.ListHistoryRuns`, and
`Service.GetHistoryRun` with bounded, workspace-scoped store queries. Run lists
and exact refresh share the existing task-state status projection and
exact-turn summary hydration while retaining the stored status separately.
Recorded task, session, and turn identities are selected directly from the run
row, so an empty recorded session stays absent and a deleted task link remains
the source identity. User-interface fallbacks that invent the current primary
session or clear a deleted task are not used for history. Foreign and missing
identities collapse into one sentinel. Source queries select at most the
requested limit plus one and report overflow.

The history query uses a narrow, history-specific projection. It does not
select trigger bodies, retry launch or prompt snapshots, claim state, or other
private payload columns, so a bounded history request cannot materialize large
excluded payloads before the public snapshot is measured. History summary
hydration reads a bounded 4,096-character prefix; the adapter strips system
content before its final credential-redacted cut. Runs created before turn
binding retain the existing task-level summary fallback, while turn-bound runs
use only their exact turn message.

Verification:

- `go -C apps/backend test -trimpath -race ./internal/automation -run 'TestAutomationHistory|TestListRunPage|TestListRuns|TestListWorkspaceRuns' -count=1`: passed.
- `go -C apps/backend run ./cmd/sqlguard ./internal`: passed.
- `go -C apps/backend test -trimpath -race ./internal/persistence/storeconformance -count=1`: passed.
- `git diff --check`: passed.
- Added regressions for empty recorded sessions with a live primary session,
  primary-session replacement, retained task identity after linked-task
  deletion, legacy runs without a turn binding using the task-level summary
  fallback, and runs carrying large excluded trigger and retry payloads.
- `TestAutomationHistoryPostgres`: skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is unset.
