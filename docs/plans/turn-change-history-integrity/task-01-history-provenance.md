---
id: "01-history-provenance"
title: "Capture and persist immutable history provenance"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.1
  - AC-TASKS-TURN-CHANGES-002.4
  - AC-TASKS-TURN-CHANGES-002.13
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-007.1
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 01: Capture and persist immutable history provenance

## Summary

Capture versioned history evidence with each raw endpoint.
Persist provenance and nullable comparison fields without changing the current
raw comparison behavior in this work order.

## In scope

- Extend common/turnchanges CheckpointResult with bounded v2 provenance and capabilities.
- Resolve real HEAD, symbolic reference, configured target identity/commit, and
  unique merge base through existing tracker resolution.
- Store provenance in the immutable checkpoint body. Retain source HEAD/target
  parents without chaining checkpoints or touching user refs/index.
- Reject raced, malformed, or mismatched receipts; distinguish unavailable target
  evidence from failed content capture.
- Add provenance/comparison columns using replayable SQLite/PostgreSQL migrations.
  Set existing rows to legacy_raw without rewriting their endpoints/files/content.
- Extend endpoint acceptance and re-read validation so retries cannot observe new provenance.

## Out of scope

Normalization, writer reconciliation, export jobs, UI, new flags, and live-data repair.

## Acceptance

1. A retained endpoint returns its original provenance after HEAD/ref movement,
   restart, and an idempotent retry; mismatches cannot replace accepted evidence.
2. Fresh databases, upgrades, replay, and table rebuilds preserve endpoints and
   legacy rows across both supported SQL dialects.
3. Failed explicit target resolution never falls back to another branch.
   Provenance collection preserves the user's index, refs, and operation state.

## TDD and regression

Add proposed process/turn_checkpoint_provenance_test.go and
repository/sqlite/turn_change_history_provenance_test.go.
RED: TestTurnCheckpointProvenanceReceiptSurvivesHeadMove must fail because current
receipts lack stored HEAD/base evidence.
Include capture-time HEAD/target movement, unborn HEAD, provider-target failure,
missing ancestry, source-object retention, duplicate receipt, and migration replay.
Do not print Git configuration values or credentials.

## Verification

Run from the repository root. Test names are proposed; require nonzero matches.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -tags fts5 ./internal/common/turnchanges ./internal/agentctl/server/process ./internal/task/repository/sqlite -run 'TestTurnCheckpointProvenance|TestTurnChangeProvenance|TestPostgresTurnChangeProvenance' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/agentctl/server/process ./internal/task/repository/sqlite -run 'TestTurnCheckpointProvenance.*(Race|Retry)|TestTurnChangeProvenance.*(Concurrent|Replay)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/persistence/storeconformance -count=1)
```

Execute the Postgres-prefixed cases with KANDEV_TEST_POSTGRES_DSN configured.
An unset DSN or skip is a parity gap, not a passing PostgreSQL result.
Run scripts/lint-go-changed on the actual changed Go files before completion.

## Files likely touched

Existing: apps/backend/internal/common/turnchanges/contract.go;
apps/backend/internal/agentctl/server/process/turn_checkpoint_capture.go;
apps/backend/internal/agentctl/server/process/turn_checkpoint_delete.go;
apps/backend/internal/agentctl/server/process/comparison_target.go;
apps/backend/internal/task/models/turn_changes.go;
apps/backend/internal/task/changes/coordinator.go;
apps/backend/internal/task/repository/turn_changes.go;
apps/backend/internal/task/repository/sqlite/base_migrations.go;
apps/backend/internal/task/repository/sqlite/turn_changes_schema_ddl.go;
apps/backend/internal/task/repository/sqlite/turn_changes.go;
apps/backend/internal/persistence/storeconformance/testdata/upgrades/.

Proposed focused migration and test files may sit beside those sources.
Update required-store/upgrade fixtures rather than adding a parallel schema owner.

## Dependencies

None. Read the existing interval ADR and the proposed history integrity decision.

## Risks

Source-history retention can pin more existing objects until cleanup.
A fixed target-tip SHA is not the starting merge base.

## Parallelism

sequential

## Inputs

- History integrity design: Provenance; Accepted comparison; Compatibility; Persistence.
- Existing capture/index/environment tests and task repository endpoint acceptance tests.
- apps/backend/AGENTS.md and agentctl scoped guidance.

## Results

Implemented versioned checkpoint provenance (`version 2`) with real HEAD, symbolic ref, configured target identity/commit, unique merge base, operation state, and tree-merge capability. Provenance is embedded in the immutable checkpoint body, observed HEAD/target commits become source parents, retries parse the accepted body, raced captures are rejected, and provenance is persisted through additive SQLite/PostgreSQL-compatible columns. Legacy terminal rows are labelled `legacy_raw` without rewriting endpoints. Verified with real-Git provenance/retry tests and migration replay tests.

### Fixup remediation (2026-10-28)

Capture now reads provenance before preparing the snapshot and verifies it after the snapshot is written. A `post-index-change` test hook moves `HEAD` during preparation and requires a provenance failure, so old working-tree content cannot be paired with new history.
