---
created: 2026-10-28
status: completed
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
system_design:
  - ../../specs/tasks/system-design/turn-change-preservation.md
  - ../../specs/system-page/system-design/history-compaction.md
legacy_specs: []
---

# Implementation plan: Git turn history and unified compaction

## Overview

Retain immutable Git checkpoints, calculate historical diffs on demand, and compact old history through the existing message-compaction policy.
This revision replaces the earlier SQL-export retry approach after upstream research and the user's explicit scope choices.
There is one compaction period, switch, analysis action, manual action, and automatic schedule.

Tasks owns checkpoint identity, content sources, and availability.
System-page independently owns the shared administrative compaction contract.
The [accepted decision](../../decisions/2026-10-28-git-turn-history-and-unified-compaction.md) links these boundaries.

## Evidence

Task `09c0f799-be0d-482e-b46a-cc39d1d9cfba` retained ten file entries and `+1346/-57`, but zero SQL content bytes.
Its execution logged a processing timeout. Both accepted Git endpoints still reproduced the recorded counts.
Current code finalizes export errors as content unavailable and skips finalized turns on retry.
The source does not log the exact original export error.

Pinned t3code production code uses Git checkpoint refs and on-demand diffs, including the revision Kandev originally studied.
Kandev retains stricter fresh endpoints and explicit remote-source preservation.
Its measured SQL diff footprint was 27.45 MiB within a 13.69 GiB database.
The redesign addresses storage ownership and completion dependencies, not a claim of substantial immediate database shrinkage.

## Scope

In scope:

- Git source locators, common-repository reads, backend-owned object archives, and legacy SQL compatibility.
- Bounded preservation and query retries with immutable identities and cleanup protection.
- One task-inactivity policy and job covering old tool details and eligible historical content.
- History-only task discovery, retained summaries, explicit expiry, exact owned-ref cleanup, and bounded recovery receipts.
- Full-scope backup preparation and restore, shared progress, desktop/phone behavior, and localization.

Out of scope:

- Production edits in this design turn, live database repair, and automatic reopening of old failed turns.
- New history-retention toggles, independent cleanup schedules, user-branch deletion, or forced GC in user repositories.
- New PostgreSQL compaction support or unsupported-executor qualification claims.
- Implicit VACUUM or changes to the existing physical database-maintenance action.

## Technical approach

1. Add a source-aware historical content reader and internal locator beside current interval metadata.
   New Git-backed captures do not require SQL patch/rendering blobs. Existing SQL history remains readable.
2. Preserve accepted Git objects across source cleanup, with durable archive receipts for disposable/remote sources.
   Resolve local common Git storage directly so a removed worktree cwd does not break valid history.
3. Expand `internal/system/toolretention` into the single administrative job owner for both data categories.
   Reuse shared maintenance admission and activity coordination. Task-owned operations provide source inventory, leases, and cleanup receipts.
4. Keep one cutoff and discover both tool-payload tasks and history-only tasks.
   Preserve conversation prose and turn/file metadata. Expire content explicitly and replay external cleanup idempotently.
5. Extend one pre-compaction backup artifact and verified receipt to SQL plus the required Git objects.
   Existing tool-only approvals require the existing preparation choice for the wider scope, without another control.
6. Keep current unsupported-engine behavior. Such installations retain their shipped history path until shared compaction is supported.
   Stop the competing fixed-age content sweep only where the unified policy has taken ownership.

The source and compaction designs define migration and failure details.
Proposed preservation timing remains bounded and requires actual object-transfer measurements.
Capacity admission is bounded independently from operator cleanup enablement.

## Compatibility

| Boundary | Treatment | Evidence |
| --- | --- | --- |
| Local and linked worktree | Exact accepted objects through common Git storage | Real Git tests after worktree removal |
| Disposable or remote executor | Verified object archive before destruction | Agentctl transport and lifecycle fixtures |
| Legacy SQL history | Existing reads until verified migration or authorized expiry | Real repository migration/expiry tests |
| Unsupported compaction engine | Preserve shipped compatible path; no advertised new cleanup | Explicit unsupported/migration regression |
| Mixed repositories | Preserve each successful source and its stable file identities | Mixed-state and repeated-import tests |

## ASCII UI preview

### UI-01: One history-compaction flow

Entry: the existing Data & Logs > Database card. Desktop and phone retain their current stacked composition.

```text
History compaction
Tasks inactive for [3] [Months]   [Analyze savings]
Analysis details: tool details + historical diffs
Automatic compaction [toggle]
[Compact history now]
```

The period is illustrative. No history-specific period, toggle, or action is added.
The current backup-or-skip preparation remains one flow with accurate combined coverage.
Phone retains 44px controls, one page scroll owner, and no horizontal overflow.
Map: AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.8, 002.10-002.12, and 003.5.

### UI-02: Historical content after compaction

The existing turn card retains counts and filenames and exposes its expired-history state.
Desktop keeps the docked viewer; phone keeps the full-height drawer and focus return.
Tool messages retain their existing removed-details state. Conversation prose remains readable.

## Tests

- `TestTurnGitHistoryReadsWithoutSQLPayloads`: new history has no SQL blobs and renders from the accepted Git pair.
- `TestTurnGitHistorySurvivesWorktreeRemoval`: use the common Git source after the original cwd disappears.
- `TestTurnGitHistoryPreservesRemoteSource`: destroy the fixture executor after verified archive and read the original patch.
- `TestTurnChangePreservationRetriesAcceptedObjects`: temporary failure retries without new endpoints or duplicate storage accounting.
- `TestHistoryCompactionIncludesHistoryOnlyTasks`: one cohort and task count covers both categories.
- `TestHistoryCompactionProtectsActiveAndLeasedSources`: revalidate activity, preservation, reads, and backup leases before mutation.
- `TestHistoryCompactionResumesRefCleanup`: crash between SQL expiry and expected-OID ref deletion without double counts.
- `TestHistoryCompactionBackupRestoresGitContent`: restore an uncommitted-only historical version after original-source removal.
- `TestHistoryCompactionScopeUpgradeRequiresPreparation`: preserve the period and reuse the single preparation flow.

Every work order records behavioral RED, exact commands, and final evidence.

## E2E tests

Desktop and mobile system compaction specs prove UI-01 with mixed tool/history fixtures and one preparation/operation.
Historical diff specs prove UI-02, on-demand reads after reload, and expired content with retained summaries.
Go integration tests establish real Git transfer, cleanup, and restore. Browser-only projections do not substitute for them.

## Work orders

- [x] [Task 01: Git sources and on-demand historical reads](task-01-git-history-content.md)
- [x] [Task 02: Durable checkpoint lifecycle](task-02-durable-checkpoint-lifecycle.md)
- [x] [Task 03: Unified history compaction and backup](task-03-unified-history-compaction.md)
- [x] [Task 04: Shared compaction surface and end-to-end evidence](task-04-shared-compaction-surface.md)

Tasks 01 through 04 were implemented in sequence.
Each work order records its focused evidence and remaining non-blocking gaps.

## Verification results

Implementation validation passed:

- `go test -trimpath ./internal/task/changes ./internal/system/toolretention ./internal/system/backups -count=1` (including archive identity, copy-section, endpoint-attribute, stream-bound, retrier, activity-cohort, reclamation, deletion-intent, pagination, backup-bundle, and legacy-engine cases).
- `go test -trimpath ./internal/task/repository/sqlite ./internal/task/handlers -run 'TurnChange|TurnHistory|Preservation|TurnGitHistory' -count=1`.
- `go test -trimpath -race ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnGitHistory|TurnChangeSource|TurnChangePreservation|HistoryCompaction'`.
- `go test -trimpath -race ./internal/system/toolretention ./internal/system/backups -count=1`.
- `go run -trimpath ./cmd/sqlguard ./internal` and changed-package Go lint (0 issues).
- Focused regressions include `TestTurnHistoryRetentionRequiresInactiveTaskAndRevalidatesMutation`, `TestTurnChangePreservationRejectsMismatchedArchiveIdentity`, `TestTurnGitHistorySelectsTheCopiedDestinationSection`, `TestTurnGitHistoryReadsUseAcceptedEndpointAttributes`, `TestTurnGitHistoryRejectsOversizedReadAtTheStreamBound`, `TestPreservationRetrierRecoversTemporaryFailureAcrossPasses`, `TestHistoryCompactionAnalysisPaginatesPastOneThousandIntervals`, `TestHistoryCompactionDrainsAllOwnedRefReceipts`, `TestTurnGitHistoryReclaimsOwnedObjectsOnlyAfterAllRefsRelease`, `TestDeleteTaskStagesTurnHistoryCleanupIntent`, `TestTurnGitHistoryCommonSourceSurvivesLinkedWorktreeRemoval`, and `TestRetentionDownloadBundleIncludesHistorySidecar`, and `TestHistoryCompactionBackupRejectsTruncatedManifest`, `TestTurnGitHistoryImportReusesKnownObjectsWithoutNewPacks`, and `TestTurnGitHistoryCommonSourceSurvivesLinkedWorktreeRemoval`.
- Desktop `tests/git/turn-changed-files.spec.ts` (6 passed) and `tests/system/tool-payload-retention.spec.ts` (5 passed).
- Mobile `tests/git/mobile-turn-changed-files.spec.ts` and `tests/system/mobile-tool-payload-retention.spec.ts` (all passed).
- `pnpm run i18n:check`, `pnpm run typecheck`, public-docs validation (54 pages, 63 tests), catalog validation (415 decisions, 1,804 specifications), specification lint, and whitespace checks.

The static-review remediation also added: shared task-activity eligibility and mutation revalidation for history expiry; removal of the obsolete 30-day read deadline on Git-owned installations with legacy-engine gating; a durable bounded preservation retrier and 90-second summary budget; complete paged backup inventory bound to the database image plus a downloadable bundle; verified common-Git local reads, missing-object transfer, and task/installation capacity admission; owned-store reclamation and task/session deletion intents; destination-section selection, endpoint attributes, and stream-bounded reads; executor archive identity validation; and paginated analysis/cleanup accounting.

PostgreSQL-specific history tests were not run because `KANDEV_TEST_POSTGRES_DSN` is unset.
The parent feature's executor and performance qualification gaps remain open.

## Risks

- Source locators and backups must remain usable after worktree or executor removal.
- Deleting refs does not prove physical bytes were reclaimed. Shared object accounting must remain honest.
- Database-only backup receipts cannot cover deleted Git history.
- Old compaction enablement cannot silently authorize the expanded removal scope.
- Existing unsupported database engines must retain their working history path during migration.
- New time, transfer, and storage bounds need measurement, not inference from SQL gzip sizes.
