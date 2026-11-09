---
id: "02-history-comparison"
title: "Accept one history-aware comparison pair"
status: done
wave: 2
depends_on:
  - 01-history-provenance
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.2
  - AC-TASKS-TURN-CHANGES-002.3
  - AC-TASKS-TURN-CHANGES-002.4
  - AC-TASKS-TURN-CHANGES-002.5
  - AC-TASKS-TURN-CHANGES-002.6
  - AC-TASKS-TURN-CHANGES-002.8
  - AC-TASKS-TURN-CHANGES-002.10
  - AC-TASKS-TURN-CHANGES-002.11
  - AC-TASKS-TURN-CHANGES-002.12
  - AC-TASKS-TURN-CHANGES-002.13
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-006.3
  - AC-TASKS-TURN-CHANGES-007.1
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 02: Accept one history-aware comparison pair

## Summary

Generate and accept a direct or upstream-adjusted comparison from immutable evidence.
Use that same pair for file metadata, line totals, patches, and content export.

## In scope

- Implement the design's eligibility table and controlled clean three-way merge.
- Probe merge-tree capability and reject unsafe custom merge drivers/attributes.
- Retain the derived baseline under an owned ref and accept it conditionally once.
- Keep raw endpoint columns unchanged; preserve explicit legacy_raw semantics.
- Return history_unavailable without numeric turn-edit claims for conflicts,
  divergent/multiple bases, changed targets, raced evidence, or unsupported movement.
- Wire effective comparison identity through agentctl API/client, coordinator,
  task DTOs, application reads, and compact summary events.
- Validate raw/effective identity on every compare/export response.
- Require explicit version negotiation. Old unversioned backend requests keep
  raw semantics even when the agentctl supports adjustment; reject unknown versions.
- Preserve ordinary commits, unchanged dirty content, reverts, modes, and exact paths.
  Healthy repository siblings must remain usable.

## Out of scope

Writer attribution, resumable jobs, final UI/copy, and automatic conflict resolution.

## Acceptance

1. A real rebase/merge fixture excludes upstream content and reports only net
   interval changes after carrying the complete starting state onto the new base.
2. Conflicting or unsupported history remains explicitly unavailable.
   A file that matches upstream after a legitimate revert is still counted.
3. Duplicate or delayed processing cannot change the accepted comparison,
   replace its files, or mix raw totals with adjusted patches.

## TDD and regression

Add proposed process/turn_checkpoint_history_test.go,
task/changes/coordinator_history_test.go, and
repository/sqlite/turn_change_comparison_test.go.
RED: TestTurnCheckpointHistoryRebaseExcludesUpstream creates an upstream advance,
rebases inside the turn, and asserts one turn file instead of the broad raw inventory.

Include TestTurnCheckpointHistoryPreservesDirtyBaseline,
TestTurnCheckpointHistoryRetainsRevert, TestTurnCheckpointHistorySameFileChanges,
TestTurnCheckpointHistoryConflictUnavailable,
TestTurnCheckpointHistoryMetadataAndModes, and
TestCoordinatorHistoryComparisonAcceptedOnce.
Exercise advanced target already present at start, unchanged base/commits/amend,
target aliasing the writable branch, missing capabilities, old backend/new agentctl
requests, and mixed healthy/unavailable checkouts.
Use a marker command to prove custom merge drivers never execute.
API tests reject a mismatched effective pair.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/service -run 'TestTurnCheckpointHistory|TestCoordinatorHistory|TestTurnChangeHistoryComparison|TestPostgresTurnChangeComparison' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/task/changes ./internal/task/repository/sqlite -run 'TestCoordinatorHistoryComparisonAcceptedOnce|TestTurnChangeHistoryComparison.*(Concurrent|Retry)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

Run Postgres-prefixed tests against the configured DSN and report actual execution.
Run scripts/lint-go-changed for this work order's changed Go files.

## Files likely touched

Existing: apps/backend/internal/agentctl/server/process/turn_checkpoint_compare.go;
apps/backend/internal/agentctl/server/process/turn_checkpoint_diff_view.go;
apps/backend/internal/agentctl/server/process/turn_checkpoint_export.go;
apps/backend/internal/agentctl/server/api/git_turn_changes.go;
apps/backend/internal/agent/runtime/agentctl/client_turn_changes.go;
apps/backend/internal/common/turnchanges/contract.go;
apps/backend/internal/task/changes/coordinator_finish.go;
apps/backend/internal/task/repository/turn_changes.go;
apps/backend/internal/task/repository/sqlite/turn_changes.go;
apps/backend/internal/task/dto/turn_changes.go;
apps/backend/internal/task/service/service_turn_changes.go.

Proposed focused normalization, comparison-acceptance, and test files belong
beside these seams.

## Dependencies

01-history-provenance. The comparison receipt is accepted before inventory/content.
Work order 05 owns visible frontend projection and rendered verification.

## Risks

A clean merge policy cannot establish exclusive authorship.
A conflict tree OID must never be accepted merely because Git printed it.
Do not execute repository merge drivers or mutate real operation files.

## Parallelism

sequential

## Inputs

- History integrity design: Comparison selection; Accepted comparison; Compatibility.
- Existing NUL parser, isolated diff-view, compare/export, and coordinator tests.
- Proposed history integrity ADR.

## Results

Implemented versioned comparison negotiation, direct/upstream-adjusted/history-unavailable selection, controlled `merge-tree` baseline construction under an owned comparison ref, conditional write-once acceptance, and effective-pair projection through the coordinator. Real-Git tests cover upstream rebase exclusion, dirty baseline preservation, legitimate reverts, conflicts, and HEAD movement without target evidence. Older agentctl responses remain explicitly legacy raw instead of looping or silently upgrading.

### Fixup remediation (2026-10-28)

Adjusted comparisons wrap the local tree in a synthetic commit before invoking `merge-tree`. Git 2.43 rejects a bare tree as the second branch argument, while the commit form works across supported versions. `TestTurnCheckpointSyntheticTreeCommitWrapsTree` guards the wrapper.
